package hooks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CompareInstalled reports drift and nothing else, so a hook that was never
// written is indistinguishable from one that matches. That gap is not
// hypothetical: two hooks existed only in a consumer for months and every
// adopter was missing them, with no command that would say so.
func TestStatus_ReportsMissingSeparatelyFromDrifted(t *testing.T) {
	projectDir := t.TempDir()
	scripts := filepath.Join(projectDir, ".claude", "scripts")
	require.NoError(t, os.MkdirAll(scripts, 0o750))

	names := hookscripts.Names()
	require.GreaterOrEqual(t, len(names), 3, "need a few canonical scripts to differentiate states")

	// One in sync, one drifted, the rest absent.
	inSync, _ := hookscripts.Get(names[0])
	require.NoError(t, os.WriteFile(filepath.Join(scripts, names[0]), inSync, 0o700))
	drifted, _ := hookscripts.Get(names[1])
	require.NoError(t, os.WriteFile(filepath.Join(scripts, names[1]),
		append(drifted, []byte("\necho 'local change'\n")...), 0o700))

	report, err := hooks.Status(projectDir)
	require.NoError(t, err)
	assert.True(t, report.Installed)

	state := map[string]hooks.ScriptState{}
	for _, s := range report.Scripts {
		state[s.Name] = s.State
	}
	assert.Equal(t, hooks.ScriptInSync, state[names[0]])
	assert.Equal(t, hooks.ScriptDrifted, state[names[1]])
	assert.Equal(t, hooks.ScriptMissing, state[names[2]],
		"a hook that was never installed is not 'fine' — it is absent")
}

// Comment-only edits are not drift: a hook's behaviour is its code. Without
// this, every local annotation would look like a change and the signal would be
// unusable — which is how a drift report trains people to ignore it.
func TestStatus_CommentOnlyEditsAreNotDrift(t *testing.T) {
	projectDir := t.TempDir()
	scripts := filepath.Join(projectDir, ".claude", "scripts")
	require.NoError(t, os.MkdirAll(scripts, 0o750))

	// Annotated the way a person actually would: AFTER the shebang, which has to
	// stay on line 1. A comment prepended before it breaks the script, so
	// treating that as a change rather than an annotation is correct.
	name := hookscripts.Names()[0]
	canonical, _ := hookscripts.Get(name)
	lines := strings.SplitN(string(canonical), "\n", 2)
	require.Len(t, lines, 2, "canonical script should be more than a shebang")
	annotated := lines[0] + "\n# a local note about why this is here\n" + lines[1]
	require.NoError(t, os.WriteFile(filepath.Join(scripts, name), []byte(annotated), 0o700))

	report, err := hooks.Status(projectDir)
	require.NoError(t, err)
	for _, s := range report.Scripts {
		if s.Name == name {
			assert.Equal(t, hooks.ScriptInSync, s.State, "a comment is not a behaviour change")
		}
	}
}

// Nothing installed at all is its own answer, not an empty success.
func TestStatus_NotInstalledIsDistinctFromInSync(t *testing.T) {
	report, err := hooks.Status(t.TempDir())
	require.NoError(t, err)
	assert.False(t, report.Installed, "no .claude/scripts means the hooks are not installed")
}

// The existing drift API must keep its exact meaning — doctor depends on it.
func TestCompareInstalled_StillReportsOnlyDrift(t *testing.T) {
	projectDir := t.TempDir()
	scripts := filepath.Join(projectDir, ".claude", "scripts")
	require.NoError(t, os.MkdirAll(scripts, 0o750))

	name := hookscripts.Names()[0]
	canonical, _ := hookscripts.Get(name)
	require.NoError(t, os.WriteFile(filepath.Join(scripts, name),
		append(canonical, []byte("\necho drift\n")...), 0o700))

	drifted, err := hooks.CompareInstalled(projectDir)
	require.NoError(t, err)
	assert.Equal(t, []string{name}, drifted,
		"missing scripts must NOT appear here — doctor counts this as drift, not absence")
}
