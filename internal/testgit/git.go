// Package testgit runs test Git commands without the caller's GIT_* variables.
// Test-only: never import it from production code.
package testgit

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Command builds a Git command rooted at dir. Hooks export GIT_DIR,
// GIT_INDEX_FILE and friends; cmd.Dir alone does not isolate a test repository.
// Keep non-Git environment variables and disable system Git configuration.
// Tests that commit must provide their own identity (local config or -c flags).
func Command(tb testing.TB, dir string, args ...string) *exec.Cmd {
	tb.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // G204: test-only; arguments come from test code, never user input.
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1")
	return cmd
}

// Run executes a Git command, returning its combined output and failing the
// calling test with that output if the command fails. Use Command for expected
// nonzero exits or when the command needs further customization.
func Run(tb testing.TB, dir string, args ...string) []byte {
	tb.Helper()
	out, err := Command(tb, dir, args...).CombinedOutput()
	require.NoError(tb, err, "git %v in %s: %s", args, dir, out)
	return out
}
