package hooks_test

import (
	"encoding/json"
	"testing"

	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The five original hooks are all READ-path or telemetry: load identity, surface
// pointers, track reads, capture the transcript. Nothing fired at the moment a
// transformation could be WRITTEN down, and nothing fired at the moment an
// irreversible command was about to run.
//
// Both gaps were closed in a consumer and never shipped. A desk that depends on
// remembering to write to it collects nothing — measured: two entries in ten
// weeks. A trigger at the moment of the reach is what fixed the read path, and
// the write path needs the same treatment.
func TestSettingsStanza_WiresTheWritePathAndReachHooks(t *testing.T) {
	stanza, err := hooks.SettingsStanza("")
	require.NoError(t, err)

	var parsed struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal([]byte(stanza), &parsed))

	assert.Contains(t, parsed.Hooks, "PreCompact",
		"compaction is the only moment where the raw material still exists AND is provably about to be destroyed")
	assert.Contains(t, stanza, "precompact-preserve.sh")

	assert.Contains(t, stanza, "vault-reach.sh",
		"an arc that surfaces all day and never at the instant it applies is not surfacing")
	assert.Contains(t, stanza, `"Bash"`, "the reach hook matches Bash, where irreversible commands run")
}

// PreToolUse now carries TWO groups with DIFFERENT matchers: Read (access
// tracking) and Bash (the reach pointers). Rendering one must not drop the
// other — they are separate concerns that happen to share an event.
func TestSettingsStanza_PreToolUseKeepsBothReadAndBash(t *testing.T) {
	stanza, err := hooks.SettingsStanza("")
	require.NoError(t, err)

	var parsed struct {
		Hooks struct {
			PreToolUse []struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"PreToolUse"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal([]byte(stanza), &parsed))
	require.Len(t, parsed.Hooks.PreToolUse, 2, "Read-tracking and reach-pointers are both PreToolUse")

	matchers := map[string]string{}
	for _, g := range parsed.Hooks.PreToolUse {
		require.NotEmpty(t, g.Hooks)
		matchers[g.Matcher] = g.Hooks[0].Command
	}
	assert.Contains(t, matchers["Read"], "vault-track-read.sh")
	assert.Contains(t, matchers["Bash"], "vault-reach.sh")
}

// The vault path must reach the new hooks too, or an adopter with a
// non-default vault gets a reach hook pointed at a directory that isn't theirs.
func TestSettingsStanza_VaultPathReachesTheNewHooks(t *testing.T) {
	stanza, err := hooks.SettingsStanza("/tmp/my-vault")
	require.NoError(t, err)

	for _, script := range []string{"precompact-preserve.sh", "vault-reach.sh"} {
		idx := 0
		for {
			i := indexFrom(stanza, script, idx)
			if i < 0 {
				break
			}
			assert.Contains(t, stanza[max0(i-160):i], "VAULTMIND_VAULT=",
				"%s must be invoked with the configured vault", script)
			idx = i + 1
		}
	}
}

func indexFrom(s, sub string, from int) int {
	if from >= len(s) {
		return -1
	}
	i := stringsIndex(s[from:], sub)
	if i < 0 {
		return -1
	}
	return from + i
}

func stringsIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func max0(i int) int {
	if i < 0 {
		return 0
	}
	return i
}
