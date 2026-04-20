package experiment_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartSessionWithCaller_PersistsCallerIdentityAndMeta(t *testing.T) {
	db := openTestDB(t)

	sid, err := db.StartSessionWithCaller("/vault", "workhorse-persona-hook",
		map[string]any{"project_dir": "/Users/peiman/dev/workhorse", "pid": 12345})
	require.NoError(t, err)
	require.NotEmpty(t, sid)

	got, err := db.GetSessionCaller(sid)
	require.NoError(t, err)
	assert.Equal(t, "workhorse-persona-hook", got.Caller)
	assert.Equal(t, "/Users/peiman/dev/workhorse", got.Meta["project_dir"])
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
