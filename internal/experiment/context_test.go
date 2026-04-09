package experiment_test

import (
	"context"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithSession_RoundTrip(t *testing.T) {
	db := openTestExpDB(t)

	sess := &experiment.Session{
		DB:        db,
		ID:        "test-session-id",
		VaultPath: "/tmp/test-vault",
	}

	ctx := experiment.WithSession(context.Background(), sess)
	got := experiment.FromContext(ctx)

	require.NotNil(t, got)
	assert.Equal(t, "test-session-id", got.ID)
	assert.Equal(t, "/tmp/test-vault", got.VaultPath)
	assert.Same(t, db, got.DB)
}

func TestFromContext_NilWhenMissing(t *testing.T) {
	got := experiment.FromContext(context.Background())
	assert.Nil(t, got)
}

func TestSession_LogSearchEvent(t *testing.T) {
	db := openTestExpDB(t)
	sessionID, err := db.StartSession("/vault")
	require.NoError(t, err)

	session := &experiment.Session{DB: db, ID: sessionID, VaultPath: "/vault"}
	eventID, err := session.LogSearchEvent("test query", "hybrid", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, eventID)

	var eventType string
	err = db.QueryRow("SELECT event_type FROM events WHERE event_id = ?", eventID).Scan(&eventType)
	require.NoError(t, err)
	assert.Equal(t, "search", eventType)
}

func TestSession_LogAskEvent(t *testing.T) {
	db := openTestExpDB(t)
	sessionID, err := db.StartSession("/vault")
	require.NoError(t, err)

	session := &experiment.Session{DB: db, ID: sessionID, VaultPath: "/vault"}
	eventID, err := session.LogAskEvent("what is activation?", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, eventID)

	var eventType string
	err = db.QueryRow("SELECT event_type FROM events WHERE event_id = ?", eventID).Scan(&eventType)
	require.NoError(t, err)
	assert.Equal(t, "ask", eventType)
}

func TestSession_LogContextPackEvent(t *testing.T) {
	db := openTestExpDB(t)
	sessionID, err := db.StartSession("/vault")
	require.NoError(t, err)

	session := &experiment.Session{DB: db, ID: sessionID, VaultPath: "/vault"}
	eventID, err := session.LogContextPackEvent(map[string]any{"items": 5})
	require.NoError(t, err)
	assert.NotEmpty(t, eventID)

	var eventType string
	err = db.QueryRow("SELECT event_type FROM events WHERE event_id = ?", eventID).Scan(&eventType)
	require.NoError(t, err)
	assert.Equal(t, "context_pack", eventType)
}

func TestSession_LogNoteAccessEvent(t *testing.T) {
	db := openTestExpDB(t)
	sessionID, err := db.StartSession("/vault")
	require.NoError(t, err)

	session := &experiment.Session{DB: db, ID: sessionID, VaultPath: "/vault"}
	eventID, err := session.LogNoteAccessEvent("note-a", "note_get")
	require.NoError(t, err)
	assert.NotEmpty(t, eventID)

	var eventType string
	err = db.QueryRow("SELECT event_type FROM events WHERE event_id = ?", eventID).Scan(&eventType)
	require.NoError(t, err)
	assert.Equal(t, "note_access", eventType)
}
