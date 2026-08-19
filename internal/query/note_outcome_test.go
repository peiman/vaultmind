package query_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func noteGetDB(t *testing.T) *index.DB {
	t.Helper()
	db := testvault.OpenSharedDB(t, testVaultPath, filepath.Join(t.TempDir(), "n.db"))
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// `note get` is the highest-signal event VaultMind emits — the agent named a
// note and asked for it. It was logged with args[0], the RAW INPUT, so a
// title- or path-resolved read landed on a key no lookup can ever match:
// NoteAccessTimes compares exact strings, and AccessedNoteIDs hands the bogus id
// to the retriever where QueryNoteByID returns nil and it is dropped.
//
// 82 such rows sit in a live log — `README.md`, `_path:benchmark-runs/...`,
// `dev/cli/vaultmind-oss/CHANGELOG.md`. The index ledger meanwhile records the
// RESOLVED id for the same event, so the two ledgers were keyed differently.
//
// RunNoteGet resolves internally, so the caller could not know the real id. It
// now returns what happened.
func TestRunNoteGet_ReportsResolvedIDNotRawInput(t *testing.T) {
	db := noteGetDB(t)
	var buf bytes.Buffer

	// Resolve by TITLE — the case where raw input and id differ.
	out, err := query.RunNoteGet(db, query.NoteGetConfig{Input: "Spreading Activation"}, &buf)
	require.NoError(t, err)

	assert.NotEqual(t, "Spreading Activation", out.NoteID,
		"logging the typed string produces a key nothing can match")
	assert.Equal(t, "concept-spreading-activation", out.NoteID)
	assert.True(t, out.BodyDelivered, "a plain note get renders the body")
}

// --frontmatter-only clears note.Body and prints none of it, yet the access was
// recorded with a hardcoded true and a comment reading "note get prints the
// body" — falsified by a flag on the same command.
func TestRunNoteGet_FrontmatterOnlyDeliversNoBody(t *testing.T) {
	db := noteGetDB(t)
	var buf bytes.Buffer

	out, err := query.RunNoteGet(db,
		query.NoteGetConfig{Input: "concept-spreading-activation", FrontmatterOnly: true}, &buf)
	require.NoError(t, err)

	assert.Equal(t, "concept-spreading-activation", out.NoteID)
	assert.False(t, out.BodyDelivered,
		"frontmatter-only renders no body; recording a delivery feeds activation a read that never happened")
}

// A miss must not report a delivered read. The access was logged BEFORE
// RunNoteGet ran, so `note get does-not-exist` recorded
// {"body_delivered":true,"note_id":"does-not-exist"} and — because note_get is
// trusted unconditionally — that entered activation. One such row is already in
// a live log.
func TestRunNoteGet_MissReportsNothingDelivered(t *testing.T) {
	db := noteGetDB(t)
	var buf bytes.Buffer

	out, _ := query.RunNoteGet(db, query.NoteGetConfig{Input: "no-such-note-anywhere"}, &buf)

	assert.Empty(t, out.NoteID, "an unresolved input has no id to attribute the read to")
	assert.False(t, out.BodyDelivered)
}
