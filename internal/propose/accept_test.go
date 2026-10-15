package propose

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
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

// Two patches to one target inside the same minute must both land. A
// minute-resolution proposal id let the second draft reuse the first one's id
// after it was accepted, so accept saw the old marker and dropped the finding.
func TestAccept_TwoPatchesInOneMinuteBothApply(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "The original body.")
	ret := &stubClosest{hit: Hit{ID: "concept-memory", Path: "concepts/memory.md", Type: "concept", Band: "strong"}, ok: true}
	for _, finding := range []string{"First finding.", "Second finding."} {
		req, _, _, _ := reqOf("How does quartz work?", finding)
		require.NoError(t, Run(context.Background(), v, req, ret))
		matches, err := filepath.Glob(filepath.Join(v.Path, "concepts", "*.proposal-*.md"))
		require.NoError(t, err)
		require.Len(t, matches, 1, "one open proposal at a time")
		rel, err := filepath.Rel(v.Path, matches[0])
		require.NoError(t, err)
		fm, _ := parseNote(t, v, filepath.ToSlash(rel))
		acc, _, _, _ := reqOf("", "")
		acc.ID, _ = fm["id"].(string)
		require.NoError(t, Accept(context.Background(), v, acc))
	}
	_, body := parseNote(t, reopen(t, v.Path), "concepts/memory.md")
	assert.Contains(t, body, "First finding.")
	assert.Contains(t, body, "Second finding.")
}

func proposePatch(t *testing.T, noteType, targetRel, targetID string, sources []string) (Vault, string, string) {
	t.Helper()
	v := testVault(t)
	writeTarget(t, v, targetRel, targetID, noteType, "Memory", "accepted", "The original body.")
	ret := &stubClosest{hit: Hit{ID: targetID, Path: targetRel, Type: noteType, Band: "strong"}, ok: true}
	req, _, _, _ := reqOf("How does quartz work?", "Quartz keeps one timeline.")
	req.Sources = sources
	require.NoError(t, Run(context.Background(), v, req, ret))
	rel := proposalPath(targetRel, targetID, "Quartz keeps one timeline.", proposeNow)
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
	rel := proposalPath("concepts/memory.md", "concept-memory", "Quartz keeps one timeline.", proposeNow)
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

func writeRawNote(t *testing.T, v Vault, rel, content string) {
	t.Helper()
	abs := filepath.Join(v.Path, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
	dbPath := filepath.Join(v.Path, v.Config.Index.DBPath)
	require.NoError(t, index.NewIndexer(v.Path, dbPath, v.Config).IndexFile(rel))
}

func proposalNote(id, patches, question, body string, sources []string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("id: " + id + "\n")
	b.WriteString("type: concept\n")
	b.WriteString("title: " + question + "\n")
	b.WriteString("status: draft\n")
	b.WriteString("question: " + question + "\n")
	b.WriteString("patches: " + patches + "\n")
	b.WriteString("created: 2026-04-03\n")
	if len(sources) > 0 {
		b.WriteString("sources:\n")
		for _, source := range sources {
			b.WriteString("  - " + source + "\n")
		}
	}
	b.WriteString("---\n\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}

// skipUnlessDirModeBlocks reports whether a 0555 directory stops block.
// Root and some filesystems ignore the mode; those runs cannot prove the
// permission window this test is about.
func skipUnlessDirModeBlocks(t *testing.T, block func(dir, file string) error) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory write permission")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "note.md")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if err := block(dir, file); err == nil {
		t.Skip("directory mode does not block the write this test needs")
	}
}

func TestAccept_RetryAfterCleanupFailureDoesNotDuplicate(t *testing.T) {
	skipUnlessDirModeBlocks(t, func(_, file string) error { return os.Remove(file) })

	v := testVault(t)
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "The original body.")
	const (
		proposalRel = "hold/proposal-quartz.md"
		proposalID  = "proposal-quartz"
		question    = "How does quartz work?"
	)
	writeRawNote(t, v, proposalRel, proposalNote(proposalID, "concept-memory", question, "Quartz keeps one timeline.", nil))
	hold := filepath.Join(v.Path, "hold")
	require.NoError(t, os.Chmod(hold, 0o555))
	t.Cleanup(func() { _ = os.Chmod(hold, 0o755) })

	req, _, _, _ := reqOf("", "")
	req.ID = proposalID
	err := Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "removing proposal")

	targetAbs := filepath.Join(v.Path, "concepts", "memory.md")
	patched, err := os.ReadFile(targetAbs)
	require.NoError(t, err)
	marker := acceptedMarker(proposalID)
	assert.Equal(t, 1, strings.Count(string(patched), marker))
	assert.Equal(t, 1, strings.Count(string(patched), "## "+question))
	assert.Contains(t, string(patched), marker+"\n## "+question)
	_, statErr := os.Stat(filepath.Join(v.Path, filepath.FromSlash(proposalRel)))
	assert.NoError(t, statErr)

	require.NoError(t, os.Chmod(hold, 0o755))
	require.NoError(t, Accept(context.Background(), v, req))
	again, err := os.ReadFile(targetAbs)
	require.NoError(t, err)
	assert.Equal(t, patched, again)
	_, statErr = os.Stat(filepath.Join(v.Path, filepath.FromSlash(proposalRel)))
	assert.ErrorIs(t, statErr, os.ErrNotExist)

	fresh := reopen(t, v.Path)
	gone, err := fresh.DB.QueryNoteByPath(proposalRel)
	require.NoError(t, err)
	assert.Nil(t, gone)
	var indexed string
	require.NoError(t, fresh.DB.QueryRow(`SELECT body_text FROM notes WHERE id = ?`, "concept-memory").Scan(&indexed))
	assert.NotContains(t, indexed, "vaultmind:accepted")
	assert.NotContains(t, indexed, proposalID)
	assert.Contains(t, indexed, "Quartz keeps one timeline.")
	excerpt := memory.Excerpt(string(again), 400)
	assert.NotContains(t, excerpt, "vaultmind:accepted")
	assert.NotContains(t, excerpt, proposalID)
}

