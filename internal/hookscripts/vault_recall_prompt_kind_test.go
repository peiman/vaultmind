package hookscripts_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Sidecar records are metadata. These keys are the whole record: a new
// field has to be added here, and prompt text has no key to land in.
//
// kind is the last field of each printf. The load-bearing mutation drops
// the literal ,"kind":"%s". With kind last, that drop leaves every %d on a
// number, the hook still exits 0, and the assertion fails on the missing
// key. A kind earlier in the format feeds the tag name to %d, and bash -e
// aborts before that assertion runs.
var (
	recallInjectKeys = []string{
		"timestamp", "prompt_len", "ask_status", "injection", "pointer_chars", "kind", "query_chars",
	}
	recallSkipKeys = []string{
		"timestamp", "prompt_len", "ask_status", "injection", "error", "kind", "query_chars",
	}
	recallEnvelopeKeys = []string{
		"timestamp", "prompt_len", "injection", "reason", "kind", "query_chars",
	}
)

const (
	promptKindSentinel       = "SENTINEL9f3a"
	quietVaultmindStub       = "#!/bin/bash\nexit 0\n"
	failingVaultmindStub     = "#!/bin/bash\nif [ \"$1\" = ask ]; then exit 1; fi\nexit 0\n"
	recallPromptKindQuestion = "how does spreading activation work"
)

// Harness tags the recall hook already knows, longest name first so the
// test matches the boundary rule (a shorter name must not win on a longer tag).
var recallHarnessTags = []string{
	"cross-session-message",
	"local-command-caveat",
	"local-command-stdout",
	"task-notification",
	"teammate-message",
	"system-reminder",
	"command-message",
	"agent-message",
	"command-name",
	"command-args",
}

type promptKindRow struct {
	name       string
	prompt     string
	mode       string // inject, empty, fail, envelope
	kind       string
	queryChars int
	reason     string
	secrets    []string
	lang       string // empty keeps the caller locale unset (byte lengths)
}

