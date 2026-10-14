package index_test

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGapClosures_MapsTheNoteThatClosedTheGap(t *testing.T) {
	db := openTestDB(t)
	early := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	opened := time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)
	closed := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)

	insertCreditNote(t, db, "gap-early", index.TypeGap, "closed")
	insertCreditNote(t, db, "gap-later", index.TypeGap, "closed")
	insertCreditNote(t, db, "gap-tie", index.TypeGap, "closed")
	insertCreditNote(t, db, "gap-open", index.TypeGap, "open")
	insertCreditNote(t, db, "not-a-gap", "concept", "closed")
	insertCreditNote(t, db, "gap-bad", index.TypeGap, "closed")
	insertCreditNote(t, db, "gap-neighbor", index.TypeGap, "closed")

	putCreditKV(t, db, "gap-later", "answer", "later question", opened, closed)
	putCreditKV(t, db, "gap-early", "answer", "how does the index stay current", early, closed)
	putCreditKV(t, db, "gap-tie", "answer", "tie question", early, closed)
	putCreditKV(t, db, "gap-open", "open-target", "still open", opened, closed)
	putCreditKV(t, db, "not-a-gap", "stray", "not a gap", opened, closed)
	putCreditKV(t, db, "gap-bad", "skipped", "bad times", time.Time{}, closed)
	putCreditKV(t, db, "gap-neighbor", "neighbor", "a neighbor question", opened, closed)

	counter := &countingQuery{RowQuery: db}
	got, err := index.GapClosures(counter)
	require.NoError(t, err)
	assert.Equal(t, 1, counter.n, "every closed gap is one query")

	credit, ok := got["answer"]
	require.True(t, ok)
	assert.Equal(t, "how does the index stay current", credit.Question)
	assert.True(t, credit.OpenedAt.Equal(early), "the earlier question wins")
	assert.True(t, credit.ClosedAt.Equal(closed))

	neighbor, ok := got["neighbor"]
	require.True(t, ok)
	assert.Equal(t, "a neighbor question", neighbor.Question)

	assert.NotContains(t, got, "open-target")
	assert.NotContains(t, got, "stray")
	assert.NotContains(t, got, "skipped")
	assert.Len(t, got, 2)

	empty, err := index.GapClosures(nil)
	require.NoError(t, err)
	assert.Empty(t, empty)

	require.NoError(t, db.Close())
	_, err = index.GapClosures(db)
	require.Error(t, err)
}

type countingQuery struct {
	index.RowQuery
	n int
}

func (c *countingQuery) Query(query string, args ...interface{}) (*sql.Rows, error) {
	c.n++
	return c.RowQuery.Query(query, args...)
}

func insertCreditNote(t *testing.T, db *index.DB, id, noteType, status string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO notes (id, path, hash, mtime, status, type) VALUES (?, ?, 'h', 1, ?, ?)`,
		id, id+".md", status, noteType)
	require.NoError(t, err)
}

func putCreditKV(t *testing.T, db *index.DB, gapID, closedBy, question string, opened, closed time.Time) {
	t.Helper()
	putKV(t, db, gapID, "closed_by", closedBy)
	putKV(t, db, gapID, "question", question)
	if !opened.IsZero() {
		putKV(t, db, gapID, "opened_at", opened.Format(time.RFC3339))
	} else {
		_, err := db.Exec(
			`INSERT INTO frontmatter_kv (note_id, key, value_json) VALUES (?, 'opened_at', ?)`,
			gapID, `"not-a-time"`)
		require.NoError(t, err)
	}
	if !closed.IsZero() {
		putKV(t, db, gapID, "closed_at", closed.Format(time.RFC3339))
	}
}

func putKV(t *testing.T, db *index.DB, noteID, key, value string) {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	_, err = db.Exec(
		`INSERT INTO frontmatter_kv (note_id, key, value_json) VALUES (?, ?, ?)`,
		noteID, key, string(raw))
	require.NoError(t, err)
}
