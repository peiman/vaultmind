package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Isolate the production predicate so Python startup, logging and the vault
// query do not hide its cost. Use stock macOS bash, including under set -e.
func envelopePredicate(t *testing.T) string {
	t.Helper()
	script, ok := hookscripts.Get("vault-recall.sh")
	require.True(t, ok)
	src := string(script)
	// The trim helpers sit immediately above the predicate and are part of it.
	// The isolated script has to carry them or the predicate cannot run.
	start := strings.Index(src, "leading_drop() {")
	fn := strings.Index(src, "is_envelope_only() {")
	require.GreaterOrEqual(t, start, 0)
	require.Greater(t, fn, start)
	_, predicate, ok := strings.Cut(src, "is_envelope_only() {")
	require.True(t, ok)
	predicate, _, ok = strings.Cut(predicate, "\n}\n")
	require.True(t, ok)
	return src[start:fn] + "is_envelope_only() {" + predicate + "\n}\n" +
		"prompt=$(cat); if is_envelope_only \"$prompt\"; then printf envelope; else printf recall; fi"
}

func TestRecallHook_HarnessEnvelopeParserTiming(t *testing.T) {
	const size = 20 * 1024
	filled := func(prefix, suffix string) string {
		return prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
	}
	prefix, suffix := "<task-notification>", "</task-notification>"
	left := size - len(prefix) - len(suffix)
	cases := []struct {
		name, prompt, result string
	}{
		{"plain", strings.Repeat("x", size), "recall"},
		{"body", filled(prefix, suffix), "envelope"},
		{"attribute", filled("<task-notification hint=\"", "\">x</task-notification>"), "envelope"},
		{"many tags", prefix + strings.Repeat("<x>x</x>", left/8) + strings.Repeat(" ", left%8) + suffix, "envelope"},
	}
	limit := 500 * time.Millisecond
	if os.Getenv("CI") != "" {
		limit = 2 * time.Second // Still far below the old parser's 12–15 seconds.
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Len(t, tc.prompt, size)
			cmd := exec.Command(stockMacBash, "-e", "-c", envelopePredicate(t))
			cmd.Env = []string{"PATH=/usr/bin:/bin"}
			cmd.Stdin = strings.NewReader(tc.prompt)
			start := time.Now()
			out, err := cmd.CombinedOutput()
			elapsed := time.Since(start)
			require.NoError(t, err, "%s", out)
			assert.Equal(t, tc.result, string(out))
			t.Logf("20480 bytes: parser %s (limit %s)", elapsed, limit)
			assert.Less(t, elapsed, limit, "envelope detection runs before the query timeout")
		})
	}
}

func TestRecallHook_HarnessEnvelopeCheapGate(t *testing.T) {
	for name, prompt := range map[string]string{
		"prose before": "Explain <task-notification hint=\"example\">finished</task-notification>",
		"prose after":  "<task-notification>finished</task-notification> explain the result",
		"BOM before":   " \t\ufeff\nExplain <task-notification>finished</task-notification> \t",
	} {
		t.Run(name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "awk.log")
			binDir := t.TempDir()
			// A recording dependency proves ungated prompts never start the parser.
			hookscripts.InstallStub(t, binDir, "awk", "#!/bin/bash\nprintf invoked > "+marker+"\nexit 1\n")
			cmd := exec.Command(stockMacBash, "-e", "-c", envelopePredicate(t))
			cmd.Env = []string{"PATH=" + binDir + ":/usr/bin:/bin"}
			cmd.Stdin = strings.NewReader(prompt)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err)
			assert.Equal(t, "recall", string(out))
			_, err = os.Stat(marker)
			assert.True(t, os.IsNotExist(err), "cheap gate must run before awk")
		})
	}
}

func TestRecallHook_HarnessEnvelopeParserLocale(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("BSD awk UTF-8 byte splitting is specific to the stock macOS hook environment")
	}
	// Unicode attribute names/whitespace are accepted by Bash in a UTF-8
	// locale. BSD awk splits bytes even there, so test the same locale explicitly.
	for _, prompt := range []string{
		"<task-notification tür=ok>done</task-notification>",
		"<task-notification\u00a0hint=ok>done</task-notification>",
		"<task-notification><é hint=\"</task-notification>\">x</é>done</task-notification>",
		"<task-notification hint=\"å中文😀\">done</task-notification>",
	} {
		t.Run(prompt, func(t *testing.T) {
			cmd := exec.Command(stockMacBash, "-e", "-c", envelopePredicate(t))
			cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C.UTF-8"}
			cmd.Stdin = strings.NewReader(prompt)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err)
			assert.Equal(t, "envelope", string(out))
		})
	}
}

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
		"same tag nesting":      "<task-notification><task-notification>finished</task-notification>done</task-notification>",
		"different nesting":     "<task-notification><system-reminder>finished</system-reminder>done</task-notification>",
		"BOM and whitespace":    " \t\ufeff\n<task-notification>finished</task-notification> \t",
		"quoted angle bracket":  "<task-notification kind=\"a > b\">finished</task-notification>",
		"quoted closing tag":    "<task-notification><task-id hint=\"</task-notification>\">abc</task-id>done</task-notification>",
		"attribute forms":       "<task-notification enabled data-id = abc other='a < b > c' flag next = \"done\">finished</task-notification \t>",
		"multiline attributes":  "<task-notification\n hint=\"first\n</task-notification>\nlast\"\n enabled\n>finished</task-notification\n>",
		"unknown markup quotes": "<task-notification><task-id hint='</task-notification>'>abc</task-id>done</task-notification>",
	}
	for name, prompt := range cases {
		t.Run(name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "argv.log")
			h := newHookEnv(t, argvRecordingStub(marker))
			payload, err := json.Marshal(map[string]string{"prompt": prompt})
			require.NoError(t, err)

			cmd := exec.Command("/bin/bash", "-e", "-c", "umask 022; exec /bin/bash -e vault-recall.sh")
			cmd.Env = h.env(false)
			cmd.Stdin = strings.NewReader(string(payload))
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "envelope-only prompts must exit 0")
			assert.Empty(t, string(out), "stdout and stderr must both stay quiet")
			_, err = os.Stat(marker)
			assert.True(t, os.IsNotExist(err), "recall must not run")

			logs, err := filepath.Glob(filepath.Join(h.home, ".vaultmind", "userprompt-hook", "envelope-skips.jsonl"))
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
			for path, mode := range map[string]os.FileMode{filepath.Dir(logs[0]): 0o700, logs[0]: 0o600} {
				info, statErr := os.Stat(path)
				require.NoError(t, statErr)
				assert.Equal(t, mode, info.Mode().Perm(), "new logs must be private under umask 022")
			}
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
		"quoted example":  "Explain <task-notification kind=\"example\">how does spreading activation work?</task-notification>",
	}
	for name, prompt := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, prompt, askQueryReceived(t, prompt))
		})
	}
}

