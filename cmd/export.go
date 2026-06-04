package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/telemetry"
	"github.com/spf13/cobra"
)

var exportCmd = MustNewCommand(commands.ExportMetadata, runExport)

func init() {
	MustAddToRoot(exportCmd)
}

func runExport(cmd *cobra.Command, _ []string) error {
	tier := getConfigValueWithFlags[string](cmd, "tier", config.KeyAppExportTier)
	if tier == "" {
		tier = getConfigValueWithFlags[string](cmd, "", config.KeyExperimentsTelemetry)
	}
	if tier == "" {
		return fmt.Errorf("no telemetry tier set — run vaultmind interactively once to opt in, or pass --tier")
	}

	expDB, err := openExperimentDB()
	if err != nil {
		return fmt.Errorf("open experiment db: %w", err)
	}
	defer func() { _ = expDB.Close() }()

	w, closer, err := openExportWriter(cmd, getConfigValueWithFlags[string](cmd, "output", config.KeyAppExportOutput))
	if err != nil {
		return err
	}
	defer closer()

	rollup := getConfigValueWithFlags[bool](cmd, "rollup", config.KeyAppExportRollup)
	preview := getConfigValueWithFlags[bool](cmd, "preview", config.KeyAppExportPreview)

	if !rollup {
		if preview {
			return previewRawExport(expDB, tier, w)
		}
		return experiment.ExportToJSONL(expDB, tier, w)
	}

	r, err := buildRollup(cmd, expDB, tier)
	if err != nil {
		return err
	}
	if preview {
		return previewRollup(r, w)
	}
	return experiment.WriteRollup(r, w)
}

func openExportWriter(cmd *cobra.Command, path string) (io.Writer, func(), error) {
	if path == "" {
		return cmd.OutOrStdout(), func() {}, nil
	}
	f, err := os.Create(path) // #nosec G304 -- path is from --output flag (operator-controlled)
	if err != nil {
		return nil, nil, fmt.Errorf("create output file: %w", err)
	}
	return f, func() { _ = f.Close() }, nil
}

func buildRollup(cmd *cobra.Command, expDB *experiment.DB, tier string) (*experiment.Rollup, error) {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppExportVault)
	cleanVault := filepath.Clean(vaultPath)

	fp, err := telemetry.EnsureFingerprint(cleanVault)
	if err != nil {
		return nil, fmt.Errorf("vault fingerprint: %w", err)
	}
	feats, err := telemetry.ComputeFeatures(filepath.Join(cleanVault, ".vaultmind", "index.db"))
	if err != nil {
		return nil, fmt.Errorf("vault features: %w", err)
	}
	variantStats, err := experiment.VariantPerformance(expDB)
	if err != nil {
		return nil, fmt.Errorf("variant performance: %w", err)
	}
	sessionCount, err := expDB.CountSessions()
	if err != nil {
		return nil, fmt.Errorf("session count: %w", err)
	}
	eventCount, err := expDB.CountEvents()
	if err != nil {
		return nil, fmt.Errorf("event count: %w", err)
	}
	outcomeCount, err := expDB.CountOutcomes()
	if err != nil {
		return nil, fmt.Errorf("outcome count: %w", err)
	}

	return &experiment.Rollup{
		Kind:             "rollup",
		SchemaVersion:    1,
		Tier:             tier,
		Fingerprint:      fp,
		NoteCount:        feats.NoteCount,
		TypeDistribution: feats.TypeDistribution,
		LinkCount:        feats.LinkCount,
		AliasCount:       feats.AliasCount,
		EmbeddingCount:   feats.EmbeddingCount,
		EmbeddingDims:    feats.EmbeddingDims,
		VariantStats:     variantStats,
		ExportedAt:       nowFormatted(),
		SessionCount:     sessionCount,
		EventCount:       eventCount,
		OutcomeCount:     outcomeCount,
	}, nil
}