func TestAccept_RefusesWhenTheTargetChanged(t *testing.T) {
	v := testVault(t)
	rel := "concepts/how-does-quartz-work.md"
	writeTarget(t, v, rel, "how-does-quartz-work", "concept", "How does quartz work?", "draft", "A finding the schema can hold.")
	abs := filepath.Join(v.Path, "concepts", "how-does-quartz-work.md")

	req, _, _, _ := reqOf("", "")
	req.ID = "how-does-quartz-work"
	req.beforeWrite = func(path string) error {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0) //nolint:gosec // path is the note this test just wrote
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString("\nCONCURRENT\n")
		return err
	}
	err := Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "changed")
	assert.Contains(t, err.Error(), "nothing was written")

	got, err := os.ReadFile(abs)
	require.NoError(t, err)
	assert.Contains(t, string(got), "CONCURRENT")
	assert.Contains(t, string(got), "status: draft")
	assert.NotContains(t, string(got), "status: accepted")
}

func TestAccept_FrontmatterPreservesUnchangedKeys(t *testing.T) {
	v := testVault(t)
	const rel = "concepts/styled.md"
	original := "" +
		"---\n" +
		"# kept\n" +
		"title: \"Quoted Title\"\n" +
		"created: 2026-04-03 # minted\n" +
		"status: draft\n" +
		"id: styled-note\n" +
		"type: concept\n" +
		"---\n" +
		"\n" +
		"The body stays.\n"
	writeRawNote(t, v, rel, original)

	req, _, _, _ := reqOf("", "")
	req.ID = "styled-note"
	require.NoError(t, Accept(context.Background(), v, req))

	got, err := os.ReadFile(filepath.Join(v.Path, "concepts", "styled.md"))
	require.NoError(t, err)
	want := strings.Replace(original, "status: draft\n", "status: accepted\n", 1)
	assert.Equal(t, want, string(got))
}

