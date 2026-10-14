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
	"unicode/utf8"

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

// largeBareHandback is the envelope the hook receives: no transcript prefix
// and no suffix. The report starts with "<", which the tag parser rejects,
// so a skip that parses the body falls through to recall.
func largeBareHandback(n int) string {
	open := "<agent-message from=\"agent-1\">\n"
	closeTag := "\n</agent-message>"
	gap := n - len(open) - len(closeTag) - 1
	return open + "<" + strings.Repeat("y", gap) + closeTag
}

func TestRecallHook_LargePromptSkipsQuickly(t *testing.T) {
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("no /bin/bash")
	}
	cases := []struct {
		name       string
		prompt     string
		reason     string // empty means the stub vaultmind must be called
		kind       string
		queryChars float64
		leak       string
	}{
		{"hand-back", largeHandback(largePromptBytes), "cross-session-envelope", "agent-message", 0, strings.Repeat("y", 80)},
		{"bare hand-back", largeBareHandback(largePromptBytes), "cross-session-envelope", "agent-message", 0, strings.Repeat("y", 80)},
		{"task-notification", largeTaskNotification(largePromptBytes), "harness-envelope", "task-notification", 0, strings.Repeat("z", 80)},
		{"human mention", largeHumanMention(largePromptBytes), "", "prose", float64(recallQueryCharCap), strings.Repeat("h", 80)},
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

			entry, raw := promptKindRecord(t, h.home)
			keys := recallEnvelopeKeys
			if tc.reason == "" {
				keys = recallInjectKeys
			}
			assertPromptKindRecord(t, entry, raw, tc.kind, tc.queryChars, keys, []string{tc.leak, "Another Claude"})
			_, statErr := os.Stat(marker)
			if tc.reason == "" {
				assert.NoError(t, statErr, "a large human prompt that only mentions the tag must still call vaultmind")
				assert.NotContains(t, string(out), "cross-session-envelope")
				return
			}
			assert.True(t, os.IsNotExist(statErr), "recall must not run")
			assert.Empty(t, string(out))
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

// Linux rejects one argv string past this many characters once it is
// UTF-8: 16384 characters is at most 64 KiB, under MAX_ARG_STRLEN.
const recallQueryCharCap = 16384

// patternTrim is a prefix or suffix removal: ${name#pat}, ${name##pat},
// ${name%pat}, or ${name%%pat}. The operator is the one that touches the
// named variable. A # or % before the name, as in ${#name}, is a length.
var patternTrim = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(##|%%|#|%)`)

// promptSubst is ${name/pat} or ${name//pat}. A colon, as in
// ${name:offset:length}, is a slice and does not match.
var promptSubst = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(//|/)`)

// promptRegex is a bash regex match with the variable on the left.
// Quotes and braces are optional: [[ $name =~, [[ ${name} =~,
// [[ "$name" =~, [[ "${name}" =~.
var promptRegex = regexp.MustCompile(`\[\[\s*"?\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?"?\s*=~`)

// Whole-prompt holders. A pattern trim, a substitution, or a regex match
// of one of these walks the prompt. head and tail are the bounded slices
// the hook is allowed to trim. QUERY="${QUERY:0:16384}" is a slice, which
// promptSubst does not see.
var wholePromptNames = map[string]bool{
	"candidate": true,
	"c":         true,
	"PROMPT":    true,
	"QUERY":     true,
	"body":      true,
	"rest":      true,
}

// TestRecallHook_WholePromptPatternTrimIsRejected fails if a prefix or
// suffix removal, a substitution, or a regex match names the whole prompt.
// head and tail are the allowlist for pattern trims: each is assigned as a
// length-capped slice, or as the helper parameter that those slices are
// passed in as. The ask cap is a substring of QUERY with a literal length,
// which is not a substitution; an allowlisted bounded variable holding
// that same slice would pass these scanners too. The check is syntactic.
// It does not evaluate the length expression, so `edge=${#c}` and then a
// slice of `edge` would still pass; the script's edge is a decimal literal.
func TestRecallHook_WholePromptPatternTrimIsRejected(t *testing.T) {
	// The scanner has to see the operator that sits on the name, including
	// the doubled forms, and it has to ignore a length and a substring.
	require.Equal(t, []string{"candidate#", "head##", "tail%%"}, patternTrimOps(
		`${candidate#x} ${head##y} ${tail%%z} ${#c} ${c:0:3}`,
	))
	require.Equal(t, []string{"QUERY//", "PROMPT/"}, wholePromptSubstOps(
		`${QUERY//a/b} ${PROMPT/a/b} ${QUERY:0:16384} ${#c} ${head//x/y}`,
	))
	require.Equal(t, []string{"QUERY", "c", "PROMPT", "body"}, wholePromptRegexOps(
		`[[ $QUERY =~ x ]] [[ ${c} =~ y ]] [[ "$PROMPT" =~ z ]] [[ "${body}" =~ b ]] [[ $head =~ ok ]]`,
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
		if wholePromptNames[name] || (name != "head" && name != "tail") {
			offenders = append(offenders, "${"+op)
		}
	}
	assert.Empty(t, offenders,
		"pattern trim must name a bounded slice (head, tail), not the whole prompt (candidate, c, PROMPT, QUERY, body, rest)")
	assert.Empty(t, wholePromptSubstOps(text),
		"substitution must not name the whole prompt (candidate, c, PROMPT, QUERY, body, rest)")
	assert.Empty(t, wholePromptRegexOps(text),
		"regex match must not name the whole prompt (candidate, c, PROMPT, QUERY, body, rest)")

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

func wholePromptSubstOps(script string) []string {
	var ops []string
	for _, m := range promptSubst.FindAllStringSubmatch(script, -1) {
		if wholePromptNames[m[1]] {
			ops = append(ops, m[1]+m[2])
		}
	}
	return ops
}

func wholePromptRegexOps(script string) []string {
	var names []string
	for _, m := range promptRegex.FindAllStringSubmatch(script, -1) {
		if wholePromptNames[m[1]] {
			names = append(names, m[1])
		}
	}
	return names
}

// recallUTF8Lang is a locale where bash counts é as one character.
// macOS has en_US.UTF-8. A minimal glibc image has C.UTF-8 or C.utf8.
func recallUTF8Lang(t *testing.T) string {
	t.Helper()
	for _, lang := range []string{"en_US.UTF-8", "C.UTF-8", "C.utf8"} {
		cmd := exec.Command("/bin/bash", "-c", `x=$'é'; [ "${#x}" -eq 1 ]`)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=" + lang}
		if cmd.Run() == nil {
			return lang
		}
	}
	t.Fatal("bash did not count é as one character in any UTF-8 locale")
	return ""
}

// askQueryInLocale runs the recall hook and returns the ask argument.
// lang is the caller locale. The slice that caps QUERY uses it.
func askQueryInLocale(t *testing.T, prompt, lang string) string {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "argv.log")
	h := newHookEnv(t, argvRecordingStub(logPath))
	stdin, err := json.Marshal(map[string]string{"prompt": prompt})
	require.NoError(t, err)
	runHookScript(t, "vault-recall.sh", append(h.env(false), "LANG="+lang), string(stdin))

	raw, err := os.ReadFile(logPath) //nolint:gosec // temp path owned by this test
	require.NoError(t, err, "stub never saw an ask call")
	args := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	require.Greater(t, len(args), 1, "ask called with no query argument")
	require.Equal(t, "ask", args[0])
	return args[1]
}

// The character at index 16383 is é (two bytes). A byte cut of 16384
// would keep the leading 0xC3 and drop 0xA9, which is not valid UTF-8
// and is not a prefix of the query.
func TestRecallHook_AskArgvIsBoundedUTF8Prefix(t *testing.T) {
	lang := recallUTF8Lang(t)
	head := strings.Repeat("a", recallQueryCharCap-1)
	prompt := head + "é" + "é-beyond"
	got := askQueryInLocale(t, prompt, lang)

	require.True(t, utf8.ValidString(got))
	require.LessOrEqual(t, utf8.RuneCountInString(got), recallQueryCharCap)
	require.True(t, strings.HasPrefix(prompt, got), "argv must be a prefix of the query")
	require.Equal(t, head+"é", got)
	last, size := utf8.DecodeLastRuneInString(got)
	require.Equal(t, 'é', last)
	require.Equal(t, 2, size)
}

func TestRecallHook_ShortMultibytePromptArgvUnchanged(t *testing.T) {
	lang := recallUTF8Lang(t)
	prompt := "noise floor — z-score"
	require.Equal(t, prompt, askQueryInLocale(t, prompt, lang))
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
