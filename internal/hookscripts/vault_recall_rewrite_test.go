package hookscripts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A3 (2026-08-28): connective retrieval phrases in a conversational prompt
// carry no ranking signal and measurably depress it — probed raw-vs-rewritten
// against a frozen 12/4/4 set, "can you find our notes about spreading
// activation" ranked the right note 3rd while the stripped query ranks it 1st
// (Δz +2.59). The shipped rewrite is META-ONLY by measurement: the variant
// that also stripped interrogative frames ("why is…", "how do I…") was the
// only source of rank regressions in the probe, so frames stay. Probe spec,
// frozen set, and the decision record live in the private repo under
// docs/reviews/a3-query-rewrite/.
//
// These tests run the hook end-to-end with a stub vaultmind that records its
// argv, and assert on the query the binary actually receives — the behaviour,
// not the script text (a text assertion here once pinned a vulnerability in
// place verbatim; not repeating that).

// argvRecordingStub records every argument of the first `ask` invocation,
// one per line, then answers like fastStub so the hook completes normally.
func argvRecordingStub(logPath string) string {
	return "#!/bin/bash\n" +
		"if [ \"$1\" = ask ]; then printf '%s\\n' \"$@\" > " + logPath + "; echo '  0.42  some-note   A Note'; fi\n" +
		"exit 0\n"
}

// askQueryReceived runs vault-recall.sh with the given user prompt and returns
// the query string the stub binary received as the argument after `ask`.
func askQueryReceived(t *testing.T, prompt string) string {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "argv.log")
	h := newHookEnv(t, argvRecordingStub(logPath))

	stdin, err := json.Marshal(map[string]string{"prompt": prompt})
	require.NoError(t, err)
	runHookScript(t, "vault-recall.sh", h.env(false), string(stdin))

	raw, err := os.ReadFile(logPath) //nolint:gosec // temp path owned by this test
	require.NoError(t, err, "stub never saw an ask call for prompt %q", prompt)
	args := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	require.Greater(t, len(args), 1, "ask called with no query argument")
	require.Equal(t, "ask", args[0])
	return args[1]
}

func TestRecallHook_StripsRetrievalMetaPhrasesBeforeAsking(t *testing.T) {
	got := askQueryReceived(t, "can you find our notes about spreading activation")
	require.Equal(t, "spreading activation", got,
		"retrieval-meta phrases must be stripped — this exact case moved the target from rank 3 to rank 1 in the probe")
}

func TestRecallHook_KeepsInterrogativeFramesIntact(t *testing.T) {
	prompt := "why is the doctor command so slow"
	require.Equal(t, prompt, askQueryReceived(t, prompt),
		"frames are signal-bearing prose: the frame-stripping variant caused the probe's only rank regressions")
}

func TestRecallHook_DeclarativeQueryPassesThroughByteIdentical(t *testing.T) {
	prompt := "noise floor z-score delivery decisions"
	require.Equal(t, prompt, askQueryReceived(t, prompt))
}

func TestRecallHook_FallsBackToRawPromptWhenStrippingLeavesTooLittle(t *testing.T) {
	prompt := "please look up sourdough"
	require.Equal(t, prompt, askQueryReceived(t, prompt),
		"a rewrite that empties the query must never replace it — fewer than 2 words left means use the raw prompt")
}