func TestRecallHook_SidecarRecordsPromptKind(t *testing.T) {
	q := recallPromptKindQuestion
	mention := "why does the recall hook skip a system-reminder payload " + promptKindSentinel
	unknown := "<example>" + promptKindSentinel + " " + q + "</example>"
	longer := "<task-notification-extra>" + promptKindSentinel + " " + q + "</task-notification-extra>"
	prefixQuestion := crossSessionPrefix + q
	prefixThenTag := crossSessionPrefix + "<agent-message>" + promptKindSentinel + "</agent-message>\n" + q
	rows := []promptKindRow{
		{
			name: "prose", prompt: q, mode: "inject", kind: "prose",
			queryChars: len(q), secrets: []string{q},
		},
		{
			name: "prose mentions a tag", prompt: mention, mode: "inject", kind: "prose",
			queryChars: len(mention), secrets: []string{promptKindSentinel, "system-reminder"},
		},
		{
			name: "unknown tag", prompt: unknown, mode: "inject", kind: "prose",
			queryChars: len(unknown), secrets: []string{promptKindSentinel, "<example>", q},
		},
		{
			name: "longer than a harness tag", prompt: longer, mode: "inject", kind: "prose",
			queryChars: len(longer), secrets: []string{promptKindSentinel, "task-notification-extra", q},
		},
		{
			name: "user query", mode: "inject", kind: "user_query",
			prompt:     "<user_query>\n" + q + "\n</user_query>",
			queryChars: len(q),
			secrets:    []string{"<user_query>", q},
		},
		{
			name: "user query after space and BOM", mode: "inject", kind: "user_query",
			prompt:     " \t\ufeff\n<user_query>" + q + "</user_query>",
			queryChars: len(q),
			secrets:    []string{"<user_query>", q},
		},
		{
			name: "meta phrases stripped", mode: "inject", kind: "prose",
			prompt:     "can you find our notes about spreading activation",
			queryChars: len("spreading activation"),
			secrets:    []string{"can you find our notes", "spreading activation"},
		},
		{
			name: "transport cap", mode: "inject", kind: "prose",
			prompt:     strings.Repeat("a", recallQueryCharCap) + promptKindSentinel,
			queryChars: recallQueryCharCap,
			secrets:    []string{promptKindSentinel, strings.Repeat("a", 64)},
		},
		{
			name: "cap keeps a whole character", mode: "inject", kind: "prose",
			prompt:     strings.Repeat("a", recallQueryCharCap-1) + "é" + "é-beyond",
			queryChars: recallQueryCharCap,
			secrets:    []string{"é-beyond", "beyond"},
			lang:       recallUTF8Lang(t),
		},
		{
			name: "empty ask still records kind", mode: "empty", kind: "prose",
			prompt: q, queryChars: len(q), secrets: []string{q},
		},
		{
			name: "failed ask still records kind", mode: "fail", kind: "prose",
			prompt: q, queryChars: len(q), secrets: []string{q},
		},
		{
			name: "cross-session prefix then a question", prompt: prefixQuestion, mode: "inject", kind: "prose",
			queryChars: len(prefixQuestion), secrets: []string{"Another Claude", q},
		},
		{
			name: "cross-session prefix then a tag and a question", prompt: prefixThenTag, mode: "inject", kind: "agent-message",
			queryChars: len(prefixThenTag), secrets: []string{promptKindSentinel, "Another Claude", q},
		},
		{
			name: "first tag wins over a later user query", mode: "inject", kind: "system-reminder",
			prompt:     "<system-reminder>" + promptKindSentinel + "</system-reminder>\n<user_query>" + q + "</user_query>",
			queryChars: len(q),
			secrets:    []string{promptKindSentinel, q, "<user_query>"},
		},
		{
			name: "indented task notification", mode: "envelope", kind: "task-notification",
			reason: "harness-envelope", queryChars: 0,
			prompt:  " \t\n<task-notification>" + promptKindSentinel + "</task-notification>\n\t",
			secrets: []string{promptKindSentinel},
		},
		{
			name: "BOM then task notification", mode: "envelope", kind: "task-notification",
			reason: "harness-envelope", queryChars: 0,
			prompt:  " \t\ufeff\n<task-notification>" + promptKindSentinel + "</task-notification>",
			secrets: []string{promptKindSentinel},
		},
		{
			name: "two envelopes, first tag", mode: "envelope", kind: "system-reminder",
			reason: "harness-envelope", queryChars: 0,
			prompt:  "<system-reminder>remember</system-reminder>\n<task-notification>" + promptKindSentinel + "</task-notification>",
			secrets: []string{promptKindSentinel, "remember"},
		},
		{
			name: "cross-session hand-back", mode: "envelope", kind: "teammate-message",
			reason: "cross-session-envelope", queryChars: 0,
			prompt:  " \t\n" + crossSessionPrefix + idleEnvelope() + crossSessionSuffixPeer,
			secrets: []string{"standing by", "Another Claude", "permission laundering"},
		},
	}
	for _, tag := range recallHarnessTags {
		rows = append(rows, promptKindRow{
			name: tag, mode: "envelope", kind: tag, reason: "harness-envelope",
			prompt:  "<" + tag + ">" + promptKindSentinel + " done</" + tag + ">",
			secrets: []string{promptKindSentinel},
		})
	}

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			// A failing ask is the pre-existing path: the hook records the status
			// and exits 0. bash -e aborts at that assignment before the
			// record is written; the harness does not run this script -e.
			h, out, _ := runPromptKindHook(t, tc.prompt, promptKindStub(tc.mode), tc.lang, tc.mode != "fail")
			entry, raw := promptKindRecord(t, h.home)
			assertPromptKindRecord(t, entry, raw, tc.kind, float64(tc.queryChars), promptKindKeys(tc.mode), tc.secrets)
			switch tc.mode {
			case "inject":
				assert.Equal(t, true, entry["injection"])
				assert.Equal(t, float64(0), entry["ask_status"])
				assert.Contains(t, out, "some-note")
			case "empty":
				assert.Equal(t, false, entry["injection"])
				assert.Equal(t, float64(0), entry["ask_status"])
				assert.Equal(t, "", entry["error"])
				assert.Empty(t, strings.TrimSpace(out))
			case "fail":
				assert.Equal(t, false, entry["injection"])
				assert.Equal(t, float64(1), entry["ask_status"])
				assert.Equal(t, "", entry["error"])
				assert.Contains(t, out, "NOT consulted")
			case "envelope":
				assert.Equal(t, false, entry["injection"])
				assert.Equal(t, tc.reason, entry["reason"])
				assert.Equal(t, float64(0), entry["query_chars"])
				assert.Empty(t, strings.TrimSpace(out))
			default:
				t.Fatalf("unknown mode %q", tc.mode)
			}
		})
	}
}

