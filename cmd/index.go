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
	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
)

// bgem3SlowPathWarning is the stderr banner that fires when `index --embed
// --model bge-m3` is invoked on a binary built without -tags ORT. Pure-Go
// hugot is documented as "hours for 130 notes"; an operator who sees
// 45 minutes of silent CPU burn on 8 notes thinks it's an OOM (I did —
// see the 2026-04-24/25 investigation). Surface the regime explicitly so
// the mistake costs seconds of reading, not 45 minutes of waiting.
const bgem3SlowPathWarning = `⚠ BGE-M3 embedding on the pure-Go hugot backend is very slow (hours for a medium vault).
  For fast indexing, rebuild with:
    task setup:ort && task build:ort
    /tmp/vaultmind-ort index --embed --model bge-m3 --vault <vault>
  To proceed on this binary anyway, re-run with --allow-slow-backend.
`

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
		if err := guardBGEM3SlowBackend(cmd, model); err != nil {
			return err
		}
		embedResult, err = idxr.RunEmbed(cmd.Context(), dbPath, model)
		if err != nil {
			return fmt.Errorf("embedding notes: %w", err)
		}
	}

	// Log experiment event (non-blocking)
	if session := experiment.FromContext(cmd.Context()); session != nil {
		session.SetVaultPath(vaultPath)
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

	return formatIndexResult(combined, model, cmd.OutOrStdout())
}

func formatIndexResult(r index.IndexAndEmbedResult, model string, w io.Writer) error {
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
		// Name the model in the human-readable output. Pure-Go fallback
		// silently producing minilm-only embeddings was the surprise the
		// 2026-05-04 onboarding dogfood surfaced — operators learned about
		// it only by running `doctor` afterward. Make it visible here so
		// the agent can tell the user "your build is on minilm; for BGE-M3
		// upgrade, run task setup:ort and rebuild" without a separate
		// inspection step.
		if _, err := fmt.Fprintf(w, "Embedded %d notes (%d skipped, %d errors) [model: %s]\n",
			r.Embed.Embedded, r.Embed.Skipped, r.Embed.Errors, model); err != nil {
			return err
		}
		// Surface empty-output count loudly when non-zero — these notes have
		// dense BGE-M3 but missing sparse/ColBERT and stay pending for the
		// next embed pass. Without this line the operator only learns about
		// the gap from `vaultmind doctor`'s post-hoc warning, after the
		// substrate is already partial. See vaultmind#22.
		if r.Embed.EmptyOutput > 0 {
			if _, err := fmt.Fprintf(w,
				"⚠ %d note(s) produced empty Sparse/ColBERT output and remain pending — see warn logs for IDs and body lengths.\n",
				r.Embed.EmptyOutput); err != nil {
				return err
			}
		}
	}
	return nil
}

// guardBGEM3SlowBackend prints a visible warning (and blocks unless the
// operator opts in) when BGE-M3 indexing would run on pure-Go hugot.
// Any other model path — or ORT-built binaries — is a no-op.
func guardBGEM3SlowBackend(cmd *cobra.Command, model string) error {
	if model != "bge-m3" || embedding.BackendName() == "ort" {
		return nil
	}
	allow := getConfigValueWithFlags[bool](cmd, "allow-slow-backend", config.KeyAppIndexAllowSlowBackend)
	if !allow {
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), bgem3SlowPathWarning)
		return fmt.Errorf("refusing to run BGE-M3 indexing on the pure-Go backend without --allow-slow-backend")
	}
	_, _ = fmt.Fprint(cmd.ErrOrStderr(), bgem3SlowPathWarning)
	return nil
}
