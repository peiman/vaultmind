package propose

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gapFrontmatter(id, question string) string {
	return "" +
		"---\n" +
		"# kept\n" +
		"title: \"Quoted Title\"\n" +
		"opened_at: \"2026-10-01T00:00:00Z\"\n" +
		"status: open\n" +
		"id: " + id + "\n" +
		"type: gap\n" +
		"question: \"" + question + "\"\n" +
		"---\n" +
		"\n" +
		"The body stays.\n"
}

func closedGapBytes(original, closedBy string) string {
	updated := strings.Replace(original, "status: open\n", "status: closed\n", 1)
	insert := "closed_at: \"2026-10-14T15:30:00Z\"\nclosed_by: " + closedBy + "\n"
	const end = "---\n\n"
	i := strings.LastIndex(updated, end)
	return updated[:i] + insert + updated[i:]
}

func TestAccept_ClosesTheLinkedGap(t *testing.T) {
	const question = "How does quartz work?"
	const gapID = "gap-quartz"
	const gapRel = "gaps/quartz.md"

	t.Run("new", func(t *testing.T) {
		v := testVault(t)
		original := gapFrontmatter(gapID, question)
		writeRawNote(t, v, gapRel, original)
		ret := &stubClosest{}
		req, _, _, _ := reqOf("", "A finding the schema can hold.")
		req.GapID = gapID
		require.NoError(t, Run(context.Background(), v, req, ret))

		draftRel := "how-does-quartz-work.md"
		fm, _ := parseNote(t, v, draftRel)
		assert.Equal(t, gapID, fm["closes_gap"])
		assert.Equal(t, question, fm["question"])
		draftID, _ := fm["id"].(string)
		assert.Equal(t, "concept-how-does-quartz-work", draftID)

		accept, _, _, _ := reqOf("", "")
		accept.ID = draftID
		require.NoError(t, Accept(context.Background(), v, accept))

		got, err := os.ReadFile(filepath.Join(v.Path, filepath.FromSlash(gapRel)))
		require.NoError(t, err)
		assert.Equal(t, closedGapBytes(original, draftID), string(got))
		row, err := v.DB.QueryNoteByID(gapID)
		require.NoError(t, err)
		require.NotNil(t, row)
		assert.Equal(t, gapStatusClosed, row.Status)
	})

	t.Run("patch", func(t *testing.T) {
		v := testVault(t)
		const targetID = "concept-memory"
		writeTarget(t, v, "concepts/memory.md", targetID, "concept", "Memory", "accepted", "The original body.")
		original := gapFrontmatter(gapID, question)
		writeRawNote(t, v, gapRel, original)
		ret := &stubClosest{hit: Hit{ID: targetID, Path: "concepts/memory.md", Type: "concept", Band: "strong"}, ok: true}
		req, _, _, _ := reqOf(question, "Quartz keeps one timeline.")
		req.GapID = gapID
		require.NoError(t, Run(context.Background(), v, req, ret))

		rel := proposalPath("concepts/memory.md", targetID, proposeNow)
		fm, _ := parseNote(t, v, rel)
		assert.Equal(t, gapID, fm["closes_gap"])
		accept, _, _, _ := reqOf("", "")
		accept.ID, _ = fm["id"].(string)
		require.NoError(t, Accept(context.Background(), v, accept))

		got, err := os.ReadFile(filepath.Join(v.Path, filepath.FromSlash(gapRel)))
		require.NoError(t, err)
		assert.Equal(t, closedGapBytes(original, targetID), string(got))
		_, statErr := os.Stat(filepath.Join(v.Path, filepath.FromSlash(rel)))
		assert.ErrorIs(t, statErr, os.ErrNotExist)
	})
}

func TestPropose_RejectsAGapThatIsNotOpen(t *testing.T) {
	const question = "How does quartz work?"
	cases := []struct {
		name  string
		write bool
		rel   string
		body  string
		gapID string
		want  string
	}{
		{name: "missing", gapID: "gap-missing", want: "not in the vault"},
		{
			name: "closed", write: true, rel: "gaps/quartz.md", gapID: "gap-quartz",
			body: strings.Replace(gapFrontmatter("gap-quartz", question), "status: open\n", "status: closed\n", 1),
			want: "not open",
		},
		{
			name: "wrong type", write: true, rel: "concepts/other.md", gapID: "concept-other",
			body: strings.ReplaceAll(gapFrontmatter("concept-other", question), "type: gap\n", "type: concept\n"),
			want: "not a gap",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := testVault(t)
			if tc.write {
				writeRawNote(t, v, tc.rel, tc.body)
			}
			req, _, _, _ := reqOf("", "A finding.")
			req.GapID = tc.gapID
			err := Run(context.Background(), v, req, &stubClosest{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			matches, globErr := filepath.Glob(filepath.Join(v.Path, "*.md"))
			require.NoError(t, globErr)
			assert.Empty(t, matches, "a rejected gap writes no draft")
		})
	}
}
