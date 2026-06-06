package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

// embeddingBackendsDocURL points operators at the backend adoption guide when
// doctor surfaces a degraded (MiniLM) index. One literal so the WARN and any
// future caller never drift on where the upgrade path is documented.
const embeddingBackendsDocURL = "https://github.com/peiman/vaultmind/blob/main/docs/embedding-backends.md"

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
		// Backend-agnostic remedy: `--embed` picks the binary's default model
		// (bge-m3 on ORT, minilm on pure-Go). Naming bge-m3 here would be
		// refused on the pure-Go binary `go install` yields. The bge-m3-specific
		// remedy below (modality imbalance) is reachable only on an ORT binary —
		// the minilm branch returns before it.
		_, err := fmt.Fprintf(w,
			"Embeddings: none (%d notes) — keyword-only retrieval\n"+
				"  run: vaultmind index --embed --vault <vault>\n",
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
	if emb.Model == "minilm" {
		// First-class degraded-recall WARN. A MiniLM index runs 2-lane recall
		// (full-text + dense) and silently lacks BGE-M3's sparse + ColBERT
		// lanes. `go install` yields a pure-Go/MiniLM binary, so an adopter
		// can land here without ever being told their recall is degraded
		// (focalc field report, P1). Name the cliff at the moment it matters.
		_, err := fmt.Fprintf(w,
			"⚠ degraded recall: this vault is indexed with MiniLM — dense-only "+
				"(2 lanes: full-text + dense). For the full 4-way BGE-M3 hybrid "+
				"(dense + sparse + ColBERT): on darwin-arm64/linux-amd64 download the "+
				"prebuilt ORT binary from the release (no build), or build from source — see:\n  %s\n",
			embeddingBackendsDocURL)
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
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppDoctorJson)
	summaryOnly := getConfigValueWithFlags[bool](cmd, "summary", config.KeyAppDoctorSummary)
	return runDoctorCore(cmd, vaultPath, jsonOut, summaryOnly)
}

// runDoctorCore is the doctor health-hub engine, shared by `doctor`,
// `doctor --summary`, and the deprecated `vault status` alias (which forces
// summaryOnly=true). It opens the vault, runs the read-only diagnosis, folds
// in the per-type breakdown + errors/warnings rollup (the merged-in
// `vault status` view, computed via the shared status.go helpers for SSOT),
// then renders JSON or the human report.
func runDoctorCore(cmd *cobra.Command, vaultPath string, jsonOut, summaryOnly bool) error {
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

	// Fold in the per-type breakdown + errors/warnings rollup that `vault
	// status` used to produce. Populated here in cmd/ because the vault config
	// and schema registry live on vdb — the same reason HookDrift is populated
	// in this layer. Both come from internal/query/status.go (SSOT).
	if result.Types, err = query.CollectTypeBreakdown(vdb.DB, vdb.Config); err != nil {
		return fmt.Errorf("collecting type breakdown: %w", err)
	}
	if result.IssuesSummary, err = query.SummarizeValidationIssues(vdb.DB, vdb.Reg); err != nil {
		return fmt.Errorf("summarizing validation issues: %w", err)
	}

	// Hook-drift detection — embedded canonical vs installed copies in
	// the project's .claude/scripts/. Resolve the project root by
	// walking up from CWD until a .claude/ dir is found (same heuristic
	// Claude Code uses for CLAUDE_PROJECT_DIR). Without walk-up, the
	// check would silently miss drift whenever the user runs `doctor`
	// from a subdirectory. Skipped silently if no project root resolves
	// or filesystem reads fail; doctor is a health summary, not a
	// filesystem-error reporter. Populated here in cmd/ because
	// internal/query and internal/hooks are both business-layer per
	// ADR-009 and cannot depend on each other.
	if projectDir, ok := findProjectRoot(); ok {
		drifted, driftErr := hooks.CompareInstalled(projectDir)
		if driftErr == nil && len(drifted) > 0 {
			result.Issues.HookDrift = len(drifted)
			result.Issues.HookDriftDetails = drifted
		}
		// Legacy `.claude/hooks.json` detection — silent-breakage
		// shape on Claude Code 2.1.129+. Populated alongside drift
		// because they share the same project-root resolution.
		result.Issues.LegacyHooksJSON = hooks.DetectLegacyHooksJSON(projectDir)
	}

	if jsonOut {
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
	if err = writeTypeBreakdown(w, result.Types); err != nil {
		return err
	}
	if err = writeEmbeddingStatus(w, result.Embeddings); err != nil {
		return err
	}
	if err = writeLinkIssues(w, &result.Issues, summaryOnly); err != nil {
		return err
	}
	if err = writeHookDrift(w, &result.Issues, summaryOnly); err != nil {
		return err
	}
	if err = writeLegacyHooksJSON(w, &result.Issues); err != nil {
		return err
	}
	if err = writeStaleIndex(w, &result.Issues, summaryOnly); err != nil {
		return err
	}
	// Errors/warnings rollup — the cold-start signal `vault status` used to
	// emit. Always printed so the operator gets the validation bottom line
	// from a single `doctor` run.
	if _, err = fmt.Fprintf(w, "Issues: %d errors, %d warnings\n",
		result.IssuesSummary.Errors, result.IssuesSummary.Warnings); err != nil {
		return err
	}
	return nil
}

// writeLinkIssues prints the Obsidian-incompatible and dead-link sections.
// Extracted from runDoctorCore to keep its cyclomatic complexity under the 30
// ceiling (gocyclo) — same shape as writeHookDrift. Under --summary the
// per-link detail lines are suppressed and replaced with a one-line pointer.
func writeLinkIssues(w io.Writer, issues *query.DoctorIssues, summaryOnly bool) error {
	if issues.ObsidianIncompatibleLinks > 0 {
		if _, err := fmt.Fprintf(w, "Obsidian-incompatible links: %d\n", issues.ObsidianIncompatibleLinks); err != nil {
			return err
		}
		if summaryOnly {
			if _, err := fmt.Fprintln(w, "  (run without --summary to see per-link details; fix with: vaultmind doctor heal wikilinks)"); err != nil {
				return err
			}
		} else {
			for _, il := range issues.IncompatibleLinkDetails {
				if _, err := fmt.Fprintf(w, "  %s: [[%s]] → [[%s|%s]]\n",
					il.SourcePath, il.TargetRaw, il.SuggestedFix, il.TargetRaw); err != nil {
					return err
				}
			}
		}
	}
	if issues.PathPseudoIDLinks == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w, "Dead link references: %d\n", issues.PathPseudoIDLinks); err != nil {
		return err
	}
	if summaryOnly {
		_, err := fmt.Fprintln(w, "  (run without --summary to see per-link details)")
		return err
	}
	for _, pl := range issues.PathPseudoIDDetails {
		if _, err := fmt.Fprintf(w, "  %s: [[%s]] → target file does not exist\n",
			pl.SourcePath, pl.TargetRaw); err != nil {
			return err
		}
	}
	return nil
}

