package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
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

	info, err := os.Stat(vaultPath)
	if err != nil || !info.IsDir() {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "index", "vault_not_found",
				fmt.Sprintf("vault path %q does not exist or is not a directory", vaultPath))
		}
		return fmt.Errorf("vault path %q does not exist or is not a directory", vaultPath)
	}

	cfg, err := vault.LoadConfig(vaultPath)
	if err != nil {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "index", "config_error",
				fmt.Sprintf("loading config: %v", err))
		}
		return fmt.Errorf("loading config: %w", err)
	}

	dbPath := filepath.Join(vaultPath, cfg.Index.DBPath)
	idxr := index.NewIndexer(vaultPath, dbPath, cfg)

	var result *index.IndexResult
	if fullRebuild {
		result, err = idxr.Rebuild()
		if err != nil {
			return fmt.Errorf("rebuilding index: %w", err)
		}
		result.FullRebuild = true
	} else {
		result, err = idxr.Incremental()
		if err != nil {
			return fmt.Errorf("incremental index: %w", err)
		}
	}

	// Log index_embed experiment event (non-blocking)
	if session := experiment.FromContext(cmd.Context()); session != nil {
		session.VaultPath = vaultPath
		_, _ = session.DB.LogEvent(experiment.Event{
			SessionID: session.ID,
			Type:      experiment.EventIndexEmbed,
			VaultPath: vaultPath,
			Data: map[string]any{
				"full_rebuild": result.FullRebuild,
				"indexed":      result.Indexed,
				"added":        result.Added,
				"updated":      result.Updated,
				"skipped":      result.Skipped,
				"deleted":      result.Deleted,
				"errors":       result.Errors,
			},
		})
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
