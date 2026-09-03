package hookscripts_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// dabir's layer-2 incident (2026-09-03): a note was corrected on disk while
// the recall index kept serving the OLD text for hours. `vaultmind doctor` has
// detected that state all along ("Stale index: N note(s) changed since last
// index pass") — but this hook, the one surface an agent reliably reads at
// session start, never relayed it. A check that exists but is not surfaced
// where the agent looks is the present-but-dead class again, one level up.

func TestHealthHook_SurfacesStaleIndexWarning(t *testing.T) {
	dir := projectWithVault(t)
	stub := stubVaultmind(t,
		"Index: 42 notes (bge-m3)\n⚠ Stale index: 3 note(s) changed since last index pass\nIssues: 0 errors, 1 warnings\n")
	out, _ := runHealthHook(t, dir, stub)

	assert.Contains(t, out, "Stale index: 3 note(s) changed since last index pass",
		"the stale-index warning must reach the session-start surface")
	assert.Contains(t, out, "vaultmind index --vault",
		"the warning must carry the one command that fixes it")
	assert.Contains(t, strings.ToLower(out), "old text",
		"the warning must say what the defect DOES — recall serving stale content — not just name a state")
}

func TestHealthHook_NoStaleWarningWhenIndexIsFresh(t *testing.T) {
	dir := projectWithVault(t)
	stub := stubVaultmind(t, "Index: 42 notes (bge-m3)\nIssues: 0 errors, 0 warnings\n")
	out, _ := runHealthHook(t, dir, stub)

	assert.NotContains(t, out, "Stale index",
		"a fresh index must not produce a stale warning")
}
