// cmd/note_create_test.go

package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupNoteCreateVault creates a temp vault with the given config YAML and
// optional template files. Returns the vault path.
func setupNoteCreateVault(t *testing.T, configYAML string, templates map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".vaultmind")
	require.NoError(t, os.MkdirAll(configDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0o644))

	for relPath, content := range templates {
		absPath := filepath.Join(dir, relPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(absPath), 0o755))
		require.NoError(t, os.WriteFile(absPath, []byte(content), 0o644))
	}
	return dir
}

// newNoteCreateCmd returns a minimal cobra.Command wired to executeNoteCreate
// with required flags registered, using viper for config.
func newNoteCreateCmd(t *testing.T) *cobra.Command {
	t.Helper()
	c := &cobra.Command{Use: "create"}
	c.Flags().String("vault", "", "vault path")
	c.Flags().Bool("json", false, "json output")
	c.Flags().String("type", "", "note type")
	c.Flags().String("body", "", "body override")
	c.Flags().Bool("commit", false, "commit")
	c.Flags().StringSlice("field", nil, "frontmatter field overrides")
	return c
}

// ─── C1: Path traversal validation ───────────────────────────────────────────

func TestExecuteNoteCreate_PathTraversal_Rejected(t *testing.T) {
	vaultCfg := `types:
  note:
    required: []
    optional: []
`
	dir := setupNoteCreateVault(t, vaultCfg, nil)

	viper.Reset()
	defer viper.Reset()
	viper.Set("app.notecreate.vault", dir)
	viper.Set("app.notecreate.json", false)
	viper.Set("app.notecreate.type", "note")
	viper.Set("app.notecreate.body", "")
	viper.Set("app.notecreate.commit", false)

	cmd := newNoteCreateCmd(t)
	outBuf := &bytes.Buffer{}
	cmd.SetOut(outBuf)

	err := executeNoteCreate(cmd, "../escaped.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path traversal")
}

func TestExecuteNoteCreate_PathTraversal_AbsoluteEscapeRejected(t *testing.T) {
	vaultCfg := `types:
  note:
    required: []
    optional: []
`
	dir := setupNoteCreateVault(t, vaultCfg, nil)

	viper.Reset()
	defer viper.Reset()
	viper.Set("app.notecreate.vault", dir)
	viper.Set("app.notecreate.json", false)
	viper.Set("app.notecreate.type", "note")
	viper.Set("app.notecreate.body", "")
	viper.Set("app.notecreate.commit", false)

	// Attempt to escape using ../ chain
	cmd := newNoteCreateCmd(t)
	outBuf := &bytes.Buffer{}
	cmd.SetOut(outBuf)

	err := executeNoteCreate(cmd, "sub/../../outside.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path traversal")
}

func TestExecuteNoteCreate_PathTraversal_ValidPathAccepted(t *testing.T) {
	vaultCfg := `types:
  note:
    required: []
    optional: []
    template: .vaultmind/templates/note.md
`
	tmpl := "---\nid: <%=id%>\ntype: <%=type%>\ntitle: <%=title%>\ncreated: <%=created%>\nvm_updated: <%=vm_updated%>\n---\n"
	dir := setupNoteCreateVault(t, vaultCfg, map[string]string{
		".vaultmind/templates/note.md": tmpl,
	})

	viper.Reset()
	defer viper.Reset()
	viper.Set("app.notecreate.vault", dir)
	viper.Set("app.notecreate.json", false)
	viper.Set("app.notecreate.type", "note")
	viper.Set("app.notecreate.body", "")
	viper.Set("app.notecreate.commit", false)

	cmd := newNoteCreateCmd(t)
	outBuf := &bytes.Buffer{}
	cmd.SetOut(outBuf)

	// A normal subdirectory path must succeed
	err := executeNoteCreate(cmd, "notes/valid.md")
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "notes/valid.md")
}

// ─── C2: write_hash is SHA-256 of content, not DB file hash ──────────────────

