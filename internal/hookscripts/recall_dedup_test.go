package hookscripts_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recall hook repeated itself: across 908 recall turns, 25.7% of the
// excerpts it delivered had already been delivered in the same conversation
// within 30 minutes (measured 2026-10-03). It asks with a dedup window, so a
// note it already delivered comes back as its title. The ledger is shared
// with the reach hook, so the two don't repeat each other either.
func TestRecall_AsksWithADedupWindow(t *testing.T) {
	h := newHookEnv(t, argsStub)
	stdin, err := json.Marshal(map[string]string{
		"prompt":     "what do I know about spreading activation in memory systems",
		"session_id": "conv-dedup",
	})
	require.NoError(t, err)

	out, _ := runHookScript(t, "vault-recall.sh", h.env(false), string(stdin))
	assert.Contains(t, out, "--dedup-window 30m")
}

// The window can be changed, like reach's.
func TestRecall_DedupWindowCanBeChanged(t *testing.T) {
	h := newHookEnv(t, argsStub)
	stdin, err := json.Marshal(map[string]string{
		"prompt":     "what do I know about spreading activation in memory systems",
		"session_id": "conv-dedup",
	})
	require.NoError(t, err)

	out, _ := runHookScript(t, "vault-recall.sh", append(h.env(false), "VAULTMIND_RECALL_DEDUP_WINDOW=5m"), string(stdin))
	assert.Contains(t, out, "--dedup-window 5m")
}
