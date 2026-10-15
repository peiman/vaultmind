package propose

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
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

		rel := proposalPath("concepts/memory.md", targetID, "Quartz keeps one timeline.", proposeNow)
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

func draftClosing(id, gapID string) string {
	return "---\nid: " + id + "\ntype: concept\ntitle: Finding\nstatus: draft\ncloses_gap: " + gapID + "\ncreated: 2026-04-03\n---\n\nThe finding.\n"
}

func TestAccept_MissingGapWarnsAndStillPublishes(t *testing.T) {
	const id = "concept-finding"
	const gapID = "gap-gone"
	const want = `gap "gap-gone" is not in the vault; accepted the note anyway`

	t.Run("text", func(t *testing.T) {
		v := testVault(t)
		writeRawNote(t, v, "finding.md", draftClosing(id, gapID))
		req, out, errOut, _ := reqOf("", "")
		req.ID = id
		require.NoError(t, Accept(context.Background(), v, req))
		assert.Contains(t, errOut.String(), "warning: "+want)
		assert.Contains(t, out.String(), "accepted "+id)
		fm, _ := parseNote(t, v, "finding.md")
		assert.Equal(t, "accepted", fm["status"])
	})

	t.Run("json", func(t *testing.T) {
		v := testVault(t)
		writeRawNote(t, v, "finding.md", draftClosing(id, gapID))
		req, out, _, _ := reqOf("", "")
		req.JSON = true
		req.ID = id
		require.NoError(t, Accept(context.Background(), v, req))
		warning := decodeAcceptWarning(t, out.Bytes())
		assert.Equal(t, "warning", warning.Status)
		require.Len(t, warning.Warnings, 1)
		assert.Equal(t, "gap_missing", warning.Warnings[0].Code)
		assert.Equal(t, want, warning.Warnings[0].Message)
		assert.Equal(t, gapID, warning.Warnings[0].Field)
		assert.NotContains(t, out.String(), "reindex_failed")
		fm, _ := parseNote(t, v, "finding.md")
		assert.Equal(t, "accepted", fm["status"])
	})

	t.Run("patch", func(t *testing.T) {
		v := testVault(t)
		writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "The original body.")
		body := strings.Replace(proposalNote("proposal-memory", "concept-memory", "How does quartz work?", "Quartz keeps one timeline.\n", nil), "status: draft\n", "status: draft\ncloses_gap: "+gapID+"\n", 1)
		writeRawNote(t, v, "proposals/memory.md", body)
		req, out, _, _ := reqOf("", "")
		req.JSON = true
		req.ID = "proposal-memory"
		require.NoError(t, Accept(context.Background(), v, req))
		warning := decodeAcceptWarning(t, out.Bytes())
		require.Len(t, warning.Warnings, 1)
		assert.Equal(t, "gap_missing", warning.Warnings[0].Code)
		assert.Equal(t, want, warning.Warnings[0].Message)
		_, statErr := os.Stat(filepath.Join(v.Path, "proposals", "memory.md"))
		assert.ErrorIs(t, statErr, os.ErrNotExist)
	})
}

func decodeAcceptWarning(t *testing.T, raw []byte) struct {
	Status   string `json:"status"`
	Warnings []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field"`
	} `json:"warnings"`
} {
	t.Helper()
	var env struct {
		Status   string `json:"status"`
		Warnings []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Field   string `json:"field"`
		} `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal(raw, &env))
	return env
}

func TestAccept_LinkedNoteThatIsNotAGapStaysADraft(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "concepts/other.md", "concept-other", "concept", "Other", "accepted", "body")
	writeRawNote(t, v, "finding.md", draftClosing("concept-finding", "concept-other"))
	var buf bytes.Buffer
	req, _, _, _ := reqOf("", "")
	req.JSON = true
	req.Stdout = &buf
	req.ID = "concept-finding"
	err := Accept(context.Background(), v, req)
	require.ErrorIs(t, err, envelope.ErrAlreadyWritten)
	assert.Contains(t, buf.String(), `"code":"gap_invalid"`)
	assert.Contains(t, buf.String(), "not a gap")
	fm, _ := parseNote(t, v, "finding.md")
	assert.Equal(t, index.StatusDraft, fm["status"])
}

func TestAccept_UnreadableGapStaysADraft(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses mode 000")
	}
	v := testVault(t)
	const gapID = "gap-hidden"
	rel := "gaps/hidden.md"
	writeRawNote(t, v, rel, gapFrontmatter(gapID, "Hidden?"))
	abs := filepath.Join(v.Path, filepath.FromSlash(rel))
	t.Cleanup(func() { _ = os.Chmod(abs, 0o644) })
	require.NoError(t, os.Chmod(abs, 0))
	writeRawNote(t, v, "finding.md", draftClosing("concept-finding", gapID))
	var buf bytes.Buffer
	req, _, _, _ := reqOf("", "")
	req.JSON = true
	req.Stdout = &buf
	req.ID = "concept-finding"
	err := Accept(context.Background(), v, req)
	require.ErrorIs(t, err, envelope.ErrAlreadyWritten)
	assert.Contains(t, buf.String(), `"code":"gap_unreadable"`)
	fm, _ := parseNote(t, v, "finding.md")
	assert.Equal(t, index.StatusDraft, fm["status"])
}

func TestAccept_UnparseableGapStaysADraft(t *testing.T) {
	v := testVault(t)
	const gapID = "gap-broken"
	rel := "gaps/broken.md"
	writeRawNote(t, v, rel, gapFrontmatter(gapID, "Broken?"))
	abs := filepath.Join(v.Path, filepath.FromSlash(rel))
	require.NoError(t, os.WriteFile(abs, []byte("---\n[\n---\n"), 0o644))
	writeRawNote(t, v, "finding.md", draftClosing("concept-finding", gapID))
	var buf bytes.Buffer
	req, _, _, _ := reqOf("", "")
	req.JSON = true
	req.Stdout = &buf
	req.ID = "concept-finding"
	err := Accept(context.Background(), v, req)
	require.ErrorIs(t, err, envelope.ErrAlreadyWritten)
	assert.Contains(t, buf.String(), `"code":"gap_invalid"`)
	assert.Contains(t, buf.String(), "parsing gap")
	fm, _ := parseNote(t, v, "finding.md")
	assert.Equal(t, index.StatusDraft, fm["status"])
}