func TestAccept_PatchKeepsFrontmatterExceptSources(t *testing.T) {
	v := testVault(t)
	const rel = "concepts/memory.md"
	original := "" +
		"---\n" +
		"# kept\n" +
		"title: \"Quoted Title\"\n" +
		"created: 2026-04-03 # minted\n" +
		"status: accepted\n" +
		"id: concept-memory\n" +
		"type: concept\n" +
		"---\n" +
		"\n" +
		"The original body.\n"
	writeRawNote(t, v, rel, original)
	const proposalRel = "concepts/proposal-styled.md"
	writeRawNote(t, v, proposalRel, proposalNote(
		"proposal-styled", "concept-memory", "How does quartz work?", "Quartz keeps one timeline.",
		[]string{"https://example.com/quartz"},
	))

	req, _, _, _ := reqOf("", "")
	req.ID = "proposal-styled"
	require.NoError(t, Accept(context.Background(), v, req))

	got, err := os.ReadFile(filepath.Join(v.Path, "concepts", "memory.md"))
	require.NoError(t, err)
	const sourcesBlock = "sources:\n  - https://example.com/quartz\n"
	head, _, ok := strings.Cut(original, "type: concept\n---\n")
	require.True(t, ok)
	wantHead := head + "type: concept\n" + sourcesBlock + "---\n\n"
	assert.True(t, strings.HasPrefix(string(got), wantHead), "got:\n%s", got)
	assert.Contains(t, string(got), "The original body.")
	assert.Contains(t, string(got), acceptedMarker("proposal-styled")+"\n## How does quartz work?")
}

func TestAccept_WriteUsesATempFile(t *testing.T) {
	skipUnlessDirModeBlocks(t, func(dir, _ string) error {
		f, err := os.CreateTemp(dir, ".vaultmind-*.tmp")
		if err != nil {
			return err
		}
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
		return nil
	})

	v := testVault(t)
	rel := "concepts/how-does-quartz-work.md"
	writeTarget(t, v, rel, "how-does-quartz-work", "concept", "How does quartz work?", "draft", "A finding the schema can hold.")
	abs := filepath.Join(v.Path, "concepts", "how-does-quartz-work.md")
	before, err := os.ReadFile(abs)
	require.NoError(t, err)
	dir := filepath.Dir(abs)
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	req, _, _, _ := reqOf("", "")
	req.ID = "how-does-quartz-work"
	err = Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating temp file")
	got, err := os.ReadFile(abs)
	require.NoError(t, err)
	assert.Equal(t, before, got)
}

func TestAccept_RefusesADraftTarget(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "draft", "The original body.")
	const proposalRel = "concepts/proposal-quartz.md"
	writeRawNote(t, v, proposalRel, proposalNote("proposal-quartz", "concept-memory", "How does quartz work?", "Quartz keeps one timeline.", nil))
	before, err := os.ReadFile(filepath.Join(v.Path, "concepts", "memory.md"))
	require.NoError(t, err)

	req, _, _, _ := reqOf("", "")
	req.ID = "proposal-quartz"
	err = Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "draft")
	assert.Contains(t, err.Error(), "concept-memory")
	assert.NotContains(t, err.Error(), "itself a proposal")

	got, err := os.ReadFile(filepath.Join(v.Path, "concepts", "memory.md"))
	require.NoError(t, err)
	assert.Equal(t, before, got)
	_, statErr := os.Stat(filepath.Join(v.Path, filepath.FromSlash(proposalRel)))
	assert.NoError(t, statErr)
}

func TestAccept_RefusesAProposalTarget(t *testing.T) {
	v := testVault(t)
	writeRawNote(t, v, "concepts/inner.md", proposalNote("concept-inner", "concept-memory", "Inner question?", "Inner finding.", nil))
	const proposalRel = "concepts/proposal-outer.md"
	writeRawNote(t, v, proposalRel, proposalNote("proposal-outer", "concept-inner", "How does quartz work?", "Quartz keeps one timeline.", nil))
	before, err := os.ReadFile(filepath.Join(v.Path, "concepts", "inner.md"))
	require.NoError(t, err)

	req, _, _, _ := reqOf("", "")
	req.ID = "proposal-outer"
	err = Accept(context.Background(), v, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "itself a proposal")
	assert.Contains(t, err.Error(), "concept-inner")

	got, err := os.ReadFile(filepath.Join(v.Path, "concepts", "inner.md"))
	require.NoError(t, err)
	assert.Equal(t, before, got)
	_, statErr := os.Stat(filepath.Join(v.Path, filepath.FromSlash(proposalRel)))
	assert.NoError(t, statErr)
}