func TestRecallHook_PromptKindLargeStaysFast(t *testing.T) {
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("no /bin/bash")
	}
	open, closeTag := "<user_query>", "</user_query>"
	userQuery := open + strings.Repeat("q", largePromptBytes-len(open)-len(closeTag)) + closeTag
	require.Equal(t, largePromptBytes, len(userQuery))
	cases := []struct {
		name       string
		prompt     string
		kind       string
		queryChars float64
		keys       []string
		leak       string
	}{
		{"user query", userQuery, "user_query", float64(recallQueryCharCap), recallInjectKeys, strings.Repeat("q", 80)},
		{"prose", strings.Repeat("p", largePromptBytes), "prose", float64(recallQueryCharCap), recallInjectKeys, strings.Repeat("p", 80)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, largePromptBytes, len(tc.prompt))
			h, _, elapsed := runPromptKindHook(t, tc.prompt, fastStub, "", true)
			t.Logf("%s: %d bytes in %s", tc.name, len(tc.prompt), elapsed)
			assert.Less(t, elapsed, largePromptBudget, "kind must be read from a bounded head")
			entry, raw := promptKindRecord(t, h.home)
			assertPromptKindRecord(t, entry, raw, tc.kind, tc.queryChars, tc.keys, []string{tc.leak})
		})
	}
}

func promptKindStub(mode string) string {
	switch mode {
	case "empty":
		return quietVaultmindStub
	case "fail":
		return failingVaultmindStub
	default:
		return fastStub
	}
}

func promptKindKeys(mode string) []string {
	switch mode {
	case "inject":
		return recallInjectKeys
	case "empty", "fail":
		return recallSkipKeys
	case "envelope":
		return recallEnvelopeKeys
	default:
		return nil
	}
}

func runPromptKindHook(t *testing.T, prompt, stub, lang string, errexit bool) (hookEnv, string, time.Duration) {
	t.Helper()
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("no /bin/bash")
	}
	h := newHookEnv(t, stub)
	payload, err := json.Marshal(map[string]string{"prompt": prompt, "session_id": "prompt-kind"})
	require.NoError(t, err)
	// -e matches the envelope tests. The ask-failure record is written
	// after a non-zero ask, which bash -e does not reach.
	script := "umask 022; exec /bin/bash vault-recall.sh"
	if errexit {
		script = "umask 022; exec /bin/bash -e vault-recall.sh"
	}
	cmd := exec.Command("/bin/bash", "-c", script)
	env := h.env(false)
	if lang != "" {
		env = append(env, "LANG="+lang)
	}
	cmd.Env = env
	cmd.Stdin = strings.NewReader(string(payload))
	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	require.NoError(t, err, "hook must exit 0: %s", out)
	return h, string(out), elapsed
}

func promptKindRecord(t *testing.T, home string) (map[string]any, []byte) {
	t.Helper()
	dir := filepath.Join(home, ".vaultmind", "userprompt-hook")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var files []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".jsonl") {
			files = append(files, name)
		}
	}
	require.Len(t, files, 1, "one sidecar record, got %v", files)
	raw, err := os.ReadFile(filepath.Join(dir, files[0])) //nolint:gosec // test-owned home
	require.NoError(t, err)
	line := strings.TrimSpace(string(raw))
	if i := strings.LastIndex(line, "\n"); i >= 0 {
		line = line[i+1:]
	}
	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &entry), "sidecar must be one JSON object: %s", line)
	return entry, raw
}

func assertPromptKindRecord(t *testing.T, entry map[string]any, raw []byte, kind string, queryChars float64, keys, secrets []string) {
	t.Helper()
	assert.Equal(t, kind, entry["kind"])
	assert.Equal(t, queryChars, entry["query_chars"])
	assertSidecarKeys(t, entry, keys)
	for _, secret := range secrets {
		assert.NotContains(t, string(raw), secret)
	}
	got, ok := entry["kind"].(string)
	require.True(t, ok, "kind must be a string")
	assert.NotContains(t, got, " ")
	assert.Less(t, len(got), 40, "kind is a tag name, not prompt text")
}

func assertSidecarKeys(t *testing.T, entry map[string]any, allowed []string) {
	t.Helper()
	got := make([]string, 0, len(entry))
	for k := range entry {
		got = append(got, k)
	}
	sort.Strings(got)
	want := append([]string(nil), allowed...)
	sort.Strings(want)
	assert.Equal(t, want, got, "the record holds only metadata keys")
}
