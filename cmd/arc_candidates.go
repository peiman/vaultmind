package cmd

import (
	"context"
	"encoding/json"
	"errors"
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
	// failures are the diagnostics that represent something BROKEN, as opposed
	// to an optional capability simply not being configured. Only these set the
	// envelope to "warning".
	var failures []string
	// A desk scan failure must not lose the episode candidates already found:
	// report what it has and surface the reason alongside the parse errors.
	desk, deskDiags, deskErr := distill.ScanDesk(vaultPath)
	if deskErr != nil {
		report.Diagnostics = append(report.Diagnostics, deskErr.Error())
		failures = append(failures, deskErr.Error()) // an unreadable desk IS a fault
	}
	report.Diagnostics = append(report.Diagnostics, deskDiags...)
	failures = append(failures, deskDiags...) // an unparseable entry is a fault too
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
		report = distill.AnnotateNearestArcs(cmd.Context(), report, finder, query.DefaultNearestArcs)
	} else {
		report.Diagnostics = append(report.Diagnostics,
			"nearest-arc de-duplication unavailable: "+ferr.Error())
		// An unembedded vault is a CONFIGURATION, not a fault: the reader is
		// told the aid did not run (above), but the run itself is not degraded.
		// Anything else — a path that isn't a vault, an unreadable index — is a
		// real failure and must reach a caller gating on status.
		if !errors.Is(ferr, query.ErrNoEmbedder) {
			failures = append(failures, ferr.Error())
		}
	}
	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppArcCandidatesJson) {
		env := envelope.OK("arc-candidates", report)
		// A degraded run must not report unqualified success. v0.3.0's headline
		// fix was a command answering from the wrong vault while saying
		// status "ok"; a caller gating on status must see that the de-duplication
		// aid was off or the desk went unread.
		for _, f := range failures {
			env.AddWarning("degraded", f, "")
		}
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}
	return distill.FormatReport(report, cmd.OutOrStdout())
}

// arcFinderAdapter bridges query.ArcFinder to the distill.ArcFinder interface.
// The two types are kept apart on purpose: distill is a pure package with no
// retrieval dependency, and query must not depend on distill (both are business
// logic — ADR-009). cmd is the layer allowed to know about both, so the seam
// lives here, where wiring belongs.
type arcFinderAdapter struct{ inner *query.ArcFinder }

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
	// The GUARDED opener, not the raw one. cmdutil.OpenVaultDB CREATES
	// .vaultmind/index.db under whatever path it is handed — the self-propagating
	// mistake v0.3.0 fixed in the read path. Using it here reintroduced it: a
	// scan pointed at a plain directory quietly turned that directory into a
	// vault every later walk-up would find. A propose-only reader must not
	// write a database anywhere.
	if info, statErr := os.Stat(arcsVault); statErr != nil || !info.IsDir() {
		return nil, nil, fmt.Errorf("arcs vault %q does not exist or is not a directory", arcsVault)
	}
	if _, statErr := os.Stat(filepath.Join(arcsVault, ".vaultmind")); statErr != nil {
		return nil, nil, fmt.Errorf("arcs vault %q is not a vault (no .vaultmind/ directory)", arcsVault)
	}
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
