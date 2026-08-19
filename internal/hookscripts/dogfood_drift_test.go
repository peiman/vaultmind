package hookscripts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This repo installs its own hooks into .claude/scripts/ and runs them on every
// session — it is the tool's first consumer. Those copies are supposed to BE the
// scripts the binary ships, and nothing checked that they still were.
//
// They had rotted badly: five scripts differed, one by 60 lines, including a
// noise guard the canonical had and the dogfood copy did not. So the repo was
// running an older VaultMind than it shipped, and the difference was invisible.
//
// It bit for real. The delivery work was applied to a working copy of these
// hooks and NOT to internal/hookscripts/, so `vaultmind hooks install` kept
// writing the pre-fix versions and every adopter would have got none of it.
// A reviewer found that by reading; this test finds it by failing.
//
// `vaultmind doctor` already reports the same drift for an installed vault at
// runtime. This is that check pointed at the repo itself, in CI, before release.
func TestDogfoodHooksMatchEmbedded(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	localDir := filepath.Join(repoRoot, ".claude", "scripts")

	names := hookscripts.Names()
	require.NotEmpty(t, names, "no embedded scripts found — the test would pass vacuously")

	var compared int
	for _, name := range names {
		localPath := filepath.Join(localDir, name)
		local, readErr := os.ReadFile(localPath) //nolint:gosec // repo-relative, test-only
		if os.IsNotExist(readErr) {
			// Not installed here. Whether this repo SHOULD dogfood every hook is
			// a separate decision; drift is only meaningful for the ones it does.
			t.Logf("not installed in .claude/scripts (skipped): %s", name)
			continue
		}
		require.NoError(t, readErr)

		embedded, ok := hookscripts.Get(name)
		require.True(t, ok, "embedded script %s vanished between Names() and Get()", name)
		compared++

		// Compared by hash, and reported as a count. assert.Equal on the contents
		// dumps both files in full — 35KB for one script — which buries the one
		// line a CI reader needs.
		if string(embedded) != string(local) {
			embLines, locLines := len(strings.Split(string(embedded), "\n")), len(strings.Split(string(local), "\n"))
			assert.Fail(t,
				"dogfood hook has drifted from the embedded canonical",
				"%s: canonical %d lines, .claude/scripts copy %d lines\n"+
					"This repo runs .claude/scripts/ as its own hooks, so a difference means it is "+
					"dogfooding something other than what it ships — which is exactly how the delivery "+
					"fix reached the working copies and missed the shipped ones.\n"+
					"  see:  diff internal/hookscripts/%s .claude/scripts/%s\n"+
					"  fix:  cp internal/hookscripts/%s .claude/scripts/%s",
				name, embLines, locLines, name, name, name, name)
		}
	}

	require.Positive(t, compared,
		"compared no scripts at all — the embedded names and .claude/scripts/ share none, "+
			"so this test is not checking anything")
}
