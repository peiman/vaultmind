package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/onboard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommandsMarkdown_DriftGate is the regenerate-and-diff drift gate
// (mirroring check-constants.sh): it regenerates the command-catalog markdown
// in-memory from the live RootCmd and asserts it byte-for-byte equals the
// committed internal/onboard/COMMANDS.md. The moment someone adds, renames, or
// re-groups a command without regenerating, this test goes red — so the
// embedded onboarding reference can never silently drift from the tree.
//
// Fix when red: vaultmind docs commands --output internal/onboard/COMMANDS.md
func TestCommandsMarkdown_DriftGate(t *testing.T) {
	fresh := commandsMarkdown()
	committed := string(onboard.Commands())
	assert.Equal(t, committed, fresh,
		"internal/onboard/COMMANDS.md is stale — run: "+
			"vaultmind docs commands --output internal/onboard/COMMANDS.md")
}

// TestRunDocsCommands_Stdout the `docs commands` command writes the grouped
// markdown reference to its output writer when no --output is set.
func TestRunDocsCommands_Stdout(t *testing.T) {
	out, _, err := runRootCmd(t, "docs", "commands")
	require.NoError(t, err)
	body := out.String()
	assert.Contains(t, body, "# VaultMind Commands")
	assert.Contains(t, body, "| Command | What | When to use |")
	assert.Contains(t, body, "`vaultmind ask`")
}

// TestWriteCommandsMarkdown_ToFile the --output path writes the reference to
// disk and round-trips identically.
func TestWriteCommandsMarkdown_ToFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "COMMANDS.md")
	out, _, err := runRootCmd(t, "docs", "commands", "--output", target)
	require.NoError(t, err)
	assert.Empty(t, out.String(), "file mode must not also print to stdout")

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, commandsMarkdown(), string(got),
		"written file must match the rendered catalog markdown")
}

// TestWriteCommandsMarkdown_ErrorOnBadPath an unwritable output path surfaces a
// wrapped error rather than panicking.
func TestWriteCommandsMarkdown_ErrorOnBadPath(t *testing.T) {
	// A path whose parent directory does not exist cannot be written.
	bad := filepath.Join(t.TempDir(), "no-such-dir", "COMMANDS.md")
	_, _, err := runRootCmd(t, "docs", "commands", "--output", bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing command reference")
}
