package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #407: ask delivers note text from the index. A note edited after the last
// `vaultmind index` came back with its pre-edit text and no sign of it.

const staleMarkerText = "changed on disk since indexed"

// editAlpha rewrites alpha's body on disk without reindexing and moves its
// mtime forward, so a same-second edit cannot hide behind the indexed mtime.
func editAlpha(t *testing.T, vault, body string) {
	t.Helper()
	p := filepath.Join(vault, "concepts", "alpha.md")
	require.NoError(t, os.WriteFile(p, []byte("---\nid: concept-alpha\ntype: concept\ntitle: Alpha Concept\ntags: [foundational]\nrelated_ids: [proj-beta]\n---\n"+body+"\n"), 0o644))
	later := time.Now().Add(time.Minute)
	require.NoError(t, os.Chtimes(p, later, later))
}

func askText(t *testing.T, vault string) string {
	t.Helper()
	out, _, err := runRootCmd(t, "ask", "Alpha", "--vault", vault)
	require.NoError(t, err)
	return out.String()
}

func TestAsk_EditedNoteIsMarkedStaleWithItsOldText(t *testing.T) {
	vault := buildIndexedTestVault(t)
	editAlpha(t, vault, "Alpha was rewritten after indexing.")

	text := askText(t, vault)

	assert.Contains(t, text, "Alpha body.", "probe: the pre-edit text is still what ask delivers")
	assert.NotContains(t, text, "rewritten after indexing")
	assert.Contains(t, text, staleMarkerText, "the delivered text must carry a changed-on-disk marker")
}

func TestAsk_DeletedNoteIsMarkedStale(t *testing.T) {
	vault := buildIndexedTestVault(t)
	require.NoError(t, os.Remove(filepath.Join(vault, "concepts", "alpha.md")))

	assert.Contains(t, askText(t, vault), staleMarkerText)
}

func TestAsk_UnchangedNoteOutputIsUntouched(t *testing.T) {
	vault := buildIndexedTestVault(t)
	before := askText(t, vault)
	assert.NotContains(t, before, staleMarkerText)

	// A touch with the same content is not a change.
	p := filepath.Join(vault, "concepts", "alpha.md")
	later := time.Now().Add(time.Minute)
	require.NoError(t, os.Chtimes(p, later, later))

	assert.Equal(t, before, askText(t, vault), "a touched but identical note must produce byte-identical output")
}

func TestAsk_JSONMarksStaleItemAndCountsThem(t *testing.T) {
	vault := buildIndexedTestVault(t)
	editAlpha(t, vault, "Alpha was rewritten after indexing.")

	out, _, err := runRootCmd(t, "ask", "Alpha", "--vault", vault, "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			StaleNotes int `json:"stale_notes"`
			Context    struct {
				TargetID string `json:"target_id"`
				Target   struct {
					Stale bool `json:"stale"`
				} `json:"target"`
				Context []struct {
					ID    string `json:"id"`
					Stale bool   `json:"stale"`
				} `json:"context"`
			} `json:"context"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	n := 0
	if env.Result.Context.TargetID == "concept-alpha" && env.Result.Context.Target.Stale {
		n++
	}
	for _, it := range env.Result.Context.Context {
		if it.ID == "concept-alpha" && it.Stale {
			n++
		}
	}
	assert.Equal(t, 1, n, "alpha must be marked stale exactly once")
	assert.Equal(t, 1, env.Result.StaleNotes)

	fresh := buildIndexedTestVault(t)
	out, _, err = runRootCmd(t, "ask", "Alpha", "--vault", fresh, "--json")
	require.NoError(t, err)
	assert.NotContains(t, out.String(), `"stale`, "a fresh vault adds no stale fields")
}