func TestRecallHook_HarnessEnvelopeRejectsMalformedInput(t *testing.T) {
	cases := []string{
		"how does <task-notification> without a closing tag work",
		"how does <task-notification-extra>completed</task-notification-extra> work",
		"how does <example>spreading activation</example> work",
		"<task-notification>unfinished background task",
		"<task-notification><task-notification>finished</task-notification>",
		"<task-notification>finished</task-notification></task-notification>",
		"<task-notification><system-reminder>finished</task-notification></system-reminder>",
		"<task-notification>finished</task-notification invalid>",
		"<task-notification kind=\"unfinished>finished</task-notification>",
		"<task-notification kind=>finished</task-notification>",
		"<task-notification/>finished</task-notification>",
		"<task-notification><task-notification/>finished</task-notification>",
		"<task-notification><task-notification!>finished</task-notification>",
		"<task-notification><task-id hint=\"</task-notification>",
		"<task-notification hint=\"done\"next=abc>finished</task-notification>",
		"<task-notification hint=a=b>finished</task-notification>",
		"<task-notification hint=a/b>finished</task-notification>",
		"<task-notification hint= >finished</task-notification>",
		"<task-notification>finished</task-notification>\ufeff",
		"<task-notification>finished</task-notification> prose <system-reminder>done</system-reminder>",
		"<task-notification>finished</task-notification><unknown>done</unknown>",
		"<task-notification>finished</task-notification> user question follows",
	}
	for _, prompt := range cases {
		t.Run(prompt, func(t *testing.T) {
			assert.Equal(t, prompt, askQueryReceived(t, prompt))
		})
	}
}

// Use the real hook even for repeated skips; the rolling record must remain
// bounded, parseable, private, and free of prompt contents under a normal umask.
func TestRecallHook_HarnessEnvelopeLogBoundAndPermissions(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "argv.log")
	h := newHookEnv(t, argvRecordingStub(marker))
	logDir := filepath.Join(h.home, ".vaultmind", "userprompt-hook")
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	logPath := filepath.Join(logDir, "envelope-skips.jsonl")
	require.NoError(t, os.WriteFile(logPath, nil, 0o644))
	for i := 0; i < 140; i++ {
		runEnvelopeSkip(t, h, "<task-notification>private payload</task-notification>")
	}
	entries, err := os.ReadDir(logDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no per-skip files or leftover locks/temporaries")
	raw, err := os.ReadFile(logPath) //nolint:gosec // test-owned path
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	require.Len(t, lines, 128, "retain only the newest 128 countable records")
	for _, line := range lines {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		assert.Equal(t, "harness-envelope", entry["reason"])
		assert.NotContains(t, entry, "prompt")
	}
	assert.NotContains(t, string(raw), "private payload")
	assert.Less(t, len(raw), 32*1024, "metadata must also stay small in bytes")
	for path, mode := range map[string]os.FileMode{logDir: 0o700, logPath: 0o600} {
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		assert.Equal(t, mode, info.Mode().Perm(), path)
	}
	_, err = os.Stat(marker)
	assert.True(t, os.IsNotExist(err), "recall must not run")
}

func TestRecallHook_HarnessEnvelopeLoggingFailureIsQuiet(t *testing.T) {
	for _, failure := range []string{"directory", "file", "lock"} {
		t.Run(failure, func(t *testing.T) {
			h := newHookEnv(t, fastStub)
			root := filepath.Join(h.home, ".vaultmind")
			if failure == "directory" {
				require.NoError(t, os.WriteFile(root, []byte("not a directory"), 0o600))
			} else {
				logDir := filepath.Join(root, "userprompt-hook")
				require.NoError(t, os.MkdirAll(logDir, 0o700))
				name := "envelope-skips.jsonl"
				if failure == "lock" {
					name = ".envelope-skips.lock"
				}
				require.NoError(t, os.Mkdir(filepath.Join(logDir, name), 0o700))
			}
			runEnvelopeSkip(t, h, "<task-notification>finished</task-notification>")
		})
	}
}

func runEnvelopeSkip(t *testing.T, h hookEnv, prompt string) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"prompt": prompt})
	require.NoError(t, err)
	cmd := exec.Command("/bin/bash", "-e", "-c", "umask 022; exec /bin/bash -e vault-recall.sh")
	cmd.Env = h.env(false)
	cmd.Stdin = strings.NewReader(string(payload))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "skip must exit 0 even under set -e")
	assert.Empty(t, string(out), "skip must be quiet on both stdout and stderr")
}
