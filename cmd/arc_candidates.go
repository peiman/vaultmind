package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/distill"
	"github.com/peiman/vaultmind/internal/envelope"
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
	// A desk scan failure must not lose the episode candidates already found:
	// report what it has and surface the reason alongside the parse errors.
	desk, deskErr := distill.ScanDesk(vaultPath)
	if deskErr != nil {
		report.ParseErrors = append(report.ParseErrors, deskErr.Error())
	}
	report.DeskPending = desk
	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppArcCandidatesJson) {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(envelope.OK("arc-candidates", report))
	}
	return distill.FormatReport(report, cmd.OutOrStdout())
}
