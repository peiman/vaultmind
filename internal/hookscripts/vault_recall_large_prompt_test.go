package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Bash 3.2 on macOS walks ${var#pattern} one UTF-8 character at a time.
// A ~200 KB prompt makes that trim take several seconds, and the hook used
// to do it more than once before it could decide to skip. These tests run
// the hook script itself under /bin/bash with LANG=en_US.UTF-8 and no
// LC_ALL, which is the locale where that trim is slow. The hook must
// finish inside the budget without the caller setting LC_ALL=C.

const largePromptBytes = 200000

const largePromptBudget = 2 * time.Second

func largeHandback(n int) string {
	open := "<agent-message from=\"agent-1\">\n"
	closeTag := "\n</agent-message>"
	fixed := len(crossSessionPrefix) + len(open) + len(closeTag) + len(crossSessionSuffixPeer)
	return crossSessionPrefix + open + strings.Repeat("y", n-fixed) + closeTag + crossSessionSuffixPeer
}

func largeTaskNotification(n int) string {
	open := "<task-notification>\n"
	closeTag := "</task-notification>"
	return open + strings.Repeat("z", n-len(open)-len(closeTag)) + closeTag
}

func largeHumanMention(n int) string {
	mid := "<agent-message"
	pad := n - len(mid)
	return strings.Repeat("h", pad/2) + mid + strings.Repeat("h", pad-pad/2)
}

func TestRecallHook_LargePromptSkipsQuickly(t *testing.T) {
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("no /bin/bash")
	}
	cases := []struct {
		name   string
		prompt string
		reason string // empty means the stub vaultmind must be called
	}{
		{"hand-back", largeHandback(largePromptBytes), "cross-session-envelope"},
		{"task-notification", largeTaskNotification(largePromptBytes), "harness-envelope"},
		{"human mention", largeHumanMention(largePromptBytes), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, largePromptBytes, len(tc.prompt))
			marker := filepath.Join(t.TempDir(), "argv.log")
			h := newHookEnv(t, argvRecordingStub(marker))
			payload, err := json.Marshal(map[string]string{"prompt": tc.prompt, "session_id": "large-prompt"})
			require.NoError(t, err)

			cmd := exec.Command("/bin/bash", "-e", "-c", "umask 022; exec /bin/bash -e vault-recall.sh")
			cmd.Env = append(h.env(false), "LANG=en_US.UTF-8")
			cmd.Stdin = strings.NewReader(string(payload))
			start := time.Now()
			out, err := cmd.CombinedOutput()
			elapsed := time.Since(start)
			require.NoError(t, err, "hook must exit 0: %s", out)
			t.Logf("%s: %d bytes in %s", tc.name, len(tc.prompt), elapsed)
			assert.Less(t, elapsed, largePromptBudget, "skip logic must stay inside the hook budget")

			_, statErr := os.Stat(marker)
			if tc.reason == "" {
				assert.NoError(t, statErr, "a large human prompt that only mentions the tag must still call vaultmind")
				assert.NotContains(t, string(out), "cross-session-envelope")
				return
			}
			assert.True(t, os.IsNotExist(statErr), "recall must not run")
			assert.Empty(t, string(out))
			logs, globErr := filepath.Glob(filepath.Join(h.home, ".vaultmind", "userprompt-hook", "envelope-skips.jsonl"))
			require.NoError(t, globErr)
			require.Len(t, logs, 1)
			raw, readErr := os.ReadFile(logs[0]) //nolint:gosec // test-owned temporary directory
			require.NoError(t, readErr)
			var entry map[string]any
			require.NoError(t, json.Unmarshal(raw, &entry))
			assert.Equal(t, tc.reason, entry["reason"])
			assert.Equal(t, false, entry["injection"])
		})
	}
}

// prefix + system-reminder + the peer suffix matches the cross-session
// shape, but system-reminder is not one of the three observed hand-back
// tags. It must be recalled, not logged as cross-session-envelope.
func TestRecallHook_CrossSessionSystemReminderNotSkipped(t *testing.T) {
	prompt := crossSessionPrefix +
		"<system-reminder>remember the setup</system-reminder>" +
		crossSessionSuffixPeer
	got := askQueryReceived(t, prompt)
	assert.Contains(t, got, "remember the setup")
}
