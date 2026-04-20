package experiment_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserSessionID_FirstSessionGetsNewID(t *testing.T) {
	db := openTestDB(t)
	id, err := db.StartSessionWithCaller("/vault", "cli",
		map[string]any{"user": "peiman", "host": "laptop"})
	require.NoError(t, err)

	got, err := db.GetSessionCaller(id)
	require.NoError(t, err)
	assert.NotEmpty(t, got.UserSessionID, "first session mints a new user-session id")
}

func TestUserSessionID_SubsequentSessionWithinThresholdReuses(t *testing.T) {
	// Two invocations from the same caller+user+host within the threshold
	// should share a user-session id — they're the same working session.
	db := openTestDB(t)
	meta := map[string]any{"user": "peiman", "host": "laptop"}

	id1, err := db.StartSessionWithCaller("/vault", "cli", meta)
	require.NoError(t, err)
	id2, err := db.StartSessionWithCaller("/vault", "cli", meta)
	require.NoError(t, err)

	c1, err := db.GetSessionCaller(id1)
	require.NoError(t, err)
	c2, err := db.GetSessionCaller(id2)
	require.NoError(t, err)
	assert.Equal(t, c1.UserSessionID, c2.UserSessionID,
		"back-to-back invocations should share user-session")
}

func TestUserSessionID_DifferentCallerGetsOwnSession(t *testing.T) {
	// Workhorse's automated persona load and my manual cli query are
	// different kinds of events — group them separately even though they
	// share user + host + time.
	db := openTestDB(t)
	meta := map[string]any{"user": "peiman", "host": "laptop"}

	cliID, err := db.StartSessionWithCaller("/vault", "cli", meta)
	require.NoError(t, err)
	hookID, err := db.StartSessionWithCaller("/vault", "workhorse-persona-hook", meta)
	require.NoError(t, err)

	cliC, err := db.GetSessionCaller(cliID)
	require.NoError(t, err)
	hookC, err := db.GetSessionCaller(hookID)
	require.NoError(t, err)
	assert.NotEqual(t, cliC.UserSessionID, hookC.UserSessionID,
		"different callers should not share a user-session")
}

func TestUserSessionID_DifferentUserGetsOwnSession(t *testing.T) {
	// Peiman on his laptop and Siavoush on his — different operators,
	// separate user-sessions regardless of time proximity.
	db := openTestDB(t)

	p, err := db.StartSessionWithCaller("/vault", "cli",
		map[string]any{"user": "peiman", "host": "laptop"})
	require.NoError(t, err)
	s, err := db.StartSessionWithCaller("/vault", "cli",
		map[string]any{"user": "siavoush", "host": "laptop"})
	require.NoError(t, err)

	pc, _ := db.GetSessionCaller(p)
	sc, _ := db.GetSessionCaller(s)
	assert.NotEqual(t, pc.UserSessionID, sc.UserSessionID)
}

func TestUserSessionID_StaleSessionGetsNewID(t *testing.T) {
	// Simulate a gap > threshold: manually insert an older session, then
	// start a new one. The new one should mint a new user-session id.
	db := openTestDB(t)
	oldTs := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)

	_, err := db.Exec(
		`INSERT INTO sessions (session_id, vault_path, started_at, caller, caller_meta, user_session_id)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"old-session", "/vault", oldTs, "cli",
		`{"user":"peiman","host":"laptop"}`, "old-user-session")
	require.NoError(t, err)

	newID, err := db.StartSessionWithCaller("/vault", "cli",
		map[string]any{"user": "peiman", "host": "laptop"})
	require.NoError(t, err)

	c, err := db.GetSessionCaller(newID)
	require.NoError(t, err)
	assert.NotEqual(t, "old-user-session", c.UserSessionID,
		"stale session beyond threshold should not be reused")
	assert.NotEmpty(t, c.UserSessionID)
}

func TestSessionsByUserSession_ReturnsAllInvocationSessions(t *testing.T) {
	db := openTestDB(t)
	meta := map[string]any{"user": "peiman", "host": "laptop"}

	id1, err := db.StartSessionWithCaller("/vault", "cli", meta)
	require.NoError(t, err)
	id2, err := db.StartSessionWithCaller("/vault", "cli", meta)
	require.NoError(t, err)

	c1, _ := db.GetSessionCaller(id1)
	got, err := db.SessionsByUserSession(c1.UserSessionID)
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Contains(t, got, id1)
	assert.Contains(t, got, id2)
}
