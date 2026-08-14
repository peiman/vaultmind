package distill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The desk is the highest-signal arc material a mind produces, and until now the
// candidate scanner ignored it entirely — it grepped session transcripts for a
// handful of phrases while the notes written *specifically* to record
// transformations sat unread one directory over.
//
// The asymmetry matters. An episode candidate is a guess: a phrase fired, go
// look. A desk entry is a judgement already made: the mind stopped mid-session
// and decided this was worth keeping. So desk entries are not scored, filtered,
// or ranked here — every undistilled one is surfaced, because the filtering
// already happened when it was written.
//
// "Undistilled" is recorded by the entry itself via `distilled_to:`, so the
// state lives with the note rather than in a cross-vault link the graph would
// have to maintain.

func writeNote(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

const deskEntryFixture = `---
id: journal-2026-08-13-the-stranger-test
type: journal
date: 2026-08-13
title: The stranger test
---

Body.
`

func TestScanDesk_SurfacesUndistilledJournalEntries(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "journal/2026-08-13-the-stranger-test.md", deskEntryFixture)

	entries, _, err := ScanDesk(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "journal-2026-08-13-the-stranger-test", entries[0].ID)
	assert.Equal(t, "The stranger test", entries[0].Title)
	assert.Equal(t, "2026-08-13", entries[0].Date)
}

func TestScanDesk_OmitsEntriesAlreadyDistilled(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "journal/done.md", `---
id: journal-2026-06-03-the-founding
type: journal
date: 2026-06-03
title: The founding
distilled_to: arc-the-desk-is-mine
---

Body.
`)

	entries, _, err := ScanDesk(dir)
	require.NoError(t, err)
	assert.Empty(t, entries,
		"an entry that names the arc it became is done; re-surfacing it trains the reader to ignore the list")
}

func TestScanDesk_IgnoresNonJournalTypes(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "OWNER.md", "---\nid: manifest-x\ntype: manifest\ntitle: Manifest\n---\n\nBody.\n")
	writeNote(t, dir, "arcs/a.md", "---\nid: arc-x\ntype: arc\ntitle: An arc\n---\n\nBody.\n")

	entries, _, err := ScanDesk(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "only journal entries are raw arc material")
}

func TestScanDesk_SkipsNotesWithoutFrontmatterAndDotDirs(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "README.md", "# Not a note\n")
	writeNote(t, dir, ".vaultmind/cache.md", "---\nid: journal-cache\ntype: journal\ntitle: Cache\n---\n")

	entries, _, err := ScanDesk(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a vault's own dotdirs and un-frontmattered files are not desk entries")
}

func TestScanDesk_NewestFirst(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "journal/old.md", "---\nid: journal-old\ntype: journal\ndate: 2026-06-03\ntitle: Old\n---\n")
	writeNote(t, dir, "journal/new.md", "---\nid: journal-new\ntype: journal\ndate: 2026-08-13\ntitle: New\n---\n")

	entries, _, err := ScanDesk(dir)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "journal-new", entries[0].ID, "most recent first — the freshest transformation is the one still recoverable")
}

