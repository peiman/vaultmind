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

// A *.md symlink is never followed (#194): in a vault you did not write,
// `secrets.md -> ~/.ssh/id_rsa` would otherwise be read and parsed, and its
// parse error could echo what it read. The link is reported, not dropped —
// a file passed over and a file never there must not look the same.
func TestValidateLive_NeverFollowsASymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(outside, []byte("---\nid: [SECRET-CONTENT\n---\n"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "secrets.md")))
	writeNote(t, dir, "ok.md", "---\nid: n-1\ntype: note\n---\nbody\n")

	res, err := ValidateLive(dir, buildRegistry(t))
	require.NoError(t, err)
	assert.Equal(t, 1, res.FilesChecked, "only the real note is read")
	require.Len(t, res.Issues, 1)
	assert.Equal(t, RuleSkippedSymlink, res.Issues[0].Rule)
	assert.Equal(t, "warning", res.Issues[0].Severity)
	assert.Equal(t, filepath.Join(dir, "secrets.md"), res.Issues[0].Path, "the same path form as the other issues")
	assert.NotContains(t, res.Issues[0].Message, "SECRET-CONTENT")
}

// A vault named through a symlink (a "current" pointer, say) is the
// operator's own choice of path and is validated, not reported as empty:
// WalkDir does not descend into a root that is a link, and a green
// "Checked 0 files" for a vault full of problems is a false clean.
func TestValidateLive_ValidatesAVaultNamedThroughASymlink(t *testing.T) {
	real := t.TempDir()
	writeNote(t, real, "bad.md", "---\nid: [unclosed\n---\n")
	link := filepath.Join(t.TempDir(), "current")
	require.NoError(t, os.Symlink(real, link))

	res, err := ValidateLive(link, buildRegistry(t))
	require.NoError(t, err)
	assert.Equal(t, 1, res.FilesChecked)
	require.Len(t, res.Issues, 1)
	assert.Equal(t, "invalid_frontmatter", res.Issues[0].Rule)
}
