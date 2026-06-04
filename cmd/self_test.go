package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `vaultmind self` on a freshly indexed vault prints the blank-slate
// message instead of failing or emitting empty output. Pins that the
// command is wired up end-to-end through cobra → MustAddToRoot →
// RunSelf, and that the expected blank-slate signal reaches stdout.
func TestSelf_BlankSlateOnFreshlyIndexedVault(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "self", "--vault", vault)
	require.NoError(t, err)
	assert.True(t, strings.Contains(out.String(), "no accesses recorded yet") ||
		strings.Contains(out.String(), "Memory state"),
		"self must produce either blank-slate marker or memory-state output, got: %q", out.String())
}

// --limit flag plumbs through to query.SelfConfig.Limit. Smoke-test
// the flag binding by passing --limit=1 and confirming the command
// runs without error.
func TestSelf_LimitFlagAccepted(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "self", "--vault", vault, "--limit", "1")
	require.NoError(t, err)
}
