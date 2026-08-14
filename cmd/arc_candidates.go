package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/distill"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

// nearestArcsPerProposal is how many similar existing arcs to show per proposal.
// Three is the number the 2026-05-31 review specified: enough to reveal a near-tie
// (which is itself the signal that the judgement is hard) without turning the
// report into a ranking exercise.
const nearestArcsPerProposal = 3

var arcCandidatesCmd = MustNewCommand(commands.ArcCandidatesMetadata, runArcCandidates)

func init() {
	arcCmd.AddCommand(arcCandidatesCmd)
}

// runArcCandidates scans the vault for arc material and prints the propose-only
// report. It never writes arcs — the distill package has no arc writer.
//
// Two sources, because they carry different weight. <vault>/episodes holds
// session captures, which the detector phrase-matches for candidate moments —
// guesses worth checking. The vault's own journal notes are the desk: raw
// transformations the mind stopped mid-session to record, already judged worth
// keeping. Both are scanned from whichever vault is named, so pointing at an
// identity vault reads its episodes and pointing at a desk vault reads its
// entries, with no extra flag to know about.
func runArcCandidates(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppArcCandidatesVault)
	// This command scans directories directly instead of opening the vault DB,
	// so nothing else checks the path. Without this, a typo'd --vault produced
	// "Scanned 0 episodes → 0 candidate moments" and exit 0 — indistinguishable
	// from a real vault with nothing pending, which is the opposite instruction
	// to the reader.
	if info, err := os.Stat(vaultPath); err != nil || !info.IsDir() {
		return fmt.Errorf("vault %q does not exist or is not a directory", vaultPath)
	}
	report, err := distill.ScanEpisodes(filepath.Join(vaultPath, "episodes"))
	if err != nil {
		return err
	}
	// A desk scan failure must not lose the episode candidates already found:
	// report what it has and surface the reason alongside the parse errors.
	desk, deskErr := distill.ScanDesk(vaultPath)
	if deskErr != nil {
		report.ParseErrors = append(report.ParseErrors, deskErr.Error())
	}
	report.DeskPending = desk

	// De-duplication: annotate every proposal with the existing arcs it
	// resembles. The 2026-05-31 review named mis-attribution — not extraction —
	// the biggest risk in this pipeline, so the tool finds the neighbours and
	// leaves the covered/new verdict to the reader. A vault with no usable index
	// simply skips the aid rather than losing the proposals.
	// Arcs need not live in the vault being scanned. A desk is its own vault
	// (raw, ungated) while arcs live in the curated identity vault, so without
	// this the de-duplication would find nothing in exactly the setup that needs
	// it most — the aid would be inert precisely where proposals originate.
	arcsVault := getConfigValueWithFlags[string](cmd, "arcs-vault", config.KeyAppArcCandidatesArcsVault)
	if strings.TrimSpace(arcsVault) == "" {
		arcsVault = vaultPath
	}
	if finder, closeFn, ferr := openArcFinder(arcsVault); ferr == nil {
		defer closeFn()
		report = distill.AnnotateNearestArcs(cmd.Context(), report, finder, nearestArcsPerProposal)
		report = annotateRecurrences(cmd.Context(), report, finder)
	} else {
		report.ParseErrors = append(report.ParseErrors,
			"nearest-arc de-duplication unavailable (vault not indexed?): "+ferr.Error())
	}
	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppArcCandidatesJson) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(envelope.OK("arc-candidates", report))
	}
	return distill.FormatReport(report, cmd.OutOrStdout())
}

// arcFinderAdapter bridges query.ArcFinder to the distill.ArcFinder interface.
// The two types are kept apart on purpose: distill is a pure package with no
// retrieval dependency, and query must not depend on distill (both are business
// logic — ADR-009). cmd is the layer allowed to know about both, so the seam
// lives here, where wiring belongs.
type arcFinderAdapter struct{ inner *query.ArcFinder }

func (a arcFinderAdapter) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	return a.inner.EmbedTexts(ctx, texts)
}

func (a arcFinderAdapter) NearestArcs(ctx context.Context, text string, limit int) ([]distill.NearArc, error) {
	matches, err := a.inner.NearestArcs(ctx, text, limit)
	if err != nil {
		return nil, err
	}
	out := make([]distill.NearArc, 0, len(matches))
	for _, m := range matches {
		out = append(out, distill.NearArc{ID: m.ID, Title: m.Title, Score: m.Score})
	}
	return out, nil
}

// openArcFinder opens the arcs vault and builds a finder over it, returning the
// finder plus the closer for the resources it borrows.
func openArcFinder(arcsVault string) (distill.ArcFinder, func(), error) {
	vdb, err := cmdutil.OpenVaultDB(arcsVault)
	if err != nil {
		return nil, nil, err
	}
	ret := query.BuildAutoRetrieverFull(vdb.DB)
	finder, err := query.NewArcFinder(vdb.DB, ret.Embedder)
	if err != nil {
		ret.Cleanup()
		vdb.Close()
		return nil, nil, err
	}
	return arcFinderAdapter{inner: finder}, func() { ret.Cleanup(); vdb.Close() }, nil
}

// minRecurrenceSources is how many distinct sources a shape must cross before
// it is reported. Two is the smallest number that can mean "again" — the whole
// point of Rule 2 is crossing a session boundary, which one source cannot do.
const minRecurrenceSources = 2

// annotateRecurrences runs Rule 2 over every proposal in the report: the desk
// entries and the phrase-matched moments together, since a shape that recurs
// across BOTH is the strongest structural signal available.
//
// A failure degrades to no recurrences and is recorded — the proposals matter
// more than the analysis over them.
func annotateRecurrences(ctx context.Context, report distill.Report, finder distill.ArcFinder) distill.Report {
	vz, ok := finder.(distill.Vectorizer)
	if !ok {
		return report
	}
	items := make([]distill.RecurrenceItem, 0, len(report.DeskPending)+len(report.Candidates))
	for _, e := range report.DeskPending {
		if text := firstNonEmpty(e.Snippet, e.Title); text != "" {
			items = append(items, distill.RecurrenceItem{Source: e.ID, Text: text})
		}
	}
	for _, c := range report.Candidates {
		if text := strings.TrimSpace(c.Verbatim); text != "" {
			items = append(items, distill.RecurrenceItem{Source: c.EpisodeID, Text: text, Trigger: c.Trigger})
		}
	}

	groups, err := distill.FindRecurrences(ctx, items, vz, minRecurrenceSources)
	if err != nil {
		report.ParseErrors = append(report.ParseErrors, "recurrence detection failed: "+err.Error())
		return report
	}
	report.Recurrences = groups
	return report
}

func firstNonEmpty(candidates ...string) string {
	for _, c := range candidates {
		if s := strings.TrimSpace(c); s != "" {
			return s
		}
	}
	return ""
}
