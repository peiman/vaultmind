package propose

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPropose_NewDraftDoesNotInheritAnImport(t *testing.T) {
	v := testVault(t)
	const (
		targetID  = "ref-incoming"
		targetRel = "imported/incoming/page.md"
	)
	writeTarget(t, v, targetRel, targetID, vault.ImportNoteType, "Page", "accepted", "An imported page.")
	ret := &stubClosest{hit: Hit{
		ID: targetID, Path: targetRel, Type: vault.ImportNoteType, Band: "moderate",
	}, ok: true}
	req, out, _, _ := reqOf("How does quartz work?", "A finding the schema can hold.")
	require.NoError(t, Run(context.Background(), v, req, ret))

	assert.Contains(t, out.String(), "mode: new\n")
	assert.Contains(t, out.String(), "path: notes/how-does-quartz-work.md\n")
	assert.NotContains(t, out.String(), "imported/")
	fm, _ := parseNote(t, v, "notes/how-does-quartz-work.md")
	assert.Equal(t, defaultKnowledgeType, fm["type"])
	assert.NotEqual(t, vault.ImportNoteType, fm["type"])
}

func TestPropose_PatchOfAnImportGoesToTheDraftsFolder(t *testing.T) {
	v := testVault(t)
	const (
		targetID  = "ref-incoming"
		targetRel = "imported/incoming/page.md"
	)
	writeTarget(t, v, targetRel, targetID, vault.ImportNoteType, "Page", "accepted", "An imported page.")
	ret := &stubClosest{hit: Hit{
		ID: targetID, Path: targetRel, Type: vault.ImportNoteType, Band: "strong",
	}, ok: true}
	req, out, _, _ := reqOf("How does quartz work?", "Quartz keeps one timeline.")
	require.NoError(t, Run(context.Background(), v, req, ret))

	want := "notes/" + targetID + ".proposal-20261014-1530.md"
	assert.Contains(t, out.String(), "mode: patch\n")
	assert.Contains(t, out.String(), "path: "+want+"\n")
	assert.NotContains(t, out.String(), "imported/")
	fm, _ := parseNote(t, v, want)
	assert.Equal(t, vault.ImportNoteType, fm["type"], "a patch keeps the target type so the proposal can be written")
	assert.Equal(t, targetID, fm["patches"])
}

func TestPropose_RegisteredImportTypeOutsideTheImportFolderStaysPut(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "concepts/ref.md", "concept-ref", vault.ImportNoteType, "Ref", "accepted", "Not an import.")
	ret := &stubClosest{hit: Hit{
		ID: "concept-ref", Path: "concepts/ref.md", Type: vault.ImportNoteType, Band: "moderate",
	}, ok: true}
	req, out, _, _ := reqOf("How does quartz work?", "A finding the schema can hold.")
	require.NoError(t, Run(context.Background(), v, req, ret))

	assert.Contains(t, out.String(), "path: concepts/how-does-quartz-work.md\n")
	fm, _ := parseNote(t, v, "concepts/how-does-quartz-work.md")
	assert.Equal(t, defaultKnowledgeType, fm["type"])
}

func TestPropose_DraftsDirOverrideAndRoot(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "imported/incoming/page.md", "ref-incoming", vault.ImportNoteType, "Page", "accepted", "An imported page.")
	ret := &stubClosest{hit: Hit{
		ID: "ref-incoming", Path: "imported/incoming/page.md", Type: vault.ImportNoteType, Band: "moderate",
	}, ok: true}

	req, out, _, _ := reqOf("How does quartz work?", "A finding the schema can hold.")
	req.DraftsDir = "drafts"
	require.NoError(t, Run(context.Background(), v, req, ret))
	assert.Contains(t, out.String(), "path: drafts/how-does-quartz-work.md\n")

	again, out2, _, _ := reqOf("Where does the day go?", "It goes to the work.")
	again.DraftsDir = "."
	require.NoError(t, Run(context.Background(), v, again, ret))
	assert.Contains(t, out2.String(), "path: where-does-the-day-go.md\n")
	assert.NotContains(t, out2.String(), "notes/")

	third, out3, _, _ := reqOf("Where does quartzkeeper live?", "Under the hill.")
	third.DraftsDir = "imported/custom"
	require.NoError(t, Run(context.Background(), v, third, ret))
	assert.Contains(t, out3.String(), "path: notes/where-does-quartzkeeper-live.md\n")
	assert.NotContains(t, out3.String(), "imported/")
}

func TestPropose_ARootFileNamedImportedIsNotAnImportFolder(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "imported.md", "note-imported", "concept", "Imported", "accepted", "A root note.")
	ret := &stubClosest{hit: Hit{
		ID: "note-imported", Path: "imported.md", Type: "concept", Band: "moderate",
	}, ok: true}
	req, out, _, _ := reqOf("How does quartz work?", "A finding the schema can hold.")
	require.NoError(t, Run(context.Background(), v, req, ret))
	assert.Contains(t, out.String(), "path: how-does-quartz-work.md\n")
	assert.NotContains(t, out.String(), "notes/")
}
