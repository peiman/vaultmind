package hookscripts_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every hook must parse under the bash an adopter actually has. Stock macOS
// ships bash 3.2 at /bin/bash; the test suite runs whatever `bash` is on PATH
// (Homebrew 5.x on a developer machine). On 2026-09-25 an apostrophe in a
// comment inside a $(cat <<'PY' … PY) heredoc parsed under 5.x and failed under
// 3.2 — which makes a PreToolUse hook exit 2, and Claude Code then BLOCKS every
// tool call the hook matches. Caught by review, not by any test.
const stockMacBash = "/bin/bash"

func TestEveryHookParsesUnderStockMacBash(t *testing.T) {
	if _, err := os.Stat(stockMacBash); err != nil {
		t.Skip("no /bin/bash on this machine")
	}
	names := hookscripts.Names()
	require.NotEmpty(t, names)
	for _, name := range names {
		if !strings.HasSuffix(name, ".sh") {
			continue
		}
		body, ok := hookscripts.Get(name)
		require.True(t, ok, name)
		cmd := exec.Command(stockMacBash, "-n")
		cmd.Stdin = strings.NewReader(string(body))
		out, err := cmd.CombinedOutput()
		assert.NoErrorf(t, err, "%s does not parse under %s: %s", name, stockMacBash, out)
	}
}
