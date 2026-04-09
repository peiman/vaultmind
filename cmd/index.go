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
	embed := getConfigValueWithFlags[bool](cmd, "embed", config.KeyAppIndexEmbed)
	model := getConfigValueWithFlags[string](cmd, "model", config.KeyAppIndexModel)

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

	var embedResult *index.EmbedResult
	if embed {
		embedResult, err = idxr.RunEmbed(cmd.Context(), dbPath, model)
		if err != nil {
			return fmt.Errorf("embedding notes: %w", err)
		}
	}

	// Log experiment event (non-blocking)
	if session := experiment.FromContext(cmd.Context()); session != nil {
		session.VaultPath = vaultPath
		data := map[string]any{
			"full_rebuild": result.FullRebuild,
			"indexed":      result.Indexed,
			"added":        result.Added,
			"updated":      result.Updated,
			"deleted":      result.Deleted,
			"errors":       result.Errors,
		}
		if embed && embedResult != nil {
			data["model"] = model
			data["embedded"] = embedResult.Embedded
			data["embed_skipped"] = embedResult.Skipped
			data["embed_errors"] = embedResult.Errors
		}
		_, _ = session.DB.LogEvent(experiment.Event{
			SessionID: session.ID,
			Type:      experiment.EventIndexEmbed,
			VaultPath: vaultPath,
			Data:      data,
		})
	}

	combined := index.IndexAndEmbedResult{Index: result, Embed: embedResult}
	if jsonOut {
		env := envelope.OK("index", combined)
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	return formatIndexResult(combined, cmd.OutOrStdout())
}

func formatIndexResult(r index.IndexAndEmbedResult, w io.Writer) error {
	if r.Index.FullRebuild {
		if _, err := fmt.Fprintf(w, "Indexed %d notes (%d domain, %d unstructured, %d errors)\n",
			r.Index.Indexed, r.Index.DomainNotes, r.Index.UnstructuredNotes, r.Index.Errors); err != nil {
			return err
		}
	} else {
		total := r.Index.Skipped + r.Index.Updated + r.Index.Added
		if _, err := fmt.Fprintf(w, "Indexed %d notes (%d skipped, %d updated, %d added, %d deleted)\n",
			total, r.Index.Skipped, r.Index.Updated, r.Index.Added, r.Index.Deleted); err != nil {
			return err
		}
	}
	if r.Embed != nil {
		if _, err := fmt.Fprintf(w, "Embedded %d notes (%d skipped, %d errors)\n",
			r.Embed.Embedded, r.Embed.Skipped, r.Embed.Errors); err != nil {
			return err
		}
	}
	return nil
}
