package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var doctorCmd = MustNewCommand(commands.DoctorMetadata, runDoctor)

func init() {
	MustAddToRoot(doctorCmd)
}

// writeEmbeddingStatus prints the vault's semantic-retrieval readiness in a
// human-readable form. When no dense embeddings exist, the output names the
// remedy explicitly so users don't have to read ask's zero-hit hint to
// discover it.
func writeEmbeddingStatus(w io.Writer, emb *query.DoctorEmbeddings) error {
	if emb == nil {
		return nil
	}
	if !emb.SemanticReady {
		_, err := fmt.Fprintf(w,
			"Embeddings: none (%d notes) — keyword-only retrieval\n"+
				"  run: vaultmind index --embed --model bge-m3 --vault <vault>\n",
			emb.TotalNotes)
		return err
	}
	_, err := fmt.Fprintf(w,
		"Embeddings: dense %d/%d (%s), sparse %d/%d, colbert %d/%d\n",
		emb.DenseCount, emb.TotalNotes, emb.Model,
		emb.SparseCount, emb.TotalNotes,
		emb.ColBERTCount, emb.TotalNotes)
	return err
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppDoctorVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "doctor")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	result, err := query.Doctor(vdb.DB, vaultPath)
	if err != nil {
		return fmt.Errorf("running doctor: %w", err)
	}

	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppDoctorJson) {
		env := envelope.OK("doctor", result)
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexHash = vdb.GetIndexHash()
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}
	w := cmd.OutOrStdout()
	if _, err = fmt.Fprintf(w,
		"Vault: %s\nNotes: %d (%d domain, %d unstructured)\nUnresolved links: %d\n",
		result.VaultPath, result.TotalFiles, result.DomainNotes,
		result.UnstructuredNotes, result.Issues.UnresolvedLinks); err != nil {
		return err
	}
	if err = writeEmbeddingStatus(w, result.Embeddings); err != nil {
		return err
	}
	if result.Issues.ObsidianIncompatibleLinks > 0 {
		if _, err = fmt.Fprintf(w, "Obsidian-incompatible links: %d\n", result.Issues.ObsidianIncompatibleLinks); err != nil {
			return err
		}
		for _, il := range result.Issues.IncompatibleLinkDetails {
			if _, err = fmt.Fprintf(w, "  %s: [[%s]] → [[%s|%s]]\n",
				il.SourcePath, il.TargetRaw, il.SuggestedFix, il.TargetRaw); err != nil {
				return err
			}
		}
	}
	if result.Issues.PathPseudoIDLinks > 0 {
		if _, err = fmt.Fprintf(w, "Dead link references: %d\n", result.Issues.PathPseudoIDLinks); err != nil {
			return err
		}
		for _, pl := range result.Issues.PathPseudoIDDetails {
			if _, err = fmt.Fprintf(w, "  %s: [[%s]] → target file does not exist\n",
				pl.SourcePath, pl.TargetRaw); err != nil {
				return err
			}
		}
	}
	return nil
}
