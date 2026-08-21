package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// TestWriteCommandsMarkdown_InjectsIntoMarkedRegion pins the SSOT behaviour for
// hand-written docs that embed the generated command catalog.
//
// docs/AGENT_USAGE.md is prose an agent reads first, and it restated flag
// semantics that the Cobra tree already owns — v0.7.0 shipped --excerpt with the
// flag help as the only source of truth and the guide silent about it. Prose that
// duplicates a registry drifts from it; the fix is to let the generator own the
// reference block and leave the surrounding explanation to a human.
//
// So: when the target file already carries a VAULTMIND:GENERATED:commands region,
// only that region is replaced. A file without markers keeps the old
// whole-file-overwrite behaviour, because that is what internal/onboard/COMMANDS.md
// is and must stay.
func TestWriteCommandsMarkdown_InjectsIntoMarkedRegion(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "GUIDE.md")

	original := strings.Join([]string{
		"# Guide",
		"",
		"Hand-written intro that must survive.",
		"",
		"<!-- VAULTMIND:GENERATED:commands:START -->",
		"stale catalog that must be replaced",
		"<!-- VAULTMIND:GENERATED:commands:END -->",
		"",
		"Hand-written outro that must survive.",
		"",
	}, "\n")
	require.NoError(t, os.WriteFile(target, []byte(original), 0o644))

	require.NoError(t, writeCommandsMarkdown(&cobra.Command{}, "FRESH CATALOG\n", target))

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	out := string(got)

	require.Contains(t, out, "Hand-written intro that must survive.")
	require.Contains(t, out, "Hand-written outro that must survive.")
	require.Contains(t, out, "FRESH CATALOG")
	require.NotContains(t, out, "stale catalog that must be replaced")
}

// TestWriteCommandsMarkdown_UnmarkedFileIsOverwritten guards the other half:
// internal/onboard/COMMANDS.md is wholly generated and has no markers, so it must
// keep being replaced outright rather than erroring on a missing region.
func TestWriteCommandsMarkdown_UnmarkedFileIsOverwritten(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "COMMANDS.md")
	require.NoError(t, os.WriteFile(target, []byte("old whole-file content\n"), 0o644))

	require.NoError(t, writeCommandsMarkdown(&cobra.Command{}, "NEW WHOLE FILE\n", target))

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "NEW WHOLE FILE\n", string(got))
}