// writeStaleIndex prints the stale-index drift section. Extracted from
// runDoctorCore for the same gocyclo reason. Per-note hash details are
// suppressed under --summary.
func writeStaleIndex(w io.Writer, issues *query.DoctorIssues, summaryOnly bool) error {
	if issues.StaleIndex == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(w,
		"⚠ Stale index: %d note(s) changed since last index pass\n"+
			"  run: vaultmind index --vault <vault>\n",
		issues.StaleIndex); err != nil {
		return err
	}
	if summaryOnly {
		return nil
	}
	for _, sv := range issues.StaleIndexDetails {
		if _, err := fmt.Fprintf(w, "  %s: current_hash=%s stored_hash=%s\n",
			sv.Path, short(sv.CurrentHash), short(sv.StoredHash)); err != nil {
			return err
		}
	}
	return nil
}

// writeTypeBreakdown prints the per-type note counts (with each type's valid
// statuses when defined) that `vault status` used to produce — now folded
// into the doctor health hub. Types are printed in sorted order so the output
// is deterministic. Nothing is printed when the vault has no registered types.
func writeTypeBreakdown(w io.Writer, types map[string]query.StatusTypeInfo) error {
	if len(types) == 0 {
		return nil
	}
	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)
	if _, err := fmt.Fprintf(w, "Types: %d\n", len(types)); err != nil {
		return err
	}
	for _, name := range names {
		ti := types[name]
		if len(ti.Statuses) > 0 {
			if _, err := fmt.Fprintf(w, "  %s: %d note(s) [statuses: %s]\n",
				name, ti.Count, strings.Join(ti.Statuses, ", ")); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(w, "  %s: %d note(s)\n", name, ti.Count); err != nil {
			return err
		}
	}
	return nil
}

// findProjectRoot walks up from CWD looking for a directory that
// contains a `.claude/` subdir — that's the project root from
// Claude Code's perspective (matches CLAUDE_PROJECT_DIR resolution).
// Returns ok=false if CWD is unavailable or no .claude/ ancestor
// exists; doctor surfaces drift only when there's something to check.
func findProjectRoot() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if info, statErr := os.Stat(filepath.Join(dir, ".claude")); statErr == nil && info.IsDir() {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
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

// writeLegacyHooksJSON warns when `.claude/hooks.json` exists at the
// project root. That standalone file is silently broken on Claude
// Code 2.1.129+; the resolution is migration into settings.json.
// No --summary suppression — the warning is one line, always relevant.
func writeLegacyHooksJSON(w io.Writer, issues *query.DoctorIssues) error {
	if !issues.LegacyHooksJSON {
		return nil
	}
	_, err := fmt.Fprintf(w,
		"⚠ Legacy hooks.json: .claude/hooks.json exists but is silently ignored on Claude Code 2.1.129+\n"+
			"  fix: merge its contents into .claude/settings.json under a top-level `hooks` key, then delete hooks.json\n",
	)
	return err
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