func nowFormatted() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// previewRollup writes a human-readable summary of the rollup payload
// — what data would be transmitted, in plain text, so the user can
// audit before sharing. No JSON, just a categorized listing.
func previewRollup(r *experiment.Rollup, w io.Writer) error {
	_, _ = fmt.Fprintln(w, "VaultMind telemetry rollup — what would be shared:")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintf(w, "  tier:              %s\n", r.Tier)
	_, _ = fmt.Fprintf(w, "  vault fingerprint: %s\n", r.Fingerprint)
	_, _ = fmt.Fprintf(w, "  exported at:       %s\n", r.ExportedAt)
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Vault features (no content; counts only):")
	_, _ = fmt.Fprintf(w, "  notes:        %d\n", r.NoteCount)
	_, _ = fmt.Fprintf(w, "  by type:      %s\n", formatTypeDist(r.TypeDistribution))
	_, _ = fmt.Fprintf(w, "  links:        %d\n", r.LinkCount)
	_, _ = fmt.Fprintf(w, "  aliases:      %d\n", r.AliasCount)
	_, _ = fmt.Fprintf(w, "  embeddings:   %d (%d dims)\n", r.EmbeddingCount, r.EmbeddingDims)
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Telemetry coverage:")
	_, _ = fmt.Fprintf(w, "  sessions:     %d\n", r.SessionCount)
	_, _ = fmt.Fprintf(w, "  events:       %d\n", r.EventCount)
	_, _ = fmt.Fprintf(w, "  outcomes:     %d\n", r.OutcomeCount)
	_, _ = fmt.Fprintln(w, "")
	if len(r.VariantStats) == 0 {
		_, _ = fmt.Fprintln(w, "Variant performance: (none — primary_variant never set)")
	} else {
		_, _ = fmt.Fprintln(w, "Variant performance (per primary_variant):")
		names := make([]string, 0, len(r.VariantStats))
		for k := range r.VariantStats {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, n := range names {
			s := r.VariantStats[n]
			_, _ = fmt.Fprintf(w, "  %-18s events=%d  outcomes=%d  Hit@5=%.3f  Hit@10=%.3f  MRR=%.3f\n",
				n, s.EventCount, s.OutcomeCount, s.HitAt5, s.HitAt10, s.MRR)
		}
	}
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "What is NOT transmitted: vault path, note IDs, note titles, note bodies, query text, file paths.")
	_, _ = fmt.Fprintln(w, "Run without --preview to emit this as JSON.")
	return nil
}

func formatTypeDist(m map[string]int) string {
	if len(m) == 0 {
		return "(empty)"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}

// previewRawExport produces a human-readable summary for the raw-event
// export path. Same intent as previewRollup but for the larger payload.
func previewRawExport(expDB *experiment.DB, tier string, w io.Writer) error {
	sessions, err := expDB.CountSessions()
	if err != nil {
		return err
	}
	events, err := expDB.CountEvents()
	if err != nil {
		return err
	}
	outcomes, err := expDB.CountOutcomes()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w, "VaultMind telemetry raw export — what would be shared:")
	_, _ = fmt.Fprintf(w, "  tier:     %s\n", tier)
	_, _ = fmt.Fprintf(w, "  sessions: %d\n", sessions)
	_, _ = fmt.Fprintf(w, "  events:   %d\n", events)
	_, _ = fmt.Fprintf(w, "  outcomes: %d\n", outcomes)
	_, _ = fmt.Fprintln(w, "")
	switch tier {
	case experiment.TelemetryAnonymous:
		_, _ = fmt.Fprintln(w, "Anonymous tier strips: vault paths, query text, result note_ids, result paths, caller_meta.")
		_, _ = fmt.Fprintln(w, "Anonymous tier preserves: ranks, scores, variant names, timestamps, type distributions.")
	case experiment.TelemetryFull:
		_, _ = fmt.Fprintln(w, "Full tier preserves: everything — including vault paths, query text, result note_ids.")
		_, _ = fmt.Fprintln(w, "Use Full tier ONLY if you've explicitly opted in to full data sharing.")
	}
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Run without --preview to emit JSONL records to stdout (or to --output FILE).")
	_, _ = fmt.Fprintln(w, "Run with --rollup to emit the smaller, federated-aggregator-shaped payload instead.")
	return nil
}