// A missing desk is the normal case for a vault that has no desk, not an error:
// arc candidates must keep working for anyone who never made one.
func TestScanDesk_MissingDirectoryIsNotAnError(t *testing.T) {
	entries, _, err := ScanDesk(filepath.Join(t.TempDir(), "nope"))
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// An entry with no id can't be pointed at by `note get` or linked from the arc
// it becomes, so it is surfaced with its path and said to be unciteable rather
// than dropped — dropping it would hide exactly the entries that need fixing.
func TestScanDesk_EntryWithoutIDIsStillSurfaced(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "journal/no-id.md", "---\ntype: journal\ndate: 2026-08-13\ntitle: No id\n---\n")

	entries, _, err := ScanDesk(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Empty(t, entries[0].ID)
	assert.Contains(t, entries[0].Path, "no-id.md")
}

// The empty-candidates line must not contradict the desk section above it.
// Printing "No candidate moments found" directly beneath four surfaced entries
// is the same reports-nothing-while-holding-something pattern this scanner
// exists to fix; a reader who sees it once stops trusting the summary line.
func TestFormatReport_DoesNotSayNothingFoundWhenDeskHasEntries(t *testing.T) {
	var buf strings.Builder
	err := FormatReport(Report{
		DeskPending: []DeskEntry{{ID: "journal-x", Title: "T", Date: "2026-08-13"}},
	}, &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "journal-x", "the entry is still listed")
	assert.NotContains(t, out, "No candidate moments found",
		"there IS material to act on; saying nothing was found contradicts the list above it")
	assert.Contains(t, out, "No phrase-matched moments",
		"be specific about which source came up empty, rather than implying both did")
}

// With neither source populated, the honest empty message stays.
func TestFormatReport_TrulyEmptyStillSaysSo(t *testing.T) {
	var buf strings.Builder
	require.NoError(t, FormatReport(Report{}, &buf))
	assert.Contains(t, buf.String(), "No candidate moments found")
}

// A desk root that exists but cannot be read is NOT an empty backlog. Reporting
// "nothing pending" for a permissions failure hides entries that are sitting
// right there — the same silent-success shape this scanner was built to end.
func TestScanDesk_UnreadableRootIsAnError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "locked")
	require.NoError(t, os.MkdirAll(sub, 0o750))
	writeNote(t, sub, "journal/e.md", deskEntryFixture)
	require.NoError(t, os.Chmod(sub, 0o000))
	t.Cleanup(func() { _ = os.Chmod(sub, 0o750) })

	_, _, err := ScanDesk(sub)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "locked")
}

// A non-directory path is not a desk, and is not an error either.
func TestScanDesk_FilePathIsNotADesk(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "notadir.md")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))

	entries, _, err := ScanDesk(f)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// A frontmatter value that is neither string nor date still surfaces rather
// than vanishing — a silently dropped field is indistinguishable from an absent
// one, and the reader can't tell which they're looking at.
func TestScanDesk_NonStringFieldStillSurfaces(t *testing.T) {
	dir := t.TempDir()
	writeNote(t, dir, "journal/n.md", "---\nid: journal-n\ntype: journal\ndate: 2026-08-13\ntitle: 12345\n---\n")

	entries, _, err := ScanDesk(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "12345", entries[0].Title)
}

// The no-id case has to be visible in the RENDERED output, not just the struct:
// an entry nothing can cite is exactly the one a reader must be told to fix.
func TestFormatReport_UnciteableEntryIsLabelled(t *testing.T) {
	var buf strings.Builder
	require.NoError(t, FormatReport(Report{
		DeskPending: []DeskEntry{{Title: "No id", Date: "2026-08-13", Path: "journal/no-id.md"}},
	}, &buf))

	out := buf.String()
	assert.Contains(t, out, "journal/no-id.md")
	assert.Contains(t, out, "no id", "say why it can't be referenced, rather than printing a blank handle")
}

// Both sources populated: desk entries lead, phrase matches follow.
func TestFormatReport_DeskEntriesPrintAboveCandidates(t *testing.T) {
	var buf strings.Builder
	require.NoError(t, FormatReport(Report{
		EpisodesScanned: 1,
		EpisodesKept:    1,
		DeskPending:     []DeskEntry{{ID: "journal-x", Title: "T", Date: "2026-08-13"}},
		Candidates: []Candidate{{
			Rule: RuleAuthorityGrant, EpisodeID: "episode-1", TurnIndex: 3,
			Verbatim: "you decide", Trigger: "you decide",
		}},
	}, &buf))

	out := buf.String()
	deskAt, candAt := strings.Index(out, "journal-x"), strings.Index(out, "episode-1")
	require.NotEqual(t, -1, deskAt)
	require.NotEqual(t, -1, candAt)
	assert.Less(t, deskAt, candAt, "judgements outrank guesses; printing guesses first buries them")
}

// The section renderer must propagate write failures rather than swallow them.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, assert.AnError }

func TestFormatReport_PropagatesWriteErrors(t *testing.T) {
	err := FormatReport(Report{
		DeskPending: []DeskEntry{{ID: "journal-x", Title: "T", Date: "2026-08-13"}},
	}, failingWriter{})
	require.Error(t, err)
}
