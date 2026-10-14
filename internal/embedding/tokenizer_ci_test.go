package embedding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/stretchr/testify/require"
)

func TestTokenizerMissIsFatal(t *testing.T) {
	tests := []struct {
		name string
		ci   string
		want bool
	}{
		{name: "github actions", ci: "true", want: true},
		{name: "explicit false", ci: "false", want: false},
		{name: "numeric", ci: "1", want: false},
		{name: "empty", ci: "", want: false},
		{name: "uppercase", ci: "TRUE", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, TokenizerMissIsFatal(tc.ci))
		})
	}
}

func TestBGEM3Tokenizer_LegacyPathAndHint(t *testing.T) {
	prev := xdg.GetAppName()
	xdg.SetAppName("")
	t.Cleanup(func() { xdg.SetAppName(prev) })
	t.Setenv("HOME", t.TempDir())

	path := BGEM3TokenizerPath()
	require.Contains(t, filepath.ToSlash(path), ".vaultmind/models/BAAI_bge-m3/tokenizer.json")
	_, err := ReadBGEM3Tokenizer()
	require.Error(t, err)

	rev, sum, url := BGEM3TokenizerPin()
	hint := TokenizerFetchHint()
	require.NotEmpty(t, sum)
	require.Contains(t, hint, path)
	require.Contains(t, hint, sum)
	require.Contains(t, hint, rev)
	require.Contains(t, hint, url)
	require.NotContains(t, hint, "model.onnx_data")
	require.Contains(t, hint, "not the model weights")
}

// TestCIPinsBGEM3TokenizerFetch locks the workflow to the pin in bgem3Files.
// The shell script repeats the hash because parsing Go from bash is brittle.
// This test is the check that the two cannot drift.
func TestCIPinsBGEM3TokenizerFetch(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	script, err := os.ReadFile(filepath.Join(root, ".github", "scripts", "fetch-bge-m3-tokenizer.sh")) //nolint:gosec // repo file
	require.NoError(t, err)
	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml")) //nolint:gosec // repo file
	require.NoError(t, err)

	rev, sum, _ := BGEM3TokenizerPin()
	text := string(script)
	require.Contains(t, text, sum)
	require.Contains(t, text, rev)
	require.Contains(t, text, "sha256sum")
	require.NotContains(t, text, "model.onnx_data")

	yml := string(workflow)
	for _, step := range []string{
		"task check:fast\n",
		"task test:race\n",
		"task test\n",
		"task test:dev\n",
		"task test:prod\n",
	} {
		idx := strings.Index(yml, step)
		require.NotEqualf(t, -1, idx, "missing %q", step)
		require.Greaterf(t, strings.LastIndex(yml[:idx], "fetch-bge-m3-tokenizer.sh"), -1, "tokenizer fetch must precede %s", step)
	}
}
