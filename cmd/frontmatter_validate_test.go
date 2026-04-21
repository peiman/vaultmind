package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrontmatterValidateLive_ReportsMissingRequiredField(t *testing.T) {
	vault := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(vault, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(vault, ".vaultmind", "config.yaml"), []byte(`
types:
  source:
    required: [url]
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(vault, "bad.md"), []byte(`---
id: s-1
type: source
---
body
`), 0o644))

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	RootCmd.SetOut(out)
	RootCmd.SetErr(errOut)
	RootCmd.SetArgs([]string{"frontmatter", "validate", "--vault", vault, "--live", "--json"})
	require.NoError(t, RootCmd.Execute())

	var env struct {
		Status string `json:"status"`
		Result struct {
			FilesChecked int `json:"files_checked"`
			Valid        int `json:"valid"`
			Issues       []struct {
				Rule  string `json:"rule"`
				Field string `json:"field"`
			} `json:"issues"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "warning", env.Status)
	assert.Equal(t, 1, env.Result.FilesChecked)
	require.Len(t, env.Result.Issues, 1)
	assert.Equal(t, "missing_required_field", env.Result.Issues[0].Rule)
	assert.Equal(t, "url", env.Result.Issues[0].Field)
}

func TestFrontmatterValidateLive_NoIndexRequired(t *testing.T) {
	// Confirms --live works on a vault that has never been indexed
	// (no .vaultmind/index.db file).
	vault := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(vault, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(vault, ".vaultmind", "config.yaml"), []byte(`
types:
  source:
    required: [url]
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(vault, "good.md"), []byte(`---
id: s-2
type: source
url: https://example.com
---
body
`), 0o644))

	out := &bytes.Buffer{}
	RootCmd.SetOut(out)
	RootCmd.SetErr(&bytes.Buffer{})
	RootCmd.SetArgs([]string{"frontmatter", "validate", "--vault", vault, "--live", "--json"})
	require.NoError(t, RootCmd.Execute())

	assert.Contains(t, out.String(), `"status":"ok"`)
	// No index.db was ever created
	_, err := os.Stat(filepath.Join(vault, ".vaultmind", "index.db"))
	assert.True(t, os.IsNotExist(err))
}
