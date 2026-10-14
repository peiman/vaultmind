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

// Reading the tokenizer must not rename the legacy cache. CI fetches
// tokenizer.json into ~/.vaultmind/models, and a test that only tokenizes
// has to leave that file where it was. Resolving the path through
// DefaultCacheDir renames the directory into the XDG cache when the app
// name is set.
func TestBGEM3TokenizerPath_ReadsTheLegacyFileWithoutMovingIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "xdg-cache"))
	prev := xdg.GetAppName()
	xdg.SetAppName("vaultmind")
	t.Cleanup(func() { xdg.SetAppName(prev) })

	legacyDir := filepath.Join(home, ".vaultmind", "models", "BAAI_bge-m3")
	require.NoError(t, os.MkdirAll(legacyDir, 0o750))
	tok := filepath.Join(legacyDir, "tokenizer.json")
	require.NoError(t, os.WriteFile(tok, []byte(`{"pad":true}`), 0o600))

	require.Equal(t, tok, BGEM3TokenizerPath())
	raw, err := ReadBGEM3Tokenizer()
	require.NoError(t, err)
	require.Equal(t, []byte(`{"pad":true}`), raw)
	require.FileExists(t, tok)
	_, err = os.Stat(filepath.Join(home, "xdg-cache", "vaultmind", "models"))
	require.True(t, os.IsNotExist(err), "reading the tokenizer renamed the legacy cache")
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
