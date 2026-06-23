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
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// bgem3SlowPathWarning is the stderr banner that fires when `index --embed
// --model bge-m3` is invoked on a binary built without -tags ORT. Pure-Go
// hugot is documented as "hours for 130 notes"; an operator who sees
// 45 minutes of silent CPU burn on 8 notes thinks it's an OOM (I did —
// see the 2026-04-24/25 investigation). Surface the regime explicitly so
// the mistake costs seconds of reading, not 45 minutes of waiting.
const bgem3SlowPathWarning = `⚠ BGE-M3 embedding on the pure-Go hugot backend is very slow (hours for a medium vault).
  Fastest fix on darwin-arm64 / linux-amd64: download the prebuilt ORT archive
    (vaultmind_<version>_<os>_<arch>_ort.tar.gz) from the GitHub release — full
    hybrid, no build (libonnxruntime bundled).
  Or build ORT from source:
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
	// Empty model means "auto" — pick the right default for the
	// binary's backend. Adapts to ORT-tagged vs pure-Go builds so a
	// user running `vaultmind index --embed` on an ORT-capable build
	// gets bge-m3 (the system's recommended path) instead of silently
	// degrading to minilm. The 2026-05-05 onboarding/companion-project dogfood
	// surfaced that the prior hardcoded "minilm" default contradicted
	// the README's framing.
	if model == "" {
		model = embedding.DefaultModel()
	}

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

	// The model ACTUALLY used may differ from the request: RunEmbed falls back
	// from bge-m3 to minilm when bge-m3 can't load. Stamp, calibrate, and report
	// against reality so the noise floor matches the vectors on disk.
	effectiveModel := model
	if embedResult != nil && embedResult.Model != "" {
		effectiveModel = embedResult.Model
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
			data["model"] = effectiveModel
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

		// Per-vault noise-floor calibration: measure N + dispersion from the
		// fresh embeddings and store a content-free snapshot. Best-effort —
		// never fail the index run over it.
		if embed && embedResult != nil {
			if err := calibrateVaultNoiseFloor(cmd.Context(), vaultPath, dbPath, effectiveModel, session.DB, embedResult.Embedded, result.Deleted); err != nil {
				// Non-fatal, but visible: a persistent failure (e.g. a mixed-model
				// vault) means `ask` falls back to the shipped per-embedder default
				// floor instead of this vault's measured one, silently degrading
				// confidence accuracy. Warn so it surfaces at the default log level.
				log.Warn().Err(err).Msg("noise-floor calibration skipped (non-fatal); ask will use the default floor")
			}
		}
	}

	if embedResult != nil {
		// Stamp the model on the result so JSON consumers see it
		// alongside Embedded/Skipped/Errors. Same purpose as
		// `[model: <name>]` in the human-readable line: an agent
		// running `vaultmind index --embed --json` should not have
		// to call doctor separately to learn which embedding path
		// the run used. RunEmbed already set this to the model actually
		// used (after any bge-m3→minilm fallback); re-affirm for the
		// pending==0 path where the field equals the request.
		embedResult.Model = effectiveModel
	}
	combined := index.IndexAndEmbedResult{Index: result, Embed: embedResult}
	if jsonOut {
		env := envelope.OK("index", combined)
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	return formatIndexResult(combined, effectiveModel, cmd.OutOrStdout())
}

func formatIndexResult(r index.IndexAndEmbedResult, model string, w io.Writer) error {
	if r.Index.FullRebuild {
		// Report deleted too: a full rebuild now purges notes no longer in the
		// include set (files removed from disk or newly excluded). Hiding the
		// count was the misleading "0 deleted" signal flagged in issue #40 —
		// the operator needs to see the purge to trust the rebuilt index.
		if _, err := fmt.Fprintf(w, "Indexed %d notes (%d domain, %d unstructured, %d deleted, %d errors)\n",
			r.Index.Indexed, r.Index.DomainNotes, r.Index.UnstructuredNotes, r.Index.Deleted, r.Index.Errors); err != nil {
			return err
		}
	} else {
		total := r.Index.Skipped + r.Index.Updated + r.Index.Added
		if _, err := fmt.Fprintf(w, "Indexed %d notes (%d skipped, %d updated, %d added, %d deleted)\n",
			total, r.Index.Skipped, r.Index.Updated, r.Index.Added, r.Index.Deleted); err != nil {
			return err
		}
	}
	// Surface files that were skipped (read/parse errors) with their cause — a
	// note dropped from the index must never vanish silently (vaultmind#40: an
	// unquoted colon in a YAML title made the frontmatter unparseable and the
	// note disappeared with no visible message; the non-full output didn't even
	// show the error count). Name the count AND each file's reason here.
	if r.Index.Errors > 0 {
		if _, err := fmt.Fprintf(w, "⚠ %d file(s) skipped — not indexed:\n", r.Index.Errors); err != nil {
			return err
		}
		for _, e := range r.Index.ErrorDetails {
			if _, err := fmt.Fprintf(w, "  - %s (%s): %s\n", e.Path, e.Kind, e.Error); err != nil {
				return err
			}
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
		if model == "minilm" {
			// `go install` (and onboarding scripts that wrap it) silently land on
			// the pure-Go MiniLM build — dense-only, no sparse/ColBERT. An adopter
			// who never runs `doctor` never learns the full hybrid exists, or that
			// it needs no compile. Name the lane gap and the no-compile upgrade at
			// the moment of embedding (focalc/Patrik field report, 2026-06-04).
			if _, err := fmt.Fprint(w,
				"  ↳ MiniLM is dense-only (2 lanes: full-text + dense). For BGE-M3's full "+
					"4-way hybrid (+ sparse + ColBERT), download the prebuilt ORT archive — no "+
					"compile — from https://github.com/peiman/vaultmind/releases (see "+
					"docs/embedding-backends.md).\n"); err != nil {
				return err
			}
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
