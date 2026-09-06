package experiment_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartSessionWithCaller_PersistsCallerIdentityAndMeta(t *testing.T) {
	db := openTestDB(t)

	sid, err := db.StartSessionWithCaller("/vault", "companion-persona-hook",
		map[string]any{"project_dir": "/Users/me/dev/companion-project", "pid": 12345})
	require.NoError(t, err)
	require.NotEmpty(t, sid)

	got, err := db.GetSessionCaller(sid)
	require.NoError(t, err)
	assert.Equal(t, "companion-persona-hook", got.Caller)
	assert.Equal(t, "/Users/me/dev/companion-project", got.Meta["project_dir"])
	assert.Equal(t, float64(12345), got.Meta["pid"])
}

func TestStartSession_DefaultsToUnknownCaller(t *testing.T) {
	// Backward compatibility: existing StartSession callers don't pass
	// caller info; they should end up with caller="" (treated as "unknown"
	// by reporting) so the schema change doesn't break anything.
	db := openTestDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	got, err := db.GetSessionCaller(sid)
	require.NoError(t, err)
	assert.Empty(t, got.Caller)
	assert.Empty(t, got.Meta)
}

func TestStartSessionWithCaller_NilMetaIsFine(t *testing.T) {
	db := openTestDB(t)
	sid, err := db.StartSessionWithCaller("/vault", "cli", nil)
	require.NoError(t, err)

	got, err := db.GetSessionCaller(sid)
	require.NoError(t, err)
	assert.Equal(t, "cli", got.Caller)
	assert.Empty(t, got.Meta)
}

// A1 slice 0 (2026-09-06): user_session_id has always been INFERRED — same
// caller+user+host within 30 minutes counts as one working session. That
// heuristic cannot tell a subagent's tool call from its parent's: both are
// caller=vaultmind-reach-hook, same user, same host, seconds apart, so they
// collapse into one "conversation". Cross-turn dedup keyed on that id would
// withhold bodies from a subagent — a mind with NO SessionStart, no identity
// load, and no context at all — which is the opposite of what it needs.
//
// The harness knows the real conversation id and puts it in every hook
// payload. When it is supplied, it wins; the heuristic remains for plain CLI
// use, where nothing better exists.

func TestUserSessionID_ExplicitIDWinsOverTheHeuristic(t *testing.T) {
	db := openTestDB(t)
	meta := map[string]any{"user": "u", "host": "h", "user_session_id": "conv-real-1"}

	sid, err := db.StartSessionWithCaller("", "vaultmind-reach-hook", meta)
	require.NoError(t, err)

	got, err := db.GetSessionCaller(sid)
	require.NoError(t, err)
	require.Equal(t, "conv-real-1", got.UserSessionID)
}

func TestUserSessionID_ExplicitIDsDefeatTheProximityCollapse(t *testing.T) {
	db := openTestDB(t)
	plain := map[string]any{"user": "u", "host": "h"}

	// FIRST establish the hazard rather than assuming it: two back-to-back
	// sessions with no explicit id collapse into ONE working session. That is
	// the collapse a subagent's tool call would suffer against its parent's.
	a, err := db.StartSessionWithCaller("", "vaultmind-reach-hook", plain)
	require.NoError(t, err)
	b, err := db.StartSessionWithCaller("", "vaultmind-reach-hook", plain)
	require.NoError(t, err)
	ac, err := db.GetSessionCaller(a)
	require.NoError(t, err)
	bc, err := db.GetSessionCaller(b)
	require.NoError(t, err)
	require.Equal(t, ac.UserSessionID, bc.UserSessionID,
		"precondition: without an explicit id the heuristic merges them — this is the hazard")

	// THEN show the explicit id is what separates them.
	c, err := db.StartSessionWithCaller("", "vaultmind-reach-hook",
		map[string]any{"user": "u", "host": "h", experiment.MetaUserSessionID: "conv-other"})
	require.NoError(t, err)
	cc, err := db.GetSessionCaller(c)
	require.NoError(t, err)
	require.NotEqual(t, ac.UserSessionID, cc.UserSessionID)
}

func TestUserSessionID_HeuristicStillAppliesWithoutAnExplicitID(t *testing.T) {
	db := openTestDB(t)
	meta := map[string]any{"user": "u", "host": "h"}

	first, err := db.StartSessionWithCaller("", "cli", meta)
	require.NoError(t, err)
	second, err := db.StartSessionWithCaller("", "cli", meta)
	require.NoError(t, err)

	fc, err := db.GetSessionCaller(first)
	require.NoError(t, err)
	sc, err := db.GetSessionCaller(second)
	require.NoError(t, err)
	require.Equal(t, fc.UserSessionID, sc.UserSessionID,
		"plain CLI use has no better signal; the time heuristic must survive")
}
