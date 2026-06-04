package cmdutil_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LoadRegistry must read a vault's config.yaml without opening the index DB,
// and the returned Registry must know the types defined there. This is the
// contract --live frontmatter validation depends on.
func TestLoadRegistry_ReadsTypesWithoutIndex(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  source:
    required: [url]
  note:
    required: [title]
`), 0o644))

	reg, err := cmdutil.LoadRegistry(dir)
	require.NoError(t, err)
	assert.True(t, reg.HasType("source"))
	assert.True(t, reg.HasType("note"))
	assert.False(t, reg.HasType("does_not_exist"))

	// No index.db was opened
	_, statErr := os.Stat(filepath.Join(dir, ".vaultmind", "index.db"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestLoadRegistry_BadPathErrors(t *testing.T) {
	_, err := cmdutil.LoadRegistry("/nonexistent/directory/really")
	require.Error(t, err)
}

// LoadRegistry must report a clear error when the target is not a directory
// (e.g. a bare file path passed accidentally).
func TestLoadRegistry_FileInsteadOfDirErrors(t *testing.T) {
	f := filepath.Join(t.TempDir(), "notadir.md")
	require.NoError(t, os.WriteFile(f, []byte("body"), 0o644))

	_, err := cmdutil.LoadRegistry(f)
	require.Error(t, err)
}

// WriteJSON must produce the envelope with status=ok, command name, and the
// arbitrary result payload round-tripped. Downstream parsers rely on this
// exact shape.
func TestWriteJSON_RoundTripsResultAndMeta(t *testing.T) {
	var buf bytes.Buffer
	payload := map[string]any{"hits": 3, "query": "Alpha"}
	require.NoError(t, cmdutil.WriteJSON(&buf, "search", payload, "/vaults/a", "deadbeef"))

	var env struct {
		Status  string         `json:"status"`
		Command string         `json:"command"`
		Result  map[string]any `json:"result"`
		Meta    struct {
			VaultPath string `json:"vault_path"`
			IndexHash string `json:"index_hash"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "search", env.Command)
	assert.Equal(t, float64(3), env.Result["hits"])
	assert.Equal(t, "Alpha", env.Result["query"])
	assert.Equal(t, "/vaults/a", env.Meta.VaultPath)
	assert.Equal(t, "deadbeef", env.Meta.IndexHash)
}
