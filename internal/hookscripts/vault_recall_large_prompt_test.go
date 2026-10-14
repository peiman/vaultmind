package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Bash 3.2 on macOS walks a whole-prompt prefix trim one UTF-8 character at
// a time. The pre-fix hook did that before it could decide to skip, and took
// 22–37s on these 200 KB fixtures. These tests run the hook itself under
// /bin/bash with LANG=en_US.UTF-8 and no LC_ALL in the caller. The wall
// clock is a backstop: a 2s bound already failed at 2.128s under the
// coverage suite on a machine at load 50–300, so the backstop is 10s, still
// under the pre-fix cost. The deterministic check is
// TestRecallHook_WholePromptPatternTrimIsRejected. Dropping the hook's own
// LC_ALL=C is not what either test guards: the trim no longer walks the
// prompt, and the locale is there so the indexes stay bytes.

const largePromptBytes = 200000

const largePromptBudget = 10 * time.Second

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

// patternTrim is a prefix or suffix removal: ${name#pat}, ${name##pat},
// ${name%pat}, or ${name%%pat}. The operator is the one that touches the
// named variable. A # or % before the name, as in ${#name}, is a length.
var patternTrim = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(##|%%|#|%)`)

// Whole-prompt holders. A pattern trim of one of these walks the prompt.
// head and tail are the bounded slices the hook is allowed to trim.
var wholePromptTrimNames = map[string]bool{
	"candidate": true,
	"c":         true,
	"PROMPT":    true,
	"body":      true,
	"rest":      true,
}

// TestRecallHook_WholePromptPatternTrimIsRejected fails if a prefix or
// suffix removal names the whole prompt. head and tail are the allowlist:
// each is assigned as a length-capped slice, or as the helper parameter
// that those slices are passed in as. The check is syntactic. It does not
// evaluate the length expression, so `edge=${#c}` and then a slice of
// `edge` would still pass; the script's edge is a decimal literal.
func TestRecallHook_WholePromptPatternTrimIsRejected(t *testing.T) {
	// The scanner has to see the operator that sits on the name, including
	// the doubled forms, and it has to ignore a length and a substring.
	require.Equal(t, []string{"candidate#", "head##", "tail%%"}, patternTrimOps(
		`${candidate#x} ${head##y} ${tail%%z} ${#c} ${c:0:3}`,
	))

	script, ok := hookscripts.Get("vault-recall.sh")
	require.True(t, ok)
	text := string(script)
	ops := patternTrimOps(text)
	require.NotEmpty(t, ops, "the scanner found no trims, so it is not reading the hook")
	require.Contains(t, ops, "head%%")
	require.Contains(t, ops, "tail##")

	var offenders []string
	for _, op := range ops {
		name := strings.TrimRight(op, "#%")
		if wholePromptTrimNames[name] || (name != "head" && name != "tail") {
			offenders = append(offenders, "${"+op)
		}
	}
	assert.Empty(t, offenders,
		"pattern trim must name a bounded slice (head, tail), not the whole prompt (candidate, c, PROMPT, body, rest)")

	for _, call := range regexp.MustCompile(`leading_drop\s+"(\$[^"]*)"`).FindAllStringSubmatch(text, -1) {
		assert.Equal(t, "$head", call[1], "leading_drop must take the bounded head")
	}
	for _, call := range regexp.MustCompile(`trailing_ws\s+"(\$[^"]*)"`).FindAllStringSubmatch(text, -1) {
		assert.Equal(t, "$tail", call[1], "trailing_ws must take the bounded tail")
	}
	require.Contains(t, text, `leading_drop "$head"`)
	require.Contains(t, text, `trailing_ws "$tail"`)

	assign := regexp.MustCompile(`(?m)^[ \t]*(?:local[ \t]+)?(head|tail)=([^\n]*)`)
	found := assign.FindAllStringSubmatch(text, -1)
	require.NotEmpty(t, found, "head and tail must be assigned as slices")
	for _, m := range found {
		assert.Truef(t, boundedSliceRHS(m[1], m[2]),
			"%s=%s copies the prompt instead of a length-capped slice", m[1], strings.TrimSpace(m[2]))
	}
	assert.Regexp(t, `(?m)^[ \t]*edge=[0-9]+\s*$`, text, "the head bound must be a literal length")
	assert.NotRegexp(t, `(?m)^[ \t]*(?:edge|bound)=.*\$\{#`, text,
		"the slice length must not be the length of the prompt")
}

func patternTrimOps(script string) []string {
	var ops []string
	for _, m := range patternTrim.FindAllStringSubmatch(script, -1) {
		ops = append(ops, m[1]+m[2])
	}
	return ops
}

func boundedSliceRHS(name, rhs string) bool {
	rhs = strings.TrimSpace(rhs)
	switch name {
	case "head":
		return rhs == `"$1"` || strings.HasPrefix(rhs, `"${head:`) || strings.HasPrefix(rhs, `"${c:0:`)
	case "tail":
		return rhs == `"$1"` || strings.HasPrefix(rhs, `"${tail:`) ||
			(strings.HasPrefix(rhs, `"${c:`) && strings.Count(rhs, ":") >= 2)
	default:
		return false
	}
}
