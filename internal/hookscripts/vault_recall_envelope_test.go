package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise the stock bash hook with a recording binary, not script-text checks.
func TestRecallHook_HarnessEnvelopesOnly(t *testing.T) {
	cases := map[string]string{
		"task notification":     " \n<task-notification>\n<task-id>abc</task-id>\ncompleted\n</task-notification>\t ",
		"combined":              "<system-reminder>remember the setup</system-reminder>\n\t<cross-session-message from=\"other\">finished the background job</cross-session-message>",
		"cross session":         "<cross-session-message from=\"other\" session-id=\"123\">finished</cross-session-message>",
		"local stdout":          "<local-command-stdout>compacted the conversation</local-command-stdout>",
		"local caveat":          "<local-command-caveat>local command output follows</local-command-caveat>",
		"command name":          "<command-name>/compact</command-name>",
		"command message":       "<command-message>compacting the conversation</command-message>",
		"command args":          "<command-args>keep the retrieval discussion</command-args>",
		"embedded user wrapper": "<task-notification><user_query>how does spreading activation work</user_query></task-notification>",
	}
	for name, prompt := range cases {
		t.Run(name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "argv.log")
			h := newHookEnv(t, argvRecordingStub(marker))
			payload, err := json.Marshal(map[string]string{"prompt": prompt})
			require.NoError(t, err)

			cmd := exec.Command("/bin/bash", "vault-recall.sh")
			cmd.Env = h.env(false)
			cmd.Stdin = strings.NewReader(string(payload))
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "envelope-only prompts must exit 0")
			assert.Empty(t, string(out), "stdout and stderr must both stay quiet")
			_, err = os.Stat(marker)
			assert.True(t, os.IsNotExist(err), "recall must not run")

			logs, err := filepath.Glob(filepath.Join(h.home, ".vaultmind", "userprompt-hook", "*-skip.json"))
			require.NoError(t, err)
			require.Len(t, logs, 1, "the skip must be countable in the sidecar")
			raw, err := os.ReadFile(logs[0]) //nolint:gosec // test-owned temporary directory
			require.NoError(t, err)
			var entry map[string]any
			require.NoError(t, json.Unmarshal(raw, &entry))
			assert.Equal(t, "harness-envelope", entry["reason"])
			assert.Equal(t, false, entry["injection"])
			assert.Equal(t, float64(len(prompt)), entry["prompt_len"])
			assert.NotEmpty(t, entry["timestamp"])
			assert.NotContains(t, entry, "ask_status", "no ask was attempted")
		})
	}
}

func TestRecallHook_HarnessEnvelopesWithUserText(t *testing.T) {
	question := "how does spreading activation work"
	cases := map[string]string{
		"after":           "<task-notification>completed the job</task-notification> " + question,
		"before":          question + " <system-reminder>remember the setup</system-reminder>",
		"between":         "<system-reminder>remember the setup</system-reminder> " + question + " <cross-session-message from=\"other\">completed</cross-session-message>",
		"inside sentence": "how does<command-message>completed</command-message>spreading activation work",
		"normal question": question,
	}
	for name, prompt := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, question, askQueryReceived(t, prompt))
		})
	}
}

func TestRecallHook_HarnessEnvelopeStrippingKeepsOtherText(t *testing.T) {
	cases := []string{
		"how does <task-notification> without a closing tag work",
		"how does <task-notification-extra>completed</task-notification-extra> work",
		"how does <example>spreading activation</example> work",
	}
	for _, prompt := range cases {
		t.Run(prompt, func(t *testing.T) {
			assert.Equal(t, prompt, askQueryReceived(t, prompt))
		})
	}
}
