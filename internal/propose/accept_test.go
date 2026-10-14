package propose

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccept_NewFlipsStatus(t *testing.T) {
	cases := []struct {
		name     string
		noteType string
		band     string
		hitPath  string
		hitID    string
		want     string
	}{
		{name: "empty status list becomes accepted", noteType: "concept", band: "moderate", hitPath: "concepts/memory.md", hitID: "concept-memory", want: "accepted"},
		{name: "active is the published status when accepted is not allowed", noteType: "project", band: "moderate", hitPath: "projects/launch.md", hitID: "project-launch", want: "active"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := testVault(t)
			writeTarget(t, v, tc.hitPath, tc.hitID, tc.noteType, "Target", tc.want, "The target.")
			ret := &stubClosest{hit: Hit{ID: tc.hitID, Path: tc.hitPath, Type: tc.noteType, Band: tc.band}, ok: true}
			created, _, _, _ := reqOf("How does quartz work?", "A finding the schema can hold.")
			require.NoError(t, Run(context.Background(), v, created, ret))
			rel := filepath.ToSlash(filepath.Dir(tc.hitPath)) + "/how-does-quartz-work.md"
			if filepath.Dir(tc.hitPath) == "." {
				rel = "how-does-quartz-work.md"
			}
			fm, _ := parseNote(t, v, rel)
			id, _ := fm["id"].(string)
			require.NotEmpty(t, id)

			outReq, out, _, embeds := reqOf("", "")
			outReq.ID = id
			require.NoError(t, Accept(context.Background(), v, outReq))
			assert.Contains(t, out.String(), "accepted "+id)
			assert.Contains(t, out.String(), "status: "+tc.want)
			assert.Equal(t, 1, *embeds)

			fm, _ = parseNote(t, v, rel)
			assert.Equal(t, tc.want, fm["status"])
			assert.Equal(t, id, fm["id"])
			row, err := v.DB.QueryNoteByID(id)
			require.NoError(t, err)
			require.NotNil(t, row)
			assert.Equal(t, tc.want, row.Status)
			assert.Equal(t, rel, row.Path)
		})
	}
}

func TestAccept_PatchMergesDeletesAndReindexes(t *testing.T) {
	t.Run("sources copied when the type allows them", func(t *testing.T) {
		v, rel, targetID := proposePatch(t, "concept", "concepts/memory.md", "concept-memory", []string{"https://example.com/quartz"})
		req, out, _, embeds := reqOf("", "")
		fm, _ := parseNote(t, v, rel)
		req.ID, _ = fm["id"].(string)
		require.NoError(t, Accept(context.Background(), v, req))
		assert.Contains(t, out.String(), "accepted "+targetID)
		assert.Contains(t, out.String(), rel)
		_, statErr := os.Stat(filepath.Join(v.Path, filepath.FromSlash(rel)))
		assert.ErrorIs(t, statErr, os.ErrNotExist)
		assert.Equal(t, 1, *embeds)

		fresh := reopen(t, v.Path)
		note, err := fresh.DB.QueryFullNote(targetID)
		require.NoError(t, err)
		require.NotNil(t, note)
		assert.Contains(t, note.Body, "## How does quartz work?")
		assert.Contains(t, note.Body, "Quartz keeps one timeline.")
		assert.Contains(t, note.Body, "## Sources")
		assert.Contains(t, note.Body, "https://example.com/quartz")
		assert.Equal(t, []string{"https://example.com/quartz"}, fmStrings(note.Frontmatter["sources"]))
		targetFM, targetBody := parseNote(t, fresh, "concepts/memory.md")
		assert.Equal(t, "Memory", targetFM["title"])
		assert.Equal(t, "accepted", targetFM["status"])
		assert.Equal(t, targetID, targetFM["id"])
		assert.Contains(t, targetBody, "The original body.")
		gone, err := fresh.DB.QueryNoteByPath(rel)
		require.NoError(t, err)
		assert.Nil(t, gone)
	})

	t.Run("sources stay in the body when the type has no sources field", func(t *testing.T) {
		v, rel, targetID := proposePatch(t, "principle", "principles/one.md", "principle-one", []string{"https://example.com/one"})
		req, _, _, _ := reqOf("", "")
		fm, _ := parseNote(t, v, rel)
		req.ID, _ = fm["id"].(string)
		require.NoError(t, Accept(context.Background(), v, req))
		fresh := reopen(t, v.Path)
		note, err := fresh.DB.QueryFullNote(targetID)
		require.NoError(t, err)
		assert.Contains(t, note.Body, "https://example.com/one")
		assert.Nil(t, note.Frontmatter["sources"])
		_, statErr := os.Stat(filepath.Join(v.Path, filepath.FromSlash(rel)))
		assert.ErrorIs(t, statErr, os.ErrNotExist)
	})
}

func proposePatch(t *testing.T, noteType, targetRel, targetID string, sources []string) (Vault, string, string) {
	t.Helper()
	v := testVault(t)
	writeTarget(t, v, targetRel, targetID, noteType, "Memory", "accepted", "The original body.")
	ret := &stubClosest{hit: Hit{ID: targetID, Path: targetRel, Type: noteType, Band: "strong"}, ok: true}
	req, _, _, _ := reqOf("How does quartz work?", "Quartz keeps one timeline.")
	req.Sources = sources
	require.NoError(t, Run(context.Background(), v, req, ret))
	rel := proposalPath(targetRel, targetID, proposeNow)
	return v, rel, targetID
}

func TestAccept_NonDraftErrors(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "body")
	req, _, _, _ := reqOf("", "")
	req.ID = "concept-memory"
	err := Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a draft")
	assert.Contains(t, err.Error(), "concept-memory")
}

func TestAccept_MissingTargetErrors(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "body")
	ret := &stubClosest{hit: Hit{ID: "concept-memory", Path: "concepts/memory.md", Type: "concept", Band: "strong"}, ok: true}
	created, _, _, _ := reqOf("How does quartz work?", "Quartz keeps one timeline.")
	require.NoError(t, Run(context.Background(), v, created, ret))
	rel := proposalPath("concepts/memory.md", "concept-memory", proposeNow)
	fm, _ := parseNote(t, v, rel)
	require.NoError(t, os.Remove(filepath.Join(v.Path, "concepts", "memory.md")))
	require.NoError(t, indexDelete(t, v, "concepts/memory.md"))

	req, _, _, _ := reqOf("", "")
	req.ID, _ = fm["id"].(string)
	err := Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in the vault")
}

func indexDelete(t *testing.T, v Vault, rel string) error {
	t.Helper()
	_, err := v.DB.Exec(`DELETE FROM notes WHERE path = ?`, rel)
	return err
}
