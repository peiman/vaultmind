package query_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedSelfDB returns a DB with three notes:
//
//	hot     — accessed 10x, 5 minutes ago
//	recent  — accessed 1x, 30 seconds ago (newest)
//	stale   — accessed 5x, 30 days ago
//
// The fixture is shaped so each section of RunSelf has a clear top hit.
func seedSelfDB(t *testing.T, now time.Time) *index.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := index.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	rows := []struct {
		id, title, last string
		count           int
	}{
		{"hot-note", "Hot", now.Add(-5 * time.Minute).UTC().Format(time.RFC3339Nano), 10},
		{"recent-note", "Recent", now.Add(-30 * time.Second).UTC().Format(time.RFC3339Nano), 1},
		{"stale-note", "Stale", now.Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339Nano), 5},
	}
	for _, r := range rows {
		_, err = db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime, access_count, last_accessed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			r.id, r.id+".md", "concept", r.title, "h", 0, r.count, r.last)
		require.NoError(t, err)
	}
	return db
}

// Empty vault prints a recognisable blank-slate message; never silent.
func TestRunSelf_EmptyVaultPrintsBlankSlateMessage(t *testing.T) {
	dir := t.TempDir()
	db, err := index.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	var buf bytes.Buffer
	require.NoError(t, query.RunSelf(db, query.SelfConfig{}, &buf))
	assert.Contains(t, buf.String(), "no accesses recorded yet")
}

// Three sections render in order with the expected top hits.
func TestRunSelf_RendersRecentHotStaleSections(t *testing.T) {
	now := time.Date(2026, 4, 29, 20, 0, 0, 0, time.UTC)
	db := seedSelfDB(t, now)

	var buf bytes.Buffer
	require.NoError(t, query.RunSelf(db, query.SelfConfig{
		Now:            now,
		Limit:          10,
		StaleThreshold: 7 * 24 * time.Hour,
		DecayD:         0.5,
	}, &buf))

	out := buf.String()
	assert.Contains(t, out, "Memory state — 3 accessed notes")
	// Section headers in the right order.
	recentIdx := strings.Index(out, "Recent (newest first):")
	hotIdx := strings.Index(out, "Hot (top activation):")
	staleIdx := strings.Index(out, "Stale (older than")
	require.True(t, recentIdx >= 0 && hotIdx > recentIdx && staleIdx > hotIdx,
		"sections must appear in Recent → Hot → Stale order; got recent=%d hot=%d stale=%d", recentIdx, hotIdx, staleIdx)

	// Recent section: recent-note should be the first ID after the
	// "Recent" header (newest first per ListAccessedNotes).
	recentBlock := out[recentIdx:hotIdx]
	hotBlock := out[hotIdx:staleIdx]
	staleBlock := out[staleIdx:]

	assert.True(t, strings.Index(recentBlock, "recent-note") < strings.Index(recentBlock, "hot-note"),
		"recent-note should appear before hot-note in Recent section")

	// Hot section: hot-note (count 10, 5m ago) outranks both others on
	// activation = ln(11) - 0.5*ln(5min/60min) = 2.4 + 1.24 = 3.64,
	// while recent-note's activation = ln(2) + (huge boost from very recent)
	// might compete. The contract is: hot-note must appear in the Hot block.
	assert.Contains(t, hotBlock, "hot-note", "hot-note must appear in Hot section")

	// Stale section: stale-note (30 days) is older than 7-day threshold.
	assert.Contains(t, staleBlock, "stale-note", "stale-note (30d ago) must appear in Stale section")
	assert.NotContains(t, staleBlock, "hot-note", "hot-note (5m ago) must NOT be in Stale section")
	assert.NotContains(t, staleBlock, "recent-note", "recent-note (30s ago) must NOT be in Stale section")
}

// When all accesses are fresh, the Stale section announces "none" rather
// than a missing/silent failure.
func TestRunSelf_NoStaleNotesPrintsNoneLine(t *testing.T) {
	now := time.Date(2026, 4, 29, 20, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	db, err := index.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime, access_count, last_accessed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"fresh", "fresh.md", "concept", "Fresh", "h", 0, 1, now.Add(-time.Minute).UTC().Format(time.RFC3339Nano))
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, query.RunSelf(db, query.SelfConfig{
		Now:            now,
		StaleThreshold: 7 * 24 * time.Hour,
	}, &buf))

	out := buf.String()
	assert.Contains(t, out, "Stale (older than 168h0m0s): none")
}

// agoString covers each branch (just-now, minutes, hours, days, ?). Pure
// formatting; locks rendering so a future refactor doesn't silently
// shift the units.
func TestRunSelf_RendersAgoBucketsAcrossElapsedRanges(t *testing.T) {
	now := time.Date(2026, 4, 29, 20, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	db, err := index.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	cases := []struct {
		id      string
		elapsed time.Duration
		want    string
	}{
		{"a-now", 10 * time.Second, "just now"},
		{"a-min", 5 * time.Minute, "5m"},
		{"a-hr", 3 * time.Hour, "3h"},
		{"a-day", 5 * 24 * time.Hour, "5d"},
	}
	for _, c := range cases {
		_, err = db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime, access_count, last_accessed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			c.id, c.id+".md", "concept", c.id, "h", 0, 1,
			now.Add(-c.elapsed).UTC().Format(time.RFC3339Nano))
		require.NoError(t, err)
	}

	var buf bytes.Buffer
	require.NoError(t, query.RunSelf(db, query.SelfConfig{Now: now}, &buf))
	out := buf.String()
	for _, c := range cases {
		assert.Contains(t, out, c.want, "want %q for %s", c.want, c.id)
	}
}

// selfTruncate's edge cases via long ID rendering: very long ID gets
// "..." suffix, short ID is left alone.
func TestRunSelf_TruncatesLongIDsInOutput(t *testing.T) {
	now := time.Date(2026, 4, 29, 20, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	db, err := index.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	longID := "concept-this-is-a-deliberately-long-identifier-that-exceeds-the-fifty-rune-truncation-window"
	_, err = db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime, access_count, last_accessed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		longID, longID+".md", "concept", "Long", "h", 0, 1, now.UTC().Format(time.RFC3339Nano))
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, query.RunSelf(db, query.SelfConfig{Now: now}, &buf))
	assert.Contains(t, buf.String(), "...", "long ID must render with truncation marker")
}

// Limit caps both the recent and hot sections to N rows.
func TestRunSelf_LimitCapsRowsPerSection(t *testing.T) {
	now := time.Date(2026, 4, 29, 20, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	db, err := index.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	defer db.Close()

	for i := 0; i < 15; i++ {
		id := "note-" + string(rune('a'+i))
		_, err = db.Exec(`INSERT INTO notes (id, path, type, title, hash, mtime, access_count, last_accessed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, id+".md", "concept", "T", "h", int64(i), 1,
			now.Add(-time.Duration(i)*time.Minute).UTC().Format(time.RFC3339Nano))
		require.NoError(t, err)
	}

	var buf bytes.Buffer
	require.NoError(t, query.RunSelf(db, query.SelfConfig{
		Now:   now,
		Limit: 3,
	}, &buf))

	// Each "note-X" id appears once in Recent and once in Hot — so at
	// most Limit*2 = 6 occurrences. (Stale section is empty since all
	// notes are < 7 days old.)
	count := strings.Count(buf.String(), "note-")
	assert.LessOrEqual(t, count, 6, "Limit=3 must cap each section to 3 rows; got %d total", count)
}
