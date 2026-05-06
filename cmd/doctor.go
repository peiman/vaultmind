package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/hooks"
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
// discover it. When BGE-M3 coverage is imbalanced (some notes missing sparse
// or colbert), a warning line surfaces the failure mode that silently
// compresses hybrid RRF ranking.
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
	if _, err := fmt.Fprintf(w,
		"Embeddings: dense %d/%d (%s), sparse %d/%d, colbert %d/%d\n",
		emb.DenseCount, emb.TotalNotes, emb.Model,
		emb.SparseCount, emb.TotalNotes,
		emb.ColBERTCount, emb.TotalNotes); err != nil {
		return err
	}
	if emb.Model == "mixed" && len(emb.MixedModel) > 0 {
		// Surface the per-model breakdown explicitly. Without this, the
		// summary line says "(mixed)" which tells the operator something is
		// off but not what fraction is which model. Knowing the split lets
		// the operator decide whether to wait for incremental embed to
		// converge or run a full --embed pass right away. See vaultmind#22.
		parts := make([]string, 0, len(emb.MixedModel))
		for _, m := range emb.MixedModel {
			parts = append(parts, fmt.Sprintf("%d %s", m.Count, m.Model))
		}
		if _, err := fmt.Fprintf(w, "  mixed-model state: %s\n", strings.Join(parts, ", ")); err != nil {
			return err
		}
	}
	if emb.HasModalityImbalance {
		_, err := fmt.Fprintf(w,
			"⚠ Partial BGE-M3 coverage: %d note(s) missing sparse, %d missing colbert — "+
				"hybrid RRF ranking will be compressed for these notes.\n"+
				"  run: vaultmind index --embed --model bge-m3 --vault <vault>\n",
			emb.DenseCount-emb.SparseCount, emb.DenseCount-emb.ColBERTCount)
		return err
	}
	return nil
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

	result, err := query.Doctor(vdb.DB, vaultPath, vdb.Reg)
	if err != nil {
		return fmt.Errorf("running doctor: %w", err)
	}

	// Hook-drift detection — embedded canonical vs installed copies in
	// CWD's .claude/scripts/. Skipped silently if CWD is unavailable
	// (e.g. a deleted dir from under us); doctor is a health summary,
	// not a filesystem-error reporter. Populated here in cmd/ because
	// internal/query and internal/hooks are both business-layer per
	// ADR-009 and cannot depend on each other.
	if cwd, cwdErr := os.Getwd(); cwdErr == nil {
		drifted, driftErr := hooks.CompareInstalled(cwd)
		if driftErr == nil && len(drifted) > 0 {
			result.Issues.HookDrift = len(drifted)
			result.Issues.HookDriftDetails = drifted
		}
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
	summaryOnly := getConfigValueWithFlags[bool](cmd, "summary", config.KeyAppDoctorSummary)
	if result.Issues.ObsidianIncompatibleLinks > 0 {
		if _, err = fmt.Fprintf(w, "Obsidian-incompatible links: %d\n", result.Issues.ObsidianIncompatibleLinks); err != nil {
			return err
		}
		if summaryOnly {
			if _, err = fmt.Fprintln(w, "  (run without --summary to see per-link details, or pipe through scripts/fix_wikilinks.py)"); err != nil {
				return err
			}
		} else {
			for _, il := range result.Issues.IncompatibleLinkDetails {
				if _, err = fmt.Fprintf(w, "  %s: [[%s]] → [[%s|%s]]\n",
					il.SourcePath, il.TargetRaw, il.SuggestedFix, il.TargetRaw); err != nil {
					return err
				}
			}
		}
	}
	if result.Issues.PathPseudoIDLinks > 0 {
		if _, err = fmt.Fprintf(w, "Dead link references: %d\n", result.Issues.PathPseudoIDLinks); err != nil {
			return err
		}
		if summaryOnly {
			if _, err = fmt.Fprintln(w, "  (run without --summary to see per-link details)"); err != nil {
				return err
			}
		} else {
			for _, pl := range result.Issues.PathPseudoIDDetails {
				if _, err = fmt.Fprintf(w, "  %s: [[%s]] → target file does not exist\n",
					pl.SourcePath, pl.TargetRaw); err != nil {
					return err
				}
			}
		}
	}
	if err = writeHookDrift(w, &result.Issues, summaryOnly); err != nil {
		return err
	}
	if result.Issues.StaleIndex > 0 {
		if _, err = fmt.Fprintf(w,
			"⚠ Stale index: %d note(s) changed since last index pass\n"+
				"  run: vaultmind index --vault <vault>\n",
			result.Issues.StaleIndex); err != nil {
			return err
		}
		if !summaryOnly {
			for _, sv := range result.Issues.StaleIndexDetails {
				if _, err = fmt.Fprintf(w, "  %s: current_hash=%s stored_hash=%s\n",
					sv.Path, short(sv.CurrentHash), short(sv.StoredHash)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// writeHookDrift prints the hook-drift section to w. Extracted from
// runDoctor to keep its cyclomatic complexity under the 30 ceiling
// (gocyclo) — same shape as writeEmbeddingStatus.
func writeHookDrift(w io.Writer, issues *query.DoctorIssues, summaryOnly bool) error {
	if issues.HookDrift == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w,
		"⚠ Hook drift: %d hook script(s) differ from the embedded canonical\n"+
			"  run: vaultmind hooks install --force .\n",
		issues.HookDrift); err != nil {
		return err
	}
	if summaryOnly {
		return nil
	}
	for _, name := range issues.HookDriftDetails {
		if _, err := fmt.Fprintf(w, "  drifted: .claude/scripts/%s\n", name); err != nil {
			return err
		}
	}
	return nil
}

// short truncates a sha256 hex string to its first 8 chars for
// human-readable display. Full hashes are still in the JSON envelope
// for consumers that need exact comparison.
func short(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
