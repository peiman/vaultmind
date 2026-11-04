package query_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const staleNoteID = "concept-act-r"

// staleVault copies the fixture vault, indexes the copy, and returns its root
// with the db, so a test can change a file after indexing.
func staleVault(t *testing.T) (string, *index.DB) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, os.DirFS(testVaultPath)))
	cfg, err := vault.LoadConfig(root)
	require.NoError(t, err)
	dbPath := filepath.Join(t.TempDir(), "test.db")
	_, err = index.NewIndexer(root, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	db, err := index.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return root, db
}

func staleNotePath(t *testing.T, root string) string {
	t.Helper()
	p := filepath.Join(root, "concepts", "act-r.md")
	require.FileExists(t, p)
	return p
}

func deliveredTarget() *query.AskResult {
	return &query.AskResult{Context: &memory.ContextPackResult{
		TargetID: staleNoteID,
		Target:   &memory.ContextPackTarget{ID: staleNoteID, Body: "indexed text"},
		Context: []memory.ContextItem{
			{ID: "concept-spreading-activation", Body: "other", BodyIncluded: true},
		},
	}}
}

func TestMarkChangedOnDisk_Cases(t *testing.T) {
	later := time.Now().Add(time.Hour)
	cases := []struct {
		name  string
		mod   func(t *testing.T, root string, p string)
		stale bool
	}{
		{"unchanged", func(*testing.T, string, string) {}, false},
		{"touched only", func(t *testing.T, _ string, p string) { require.NoError(t, os.Chtimes(p, later, later)) }, false},
		{"edited", func(t *testing.T, _ string, p string) {
			require.NoError(t, os.WriteFile(p, []byte("---\nid: concept-act-r\n---\nnew\n"), 0o644))
			require.NoError(t, os.Chtimes(p, later, later))
		}, true},
		{"deleted", func(t *testing.T, _ string, p string) { require.NoError(t, os.Remove(p)) }, true},
		{"unreadable after touch", func(t *testing.T, _ string, p string) {
			require.NoError(t, os.Chtimes(p, later, later))
			require.NoError(t, os.Chmod(p, 0o000))
			t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, db := staleVault(t)
			c.mod(t, root, staleNotePath(t, root))
			res := deliveredTarget()

			query.MarkChangedOnDisk(res, db, root)

			assert.Equal(t, c.stale, res.Context.Target.ChangedOnDisk)
			assert.False(t, res.Context.Context[0].ChangedOnDisk, "the untouched neighbour is never marked")
			if c.stale {
				assert.Equal(t, 1, res.StaleNotes)
			} else {
				assert.Zero(t, res.StaleNotes)
			}
		})
	}
}

func TestMarkChangedOnDisk_SkipsWhatItCannotJudge(t *testing.T) {
	root, db := staleVault(t)
	require.NoError(t, os.Remove(staleNotePath(t, root)))

	query.MarkChangedOnDisk(nil, db, root)
	query.MarkChangedOnDisk(&query.AskResult{}, db, root)

	res := deliveredTarget()
	res.Context.Target.ShownBefore = true
	query.MarkChangedOnDisk(res, db, root)
	assert.False(t, res.Context.Target.ChangedOnDisk, "a title-only note delivered no text to be stale")

	res = deliveredTarget()
	res.Context.Target.Body = ""
	query.MarkChangedOnDisk(res, db, root)
	assert.False(t, res.Context.Target.ChangedOnDisk)

	res = deliveredTarget()
	res.Context.TargetID, res.Context.Target.ID = "no-such-note", "no-such-note"
	query.MarkChangedOnDisk(res, db, root)
	assert.False(t, res.Context.Target.ChangedOnDisk, "a note absent from the index is not claimed stale")

	_, err := db.Exec("UPDATE notes SET path = '../escape.md' WHERE id = ?", staleNoteID)
	require.NoError(t, err)
	res = deliveredTarget()
	query.MarkChangedOnDisk(res, db, root)
	assert.False(t, res.Context.Target.ChangedOnDisk, "a path outside the vault is never read")
}

func TestFormatAsk_MarksChangedTargetAndItem(t *testing.T) {
	res := deliveredTarget()
	res.Context.Target.ChangedOnDisk = true
	res.Context.Context[0].ChangedOnDisk = true
	var buf bytes.Buffer
	require.NoError(t, query.FormatAsk(res, &buf))
	assert.Equal(t, 2, bytes.Count(buf.Bytes(), []byte("changed on disk since indexed")))

	res.Context.Target.ChangedOnDisk, res.Context.Context[0].ChangedOnDisk = false, false
	buf.Reset()
	require.NoError(t, query.FormatAsk(res, &buf))
	assert.NotContains(t, buf.String(), "changed on disk")
}
