package index_test

import (
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// prose is about n tokens (4 chars each) of one paragraph.
func prose(n int) string {
	return strings.TrimSpace(strings.Repeat("word ", n*4/5)) + "\n\n"
}

// longBody is past the model's window, with the given level-2 headings each
// holding ~1,500 tokens, plus padding to cross the threshold.
func longBody(headings ...string) string {
	var b strings.Builder
	for _, h := range headings {
		b.WriteString("## " + h + "\n\n" + prose(1500))
	}
	for b.Len() < 9000*4 {
		b.WriteString(prose(1500))
	}
	return b.String()
}

// longRecord is a stored note past the window, its sections computed the way
// the indexer computes them. The body is plain markdown prose with headings,
// so stripping changes little and the test reads directly.
func longRecord(id, rel string, headings ...string) index.NoteRecord {
	rec := buildTestRecord(id, rel)
	rec.BodyText = longBody(headings...)
	rec.Sections = index.SectionsFor(rec.BodyText, rec.BodyText)
	return rec
}

type sectionRow struct {
	ID, NoteID, Anchor, HeadingPath, Body string
	Ordinal                               int
	HasEmbedding, HasSparse               bool
}

func sectionsOf(t *testing.T, db *index.DB, noteID string) []sectionRow {
	t.Helper()
	rows, err := db.Query(`SELECT id, note_id, ordinal, anchor, heading_path, body,
		embedding IS NOT NULL, sparse_embedding IS NOT NULL
		FROM sections WHERE note_id = ? ORDER BY ordinal`, noteID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []sectionRow
	for rows.Next() {
		var r sectionRow
		require.NoError(t, rows.Scan(&r.ID, &r.NoteID, &r.Ordinal, &r.Anchor, &r.HeadingPath, &r.Body, &r.HasEmbedding, &r.HasSparse))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

func countSections(t *testing.T, db *index.DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM sections").Scan(&n))
	return n
}

// A long note is stored with its sections: one row each, in order, keyed
// <note>#<anchor>, embeddings not yet computed. Their bodies are the note's.
func TestStoreNote_LongNoteStoresItsSections(t *testing.T) {
	db := openTestDB(t)
	rec := longRecord("ref-long", "refs/long.md", "Alpha", "Beta", "Gamma")
	require.NoError(t, index.StoreNote(db, rec))

	got := sectionsOf(t, db, "ref-long")
	require.GreaterOrEqual(t, len(got), 3)
	assert.Equal(t, "ref-long#alpha", got[0].ID)
	assert.Equal(t, "alpha", got[0].Anchor)
	assert.Equal(t, "Alpha", got[0].HeadingPath)
	assert.Equal(t, "ref-long#beta", got[1].ID)
	var joined strings.Builder
	for i, r := range got {
		assert.Equal(t, i, r.Ordinal)
		assert.False(t, r.HasEmbedding || r.HasSparse, "embeddings come in a later slice")
		joined.WriteString(r.Body)
	}
	// Bodies are stripped of markdown, so they do not rejoin byte for byte;
	// what must hold is that no text is lost across the parts.
	assert.Equal(t, strings.Count(rec.BodyText, "word"), strings.Count(joined.String(), "word"),
		"every word of the note is in exactly one part")
}

// A note that fits the window gets no section rows at all: short notes are
// untouched by this change.
func TestStoreNote_ShortNoteStoresNoSections(t *testing.T) {
	db := openTestDB(t)
	require.NoError(t, index.StoreNote(db, buildTestRecord("concept-short", "concepts/short.md")))
	assert.Zero(t, countSections(t, db))
}

// The byte-identity guard: the repo's fixture vault, all short notes, indexes
// to zero sections.
func TestIndex_FixtureVaultHasNoSections(t *testing.T) {
	db := buildIndexedDB(t)
	assert.Zero(t, countSections(t, db))
}

func TestStoreNote_ReindexReplacesSections(t *testing.T) {
	db := openTestDB(t)
	rec := longRecord("ref-long", "refs/long.md", "Alpha", "Beta", "Gamma")
	require.NoError(t, index.StoreNote(db, rec))

	rec = longRecord("ref-long", "refs/long.md", "Alpha", "Delta")
	rec.Hash = "changed"
	require.NoError(t, index.StoreNote(db, rec))
	for _, r := range sectionsOf(t, db, "ref-long") {
		assert.NotEqual(t, "ref-long#beta", r.ID, "a removed heading leaves no section behind")
		assert.NotEqual(t, "ref-long#gamma", r.ID)
	}

	rec.BodyText = "Now it is short.\n"
	rec.Sections = index.SectionsFor(rec.BodyText, rec.BodyText)
	rec.Hash = "short"
	require.NoError(t, index.StoreNote(db, rec))
	assert.Zero(t, countSections(t, db), "a note shrunk below the threshold keeps no sections")
}

func TestDeleteNote_RemovesItsSections(t *testing.T) {
	db := openTestDB(t)
	require.NoError(t, index.StoreNote(db, longRecord("ref-long", "refs/long.md", "Alpha", "Beta")))
	require.NotZero(t, countSections(t, db))

	require.NoError(t, index.DeleteNoteByPath(db, "refs/long.md"))
	assert.Zero(t, countSections(t, db))
}

// A long note first indexed under its path-derived id, which then gains a
// frontmatter id: no sections are left under the old id.
func TestStoreNote_IDMigrationLeavesNoOrphanSections(t *testing.T) {
	db := openTestDB(t)
	first := longRecord("_path:refs/long.md", "refs/long.md", "Alpha", "Beta")
	require.NoError(t, index.StoreNote(db, first))
	require.NotEmpty(t, sectionsOf(t, db, "_path:refs/long.md"))

	second := longRecord("ref-long", "refs/long.md", "Alpha", "Beta")
	second.Hash = "v2"
	require.NoError(t, index.StoreNote(db, second))

	assert.Empty(t, sectionsOf(t, db, "_path:refs/long.md"), "no orphans under the old id")
	got := sectionsOf(t, db, "ref-long")
	require.NotEmpty(t, got)
	assert.Equal(t, "ref-long#alpha", got[0].ID)
}

// Each section records the hash of the note text it was cut from. A binary
// from before sections re-indexes an edited note without touching its
// sections, and the newer indexer then skips the note as unchanged — so a
// reader must be able to tell a stale section from a current one.
func TestStoreNote_SectionsRecordTheirNoteHash(t *testing.T) {
	db := openTestDB(t)
	rec := longRecord("ref-long", "refs/long.md", "Alpha", "Beta")
	rec.Hash = "hash-v1"
	require.NoError(t, index.StoreNote(db, rec))

	var stale int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sections s JOIN notes n ON n.id = s.note_id
		WHERE s.note_hash != n.hash`).Scan(&stale))
	assert.Zero(t, stale)

	_, err := db.Exec("UPDATE notes SET hash = 'hash-v2' WHERE id = 'ref-long'") // an older binary's re-index
	require.NoError(t, err)
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sections s JOIN notes n ON n.id = s.note_id
		WHERE s.note_hash != n.hash`).Scan(&stale))
	assert.Equal(t, countSections(t, db), stale, "every section is now recognisably stale")
}

// The stripped text drops fenced code, so a section that is only a heading and
// a code block strips to nothing and is left out; the stored ordinals stay
// 0, 1, 2… with no gap where it was.
func TestSectionsFor_ACodeOnlySectionIsLeftOutWithoutAGap(t *testing.T) {
	code := "```\n" + strings.Repeat("x := 1\n", 400) + "```\n\n"
	md := "## Prose\n\n" + prose(1500) + "## Code\n\n" + code + "## More\n\n" + prose(1500)
	for len(md) < 9000*4 {
		md += prose(1500)
	}
	got := index.SectionsFor(md, md)
	require.NotEmpty(t, got)
	for i, s := range got {
		assert.Equal(t, i, s.Ordinal)
		assert.NotEqual(t, "code", s.Anchor, "a part with no text left after stripping is not stored")
	}
}

// Whether to split is decided on the text the model is given (the stripped
// body); the cut is made in the markdown, where the headings are; each
// section is stored stripped, like a note's body. Found by indexing real
// pages: splitting the stripped text found no headings and cut by length.
func TestSectionsFor_DecidesOnPlainCutsMarkdownStoresPlain(t *testing.T) {
	md := longBody("Alpha", "Beta")
	plain := strings.ReplaceAll(md, "## ", "")
	got := index.SectionsFor(md, plain)
	require.NotEmpty(t, got)
	assert.Equal(t, "alpha", got[0].Anchor)
	assert.Equal(t, "Alpha", got[0].HeadingPath)
	assert.NotContains(t, got[0].Body, "## ", "stored bodies are stripped like note bodies")

	short := "## Heading\n\n" + prose(100)
	assert.Nil(t, index.SectionsFor(short, short))
	// Markdown past the window whose stripped text fits (long URLs, markup)
	// stays one unit: the model sees the stripped text.
	assert.Nil(t, index.SectionsFor(md, prose(100)))
}
