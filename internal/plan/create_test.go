package plan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateNote_Basic(t *testing.T) {
	dir := t.TempDir()
	op := Operation{
		Op: OpNoteCreate, Path: "decisions/test-decision.md", Type: "decision",
		Frontmatter: map[string]interface{}{"title": "Test Decision", "status": "proposed"},
		Body:        "## Context\n\nSome context here.\n",
	}
	result, err := CreateNote(dir, op)
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
	assert.Equal(t, "decisions/test-decision.md", result.Path)
	assert.NotEmpty(t, result.WriteHash)
	assert.NotEmpty(t, result.ID)

	content, err := os.ReadFile(filepath.Join(dir, "decisions/test-decision.md"))
	require.NoError(t, err)
	s := string(content)
	assert.Contains(t, s, "---")
	assert.Contains(t, s, "title: Test Decision")
	assert.Contains(t, s, "type: decision")
	assert.Contains(t, s, "## Context")
}

func TestCreateNote_WithID(t *testing.T) {
	dir := t.TempDir()
	op := Operation{
		Op: OpNoteCreate, Path: "decisions/custom.md", Type: "decision",
		Frontmatter: map[string]interface{}{"id": "custom-id", "title": "Custom"},
	}
	result, err := CreateNote(dir, op)
	require.NoError(t, err)
	assert.Equal(t, "custom-id", result.ID)
}

func TestCreateNote_DeriveID(t *testing.T) {
	dir := t.TempDir()
	op := Operation{
		Op: OpNoteCreate, Path: "decisions/pause-billing.md", Type: "decision",
		Frontmatter: map[string]interface{}{"title": "Pause Billing"},
	}
	result, err := CreateNote(dir, op)
	require.NoError(t, err)
	assert.Equal(t, "decision-pause-billing", result.ID)
}

func TestCreateNote_PathExists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "existing.md"), []byte("x"), 0o644))
	op := Operation{Op: OpNoteCreate, Path: "existing.md", Type: "decision", Frontmatter: map[string]interface{}{"title": "T"}}
	_, err := CreateNote(dir, op)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path_exists")
}

func TestCreateNote_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	op := Operation{Op: OpNoteCreate, Path: "../../etc/evil.md", Type: "decision", Frontmatter: map[string]interface{}{"title": "T"}}
	_, err := CreateNote(dir, op)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path_traversal")
}

func TestCreateNote_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	op := Operation{Op: OpNoteCreate, Path: "deep/nested/note.md", Type: "decision", Frontmatter: map[string]interface{}{"title": "Nested"}}
	result, err := CreateNote(dir, op)
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
	_, err = os.Stat(filepath.Join(dir, "deep/nested/note.md"))
	assert.NoError(t, err)
}

func TestCreateNote_EmptyBody(t *testing.T) {
	dir := t.TempDir()
	op := Operation{Op: OpNoteCreate, Path: "note.md", Type: "decision", Frontmatter: map[string]interface{}{"title": "No Body"}}
	result, err := CreateNote(dir, op)
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Status)
	content, _ := os.ReadFile(filepath.Join(dir, "note.md"))
	assert.Contains(t, string(content), "title: No Body")
}
