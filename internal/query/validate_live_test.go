package query

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildRegistry returns a schema.Registry preloaded with a "source" type that
// requires url and a "note" type with a fixed status enum.
func buildRegistry(t *testing.T) *schema.Registry {
	t.Helper()
	return schema.NewRegistry(map[string]vault.TypeDef{
		"source": {Required: []string{"url"}},
		"note":   {Required: []string{}, Statuses: []string{"draft", "final"}},
	})
}

func writeNote(t *testing.T, dir, name, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
}

func TestValidateLive_ValidVault(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "source-ok.md", `---
id: src-1
type: source
url: https://example.com
---
body
`)

	res, err := ValidateLive(dir, buildRegistry(t))
	require.NoError(t, err)
	assert.Equal(t, 1, res.FilesChecked)
	assert.Equal(t, 1, res.Valid)
	assert.Empty(t, res.Issues)
}

func TestValidateLive_MissingRequiredField(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "source-bad.md", `---
id: src-2
type: source
---
body
`)

	res, err := ValidateLive(dir, buildRegistry(t))
	require.NoError(t, err)
	assert.Equal(t, 1, res.FilesChecked)
	assert.Equal(t, 0, res.Valid)
	require.Len(t, res.Issues, 1)
	assert.Equal(t, "missing_required_field", res.Issues[0].Rule)
	assert.Equal(t, "url", res.Issues[0].Field)
}

func TestValidateLive_UnknownType(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "weird.md", `---
id: w-1
type: gibberish
---
body
`)

	res, err := ValidateLive(dir, buildRegistry(t))
	require.NoError(t, err)
	require.Len(t, res.Issues, 1)
	assert.Equal(t, "unknown_type", res.Issues[0].Rule)
	assert.Equal(t, "gibberish", res.Issues[0].Value)
}

func TestValidateLive_InvalidStatus(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "note.md", `---
id: n-1
type: note
status: wip
---
body
`)

	res, err := ValidateLive(dir, buildRegistry(t))
	require.NoError(t, err)
	require.Len(t, res.Issues, 1)
	assert.Equal(t, "invalid_status", res.Issues[0].Rule)
	assert.Equal(t, "wip", res.Issues[0].Value)
}

func TestValidateLive_NonDomainNotesSkipped(t *testing.T) {
	dir := t.TempDir()
	// No id+type → not a domain note
	writeNote(t, dir, "casual.md", `---
title: Just a note
---
body
`)

	res, err := ValidateLive(dir, buildRegistry(t))
	require.NoError(t, err)
	assert.Equal(t, 1, res.FilesChecked)
	assert.Equal(t, 1, res.Valid)
	assert.Empty(t, res.Issues)
}

func TestValidateLive_IgnoresDotDirsAndNonMarkdown(t *testing.T) {
	dir := t.TempDir()
	// Hidden dir (e.g. .vaultmind) should be skipped entirely
	hidden := filepath.Join(dir, ".vaultmind")
	require.NoError(t, os.Mkdir(hidden, 0o755))
	writeNote(t, hidden, "config.md", `---
id: x
type: source
---`)
	// Non-.md file
	writeNote(t, dir, "notes.txt", "just text")

	res, err := ValidateLive(dir, buildRegistry(t))
	require.NoError(t, err)
	assert.Equal(t, 0, res.FilesChecked)
}

func TestValidateLive_InvalidFrontmatterReported(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "broken.md", `---
id: b-1
type: source
url: [unterminated
---
body
`)

	res, err := ValidateLive(dir, buildRegistry(t))
	require.NoError(t, err)
	require.NotEmpty(t, res.Issues)
	assert.Equal(t, "invalid_frontmatter", res.Issues[0].Rule)
}

func TestValidateLive_VaultNotFound(t *testing.T) {
	_, err := ValidateLive("/nonexistent/path/that/does/not/exist", buildRegistry(t))
	require.Error(t, err)
}
