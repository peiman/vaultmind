package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_DomainNote(t *testing.T) {
	content := []byte("---\nid: proj-payment-retries\ntype: project\nstatus: active\ntitle: Payment Retries\naliases:\n  - Retry Engine\ncreated: 2026-04-03\nvm_updated: 2026-04-03\ntags:\n  - billing\nrelated_ids:\n  - concept-idempotency\n---\n\n# Payment Retries\n\nThis project covers retry logic. See [[Idempotency]] and [[Billing Service|the billing service]].\n\n## Rationale\n\nWe need robust retries. ^rationale-block\n\n## Implementation\n\nSee ![[architecture-diagram.png]] for the overview.")

	note, err := parser.Parse(content)
	require.NoError(t, err)

	assert.True(t, note.IsDomain)
	assert.Equal(t, "proj-payment-retries", note.ID)
	assert.Equal(t, "project", note.NoteType)
	assert.Equal(t, "Payment Retries", note.Frontmatter["title"])

	linkTargets := make([]string, len(note.Links))
	for i, l := range note.Links {
		linkTargets[i] = l.Target
	}
	assert.Contains(t, linkTargets, "Idempotency")
	assert.Contains(t, linkTargets, "Billing Service")
	assert.Contains(t, linkTargets, "architecture-diagram.png")

	assert.Len(t, note.Headings, 3)
	assert.Equal(t, "Payment Retries", note.Headings[0].Title)

	require.Len(t, note.Blocks, 1)
	assert.Equal(t, "rationale-block", note.Blocks[0].BlockID)
	assert.Equal(t, "Rationale", note.Blocks[0].Heading)

	assert.Contains(t, note.Body, "# Payment Retries")
	assert.NotEmpty(t, note.FTSBody)
	assert.NotContains(t, note.FTSBody, "[[")
}

func TestParse_UnstructuredNote(t *testing.T) {
	content := []byte("This is your new vault.\n\nMake a note of something, [[create a link]], or try [the Importer](https://help.obsidian.md/Plugins/Importer)!\n\nWhen you're ready, delete this note.")

	note, err := parser.Parse(content)
	require.NoError(t, err)

	assert.False(t, note.IsDomain)
	assert.Empty(t, note.ID)
	assert.Empty(t, note.NoteType)
	assert.Empty(t, note.Frontmatter)

	targets := make([]string, len(note.Links))
	for i, l := range note.Links {
		targets[i] = l.Target
	}
	assert.Contains(t, targets, "create a link")
	assert.Contains(t, targets, "https://help.obsidian.md/Plugins/Importer")
}

func TestParse_NoteWithIDButNoType(t *testing.T) {
	content := []byte("---\nid: orphan-note\ntitle: Orphan Note\n---\n\nThis note has an id but no type.")

	note, err := parser.Parse(content)
	require.NoError(t, err)
	assert.False(t, note.IsDomain)
	assert.Empty(t, note.ID)
}

func TestParse_NoteWithTypeButNoID(t *testing.T) {
	content := []byte("---\ntype: project\ntitle: No ID Project\n---\n\nThis note has a type but no id.")

	note, err := parser.Parse(content)
	require.NoError(t, err)
	assert.False(t, note.IsDomain)
	assert.Empty(t, note.ID)
	assert.Empty(t, note.NoteType)
}

func TestParse_InvalidFrontmatter(t *testing.T) {
	content := []byte("---\nid: [broken\ntype: project\n---\n\nBody.")
	_, err := parser.Parse(content)
	assert.Error(t, err)
}

func TestParse_EmptyFile(t *testing.T) {
	note, err := parser.Parse([]byte{})
	require.NoError(t, err)
	assert.False(t, note.IsDomain)
	assert.Empty(t, note.Links)
}

func TestParse_FTSBodyExcludesFrontmatter(t *testing.T) {
	content := []byte("---\nid: note-x\ntype: concept\ntitle: My Concept\ntags:\n  - alpha\n---\n\n# My Concept\n\nThis is the body of the concept.")

	note, err := parser.Parse(content)
	require.NoError(t, err)
	assert.NotContains(t, note.FTSBody, "id: note-x")
	assert.Contains(t, note.FTSBody, "My Concept")
}

func TestParse_RealVaultConceptNote(t *testing.T) {
	content, err := os.ReadFile("../../vaultmind-vault/concepts/act-r.md")
	require.NoError(t, err)

	note, err := parser.Parse(content)
	require.NoError(t, err)

	assert.True(t, note.IsDomain)
	assert.Equal(t, "concept-act-r", note.ID)
	assert.Equal(t, "concept", note.NoteType)
	assert.Equal(t, "ACT-R", note.Frontmatter["title"])

	targets := make([]string, len(note.Links))
	for i, l := range note.Links {
		targets[i] = l.Target
	}
	assert.Contains(t, targets, "Context Pack")
	assert.Contains(t, targets, "Spreading Activation")
	assert.Contains(t, note.FTSBody, "cognitive architecture")
	assert.NotContains(t, note.FTSBody, "[[")
}

func TestParse_RealVaultProjectNote(t *testing.T) {
	content, err := os.ReadFile("../../vaultmind-vault/projects/proj-memory-research.md")
	require.NoError(t, err)

	note, err := parser.Parse(content)
	require.NoError(t, err)

	assert.True(t, note.IsDomain)
	assert.Equal(t, "proj-memory-research", note.ID)
	assert.Equal(t, "project", note.NoteType)
	assert.Greater(t, len(note.Links), 5)
}

func TestParse_RealVaultDecisionNote(t *testing.T) {
	content, err := os.ReadFile("../../vaultmind-vault/decisions/decision-bfs-with-visited-set.md")
	require.NoError(t, err)

	note, err := parser.Parse(content)
	require.NoError(t, err)

	assert.True(t, note.IsDomain)
	assert.Equal(t, "decision-bfs-with-visited-set", note.ID)
	assert.Equal(t, "decision", note.NoteType)
	assert.GreaterOrEqual(t, len(note.Headings), 3)
	assert.NotContains(t, note.FTSBody, "id:")
}

func TestParse_RealVaultAllNotes(t *testing.T) {
	vaultRoot := "../../vaultmind-vault"
	excludes := map[string]bool{".git": true, ".obsidian": true, ".trash": true, ".vaultmind": true, "templates": true}

	err := filepath.WalkDir(vaultRoot, func(path string, d os.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() {
			if excludes[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}

		content, readErr := os.ReadFile(path)
		require.NoError(t, readErr, "reading %s", path)

		_, parseErr := parser.Parse(content)
		assert.NoError(t, parseErr, "parsing %s", path)
		return nil
	})
	require.NoError(t, err)
}
