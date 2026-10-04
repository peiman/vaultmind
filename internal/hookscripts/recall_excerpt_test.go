package hookscripts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The recall hook asks for 160-token excerpts. An excerpt is a note's lead
// paragraph (or its Principle); at 80 tokens the cut landed on the answer:
// on the long-doc eval 3/12 answers reached the agent at 80 and 5/12 at 160,
// the most any length reached, for 1.18x the tokens per prompt (median, 32
// real queries). Record: hook-excerpt-length-2026-10-04 (private repo).
func TestRecallHook_AsksFor160TokenExcerpts(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "argv.log")
	h := newHookEnv(t, argvRecordingStub(logPath))
	stdin, err := json.Marshal(map[string]string{"prompt": "how does spreading activation work"})
	require.NoError(t, err)
	runHookScript(t, "vault-recall.sh", h.env(false), string(stdin))

	raw, err := os.ReadFile(logPath) //nolint:gosec // temp path owned by this test
	require.NoError(t, err, "the hook never asked the vault")
	args := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	for i, a := range args {
		if a == "--excerpt" {
			require.Less(t, i+1, len(args))
			require.Equal(t, "160", args[i+1])
			return
		}
	}
	t.Fatalf("no --excerpt in %v", args)
}
