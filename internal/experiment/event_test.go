package experiment_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogEvent_Search(t *testing.T) {
	db := openTestExpDB(t)

	sessionID, err := db.StartSession("/tmp/test-vault")
	require.NoError(t, err)

	evt := experiment.Event{
		SessionID: sessionID,
		Type:      experiment.EventSearch,
		VaultPath: "/tmp/test-vault",
		QueryText: "spreading activation",
		QueryMode: "hybrid",
		Data:      map[string]any{"k": 10},
	}

	eventID, err := db.LogEvent(evt)
	require.NoError(t, err)
	assert.Len(t, eventID, 36)

	var eventType, queryText, queryMode, eventData string
	err = db.QueryRow(
		`SELECT event_type, query_text, query_mode, event_data
		 FROM events WHERE event_id = ?`, eventID,
	).Scan(&eventType, &queryText, &queryMode, &eventData)
	require.NoError(t, err)

	assert.Equal(t, experiment.EventSearch, eventType)
	assert.Equal(t, "spreading activation", queryText)
	assert.Equal(t, "hybrid", queryMode)

	var data map[string]any
	require.NoError(t, json.Unmarshal([]byte(eventData), &data))
	assert.Equal(t, float64(10), data["k"])
}

func TestLogEvent_NoteAccess(t *testing.T) {
	db := openTestExpDB(t)

	sessionID, err := db.StartSession("/tmp/test-vault")
	require.NoError(t, err)

	evt := experiment.Event{
		SessionID: sessionID,
		Type:      experiment.EventNoteAccess,
		VaultPath: "/tmp/test-vault",
		Data:      map[string]any{"note_id": "abc123"},
	}

	eventID, err := db.LogEvent(evt)
	require.NoError(t, err)
	assert.Len(t, eventID, 36)

	var eventType string
	err = db.QueryRow(
		`SELECT event_type FROM events WHERE event_id = ?`, eventID,
	).Scan(&eventType)
	require.NoError(t, err)
	assert.Equal(t, experiment.EventNoteAccess, eventType)
}

func TestLogEvent_Timestamp(t *testing.T) {
	testStart := time.Now().UTC().Truncate(time.Second)
	db := openTestExpDB(t)

	sessionID, err := db.StartSession("/tmp/test-vault")
	require.NoError(t, err)

	evt := experiment.Event{
		SessionID: sessionID,
		Type:      experiment.EventSearch,
		VaultPath: "/tmp/test-vault",
		Data:      map[string]any{},
	}

	eventID, err := db.LogEvent(evt)
	require.NoError(t, err)

	var ts string
	err = db.QueryRow(
		`SELECT timestamp FROM events WHERE event_id = ?`, eventID,
	).Scan(&ts)
	require.NoError(t, err)

	parsed, parseErr := time.Parse(time.RFC3339, ts)
	require.NoError(t, parseErr, "timestamp must be valid RFC3339: %q", ts)
	assert.False(t, parsed.Before(testStart),
		"event timestamp %v must not be before test start %v", parsed, testStart)
}

func TestLogEvent_PrimaryVariant(t *testing.T) {
	db := openTestExpDB(t)

	sessionID, err := db.StartSession("/tmp/test-vault")
	require.NoError(t, err)

	evt := experiment.Event{
		SessionID:      sessionID,
		Type:           experiment.EventSearch,
		VaultPath:      "/tmp/test-vault",
		PrimaryVariant: "bge-m3",
		Data:           map[string]any{},
	}

	eventID, err := db.LogEvent(evt)
	require.NoError(t, err)

	var primaryVariant string
	err = db.QueryRow(
		`SELECT primary_variant FROM events WHERE event_id = ?`, eventID,
	).Scan(&primaryVariant)
	require.NoError(t, err)
	assert.Equal(t, "bge-m3", primaryVariant)
}