func TestExecuteNoteCreate_WriteHash_IsContentSHA256(t *testing.T) {
	vaultCfg := `types:
  project:
    required: []
    optional: []
    template: .vaultmind/templates/project.md
`
	tmpl := "---\nid: <%=id%>\ntype: <%=type%>\ntitle: <%=title%>\ncreated: <%=created%>\nvm_updated: <%=vm_updated%>\n---\n"
	dir := setupNoteCreateVault(t, vaultCfg, map[string]string{
		".vaultmind/templates/project.md": tmpl,
	})

	viper.Reset()
	defer viper.Reset()
	viper.Set("app.notecreate.vault", dir)
	viper.Set("app.notecreate.json", true)
	viper.Set("app.notecreate.type", "project")
	viper.Set("app.notecreate.body", "")
	viper.Set("app.notecreate.commit", false)

	cmd := newNoteCreateCmd(t)
	outBuf := &bytes.Buffer{}
	cmd.SetOut(outBuf)

	err := executeNoteCreate(cmd, "projects/test-hash.md")
	require.NoError(t, err)

	var env map[string]interface{}
	require.NoError(t, json.Unmarshal(outBuf.Bytes(), &env))

	result, ok := env["result"].(map[string]interface{})
	require.True(t, ok, "expected 'result' key in JSON envelope")

	writeHash, ok := result["write_hash"].(string)
	require.True(t, ok, "write_hash must be a string")
	assert.True(t, strings.HasPrefix(writeHash, "sha256:"), "write_hash must start with 'sha256:' prefix, got: %q", writeHash)
}

// ─── I1: Required field validation ───────────────────────────────────────────

func TestExecuteNoteCreate_RequiredFieldMissing_ReturnsError(t *testing.T) {
	vaultCfg := `types:
  project:
    required: [goal]
    optional: []
    template: .vaultmind/templates/project.md
`
	// Template does NOT include the required "goal" field
	tmpl := "---\nid: <%=id%>\ntype: <%=type%>\ntitle: <%=title%>\ncreated: <%=created%>\nvm_updated: <%=vm_updated%>\n---\n"
	dir := setupNoteCreateVault(t, vaultCfg, map[string]string{
		".vaultmind/templates/project.md": tmpl,
	})

	viper.Reset()
	defer viper.Reset()
	viper.Set("app.notecreate.vault", dir)
	viper.Set("app.notecreate.json", false)
	viper.Set("app.notecreate.type", "project")
	viper.Set("app.notecreate.body", "")
	viper.Set("app.notecreate.commit", false)

	cmd := newNoteCreateCmd(t)
	outBuf := &bytes.Buffer{}
	cmd.SetOut(outBuf)

	// Template does not include "goal", and user hasn't provided it via --field
	err := executeNoteCreate(cmd, "projects/missing-goal.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "goal")
}

func TestExecuteNoteCreate_RequiredFieldProvided_Succeeds(t *testing.T) {
	vaultCfg := `types:
  project:
    required: [goal]
    optional: []
    template: .vaultmind/templates/project.md
`
	// Template includes the required "goal" field
	tmpl := "---\nid: <%=id%>\ntype: <%=type%>\ntitle: <%=title%>\ncreated: <%=created%>\nvm_updated: <%=vm_updated%>\ngoal: TBD\n---\n"
	dir := setupNoteCreateVault(t, vaultCfg, map[string]string{
		".vaultmind/templates/project.md": tmpl,
	})

	viper.Reset()
	defer viper.Reset()
	viper.Set("app.notecreate.vault", dir)
	viper.Set("app.notecreate.json", false)
	viper.Set("app.notecreate.type", "project")
	viper.Set("app.notecreate.body", "")
	viper.Set("app.notecreate.commit", false)

	cmd := newNoteCreateCmd(t)
	outBuf := &bytes.Buffer{}
	cmd.SetOut(outBuf)

	err := executeNoteCreate(cmd, "projects/with-goal.md")
	require.NoError(t, err)
	assert.Contains(t, outBuf.String(), "projects/with-goal.md")
}
