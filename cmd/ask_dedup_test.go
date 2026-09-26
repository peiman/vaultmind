package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Asked again in the same conversation, a note already delivered comes back
// as its title; a new conversation gets it in full again.
func TestAsk_DedupWindowSendsARepeatAsItsTitle(t *testing.T) {
	vault := indexedBaselineVault(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("VAULTMIND_USER_SESSION_ID", "conversation-1")
	args := []string{"ask", "spreading activation", "--vault", vault, "--dedup-window", "10m", "--budget", "4000"}

	first, _, err := runRootCmd(t, args...)
	require.NoError(t, err)
	require.NotContains(t, first.String(), "shown earlier this session")
	require.Contains(t, first.String(), "Spreading activation propagates", "the first answer carries the text")

	second, _, err := runRootCmd(t, args...)
	require.NoError(t, err)
	assert.Contains(t, second.String(), "shown earlier this session")
	assert.NotContains(t, second.String(), "Spreading activation propagates", "the text is not sent twice")

	t.Setenv("VAULTMIND_USER_SESSION_ID", "conversation-2")
	third, _, err := runRootCmd(t, args...)
	require.NoError(t, err)
	assert.Contains(t, third.String(), "Spreading activation propagates", "another conversation starts fresh")
}

func TestAsk_DedupWindowIsOffWithoutTheFlag(t *testing.T) {
	vault := indexedBaselineVault(t)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("VAULTMIND_USER_SESSION_ID", "conversation-1")
	for i := 0; i < 2; i++ {
		out, _, err := runRootCmd(t, "ask", "spreading activation", "--vault", vault, "--budget", "4000")
		require.NoError(t, err)
		assert.NotContains(t, out.String(), "shown earlier this session")
	}
}

func TestAsk_DedupWindowRejectsANonDuration(t *testing.T) {
	vault := indexedBaselineVault(t)
	_, _, err := runRootCmd(t, "ask", "x", "--vault", vault, "--dedup-window", "soon")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "positive duration")
}
