package experiment_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartSession(t *testing.T) {
	db := openTestExpDB(t)

	id, err := db.StartSession("/tmp/test-vault")
	require.NoError(t, err)

	// ID should be 36 chars (UUID format: 8-4-4-4-12)
	assert.Len(t, id, 36)

	var vaultPath, startedAt string
	err = db.QueryRow(
		"SELECT vault_path, started_at FROM sessions WHERE session_id = ?", id,
	).Scan(&vaultPath, &startedAt)
	require.NoError(t, err)

	assert.Equal(t, "/tmp/test-vault", vaultPath)

	// started_at must be valid RFC3339
	_, parseErr := time.Parse(time.RFC3339, startedAt)
	assert.NoError(t, parseErr, "started_at must be valid RFC3339: %q", startedAt)
}

func TestStartSession_UniqueIDs(t *testing.T) {
	db := openTestExpDB(t)

	id1, err := db.StartSession("/tmp/vault1")
	require.NoError(t, err)

	id2, err := db.StartSession("/tmp/vault2")
	require.NoError(t, err)

	assert.NotEqual(t, id1, id2, "each StartSession must produce a unique ID")
}

func TestEndSession(t *testing.T) {
	db := openTestExpDB(t)

	id, err := db.StartSession("/tmp/test-vault")
	require.NoError(t, err)

	err = db.EndSession(id)
	require.NoError(t, err)

	var endedAt string
	err = db.QueryRow(
		"SELECT ended_at FROM sessions WHERE session_id = ?", id,
	).Scan(&endedAt)
	require.NoError(t, err)

	// ended_at must be valid RFC3339
	_, parseErr := time.Parse(time.RFC3339, endedAt)
	assert.NoError(t, parseErr, "ended_at must be valid RFC3339: %q", endedAt)
}

func TestEndSession_NullBeforeEnd(t *testing.T) {
	db := openTestExpDB(t)

	id, err := db.StartSession("/tmp/test-vault")
	require.NoError(t, err)

	var endedAt sql.NullString
	err = db.QueryRow(
		"SELECT ended_at FROM sessions WHERE session_id = ?", id,
	).Scan(&endedAt)
	require.NoError(t, err)

	assert.False(t, endedAt.Valid, "ended_at must be NULL before EndSession is called")
}

func TestRecoverOrphans_NoEvents(t *testing.T) {
	db := openTestExpDB(t)

	before := time.Now().UTC()

	id, err := db.StartSession("/tmp/test-vault")
	require.NoError(t, err)

	n, err := db.RecoverOrphans()
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	var startedAt, endedAt string
	err = db.QueryRow(
		"SELECT started_at, ended_at FROM sessions WHERE session_id = ?", id,
	).Scan(&startedAt, &endedAt)
	require.NoError(t, err)

	started, err := time.Parse(time.RFC3339, startedAt)
	require.NoError(t, err)

	ended, err := time.Parse(time.RFC3339, endedAt)
	require.NoError(t, err)

	_ = before
	// ended_at should be started_at + 1 minute
	expected := started.Add(time.Minute)
	assert.WithinDuration(t, expected, ended, time.Second,
		"orphan with no events should get started_at + 1 minute")
}

func TestRecoverOrphans_WithEvents(t *testing.T) {
	db := openTestExpDB(t)

	id, err := db.StartSession("/tmp/test-vault")
	require.NoError(t, err)

	// Insert a fake event manually so we control the timestamp.
	eventTime := time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339)
	_, err = db.Exec(
		`INSERT INTO events (event_id, session_id, event_type, timestamp, vault_path, event_data)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"test-event-id", id, "search", eventTime, "/tmp/test-vault", "{}",
	)
	require.NoError(t, err)

	n, err := db.RecoverOrphans()
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	var endedAt string
	err = db.QueryRow(
		"SELECT ended_at FROM sessions WHERE session_id = ?", id,
	).Scan(&endedAt)
	require.NoError(t, err)

	assert.Equal(t, eventTime, endedAt,
		"orphan with events should get last event timestamp")
}

func TestRecoverOrphans_SkipsAlreadyEnded(t *testing.T) {
	db := openTestExpDB(t)

	id, err := db.StartSession("/tmp/test-vault")
	require.NoError(t, err)

	err = db.EndSession(id)
	require.NoError(t, err)

	n, err := db.RecoverOrphans()
	require.NoError(t, err)
	assert.Equal(t, 0, n, "already-ended sessions must not be counted as orphans")
}
