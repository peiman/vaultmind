package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
)

var indexCmd = MustNewCommand(commands.IndexMetadata, runIndex)

func init() {
	MustAddToRoot(indexCmd)
}

func runIndex(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppIndexVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppIndexJson)
	fullRebuild := getConfigValueWithFlags[bool](cmd, "full", config.KeyAppIndexFull)

	cfg, err := vault.LoadConfig(vaultPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	dbPath := filepath.Join(vaultPath, cfg.Index.DBPath)
	idxr := index.NewIndexer(vaultPath, dbPath, cfg)

	var result *index.IndexResult
	if fullRebuild {
		result, err = idxr.Rebuild()
		result.FullRebuild = true
	} else {
		result, err = idxr.Incremental()
	}
	if err != nil {
		return fmt.Errorf("indexing: %w", err)
	}

	if jsonOut {
		env := envelope.OK("index", result)
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	return formatIndexResult(result, cmd.OutOrStdout())
}

func formatIndexResult(result *index.IndexResult, w io.Writer) error {
	if result.FullRebuild {
		_, err := fmt.Fprintf(w, "Indexed %d notes (%d domain, %d unstructured, %d errors)\n",
			result.Indexed, result.DomainNotes, result.UnstructuredNotes, result.Errors)
		return err
	}
	total := result.Skipped + result.Updated + result.Added
	_, err := fmt.Fprintf(w, "Indexed %d notes (%d skipped, %d updated, %d added, %d deleted)\n",
		total, result.Skipped, result.Updated, result.Added, result.Deleted)
	return err
}
