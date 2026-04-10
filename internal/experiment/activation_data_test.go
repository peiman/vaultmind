package experiment_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoteAccessTimes_Empty(t *testing.T) {
	db := openTestExpDB(t)
	times, err := db.NoteAccessTimes("nonexistent")
	require.NoError(t, err)
	assert.Empty(t, times)
}

func TestNoteAccessTimes_ReturnsAccessEvents(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		_, err = db.LogEvent(experiment.Event{
			SessionID: sid, Type: experiment.EventNoteAccess, VaultPath: "/vault",
			Data: map[string]any{"note_id": "note-a", "source": "note_get"},
		})
		require.NoError(t, err)
	}
	// Different note
	_, _ = db.LogEvent(experiment.Event{
		SessionID: sid, Type: experiment.EventNoteAccess, VaultPath: "/vault",
		Data: map[string]any{"note_id": "note-b", "source": "note_get"},
	})

	times, err := db.NoteAccessTimes("note-a")
	require.NoError(t, err)
	assert.Len(t, times, 2)
}

func TestNoteAccessTimes_IgnoresSearchEvents(t *testing.T) {
	db := openTestExpDB(t)
	sid, _ := db.StartSession("/vault")
	_, _ = db.LogEvent(experiment.Event{
		SessionID: sid, Type: experiment.EventSearch, VaultPath: "/vault",
		Data: map[string]any{"note_id": "note-a"},
	})

	times, err := db.NoteAccessTimes("note-a")
	require.NoError(t, err)
	assert.Empty(t, times)
}

func TestRecentSessionWindows(t *testing.T) {
	db := openTestExpDB(t)
	for i := 0; i < 3; i++ {
		s, _ := db.StartSession("/vault")
		_ = db.EndSession(s)
	}

	windows, err := db.RecentSessionWindows(2)
	require.NoError(t, err)
	assert.Len(t, windows, 2)
}

func TestRecentSessionWindows_SkipsOrphans(t *testing.T) {
	db := openTestExpDB(t)
	s1, _ := db.StartSession("/vault")
	_ = db.EndSession(s1)
	_, _ = db.StartSession("/vault") // orphan

	windows, err := db.RecentSessionWindows(10)
	require.NoError(t, err)
	assert.Len(t, windows, 1)
}

func TestBatchNoteAccessTimes(t *testing.T) {
	db := openTestExpDB(t)
	sid, _ := db.StartSession("/vault")
	for _, nid := range []string{"note-a", "note-a", "note-b"} {
		_, _ = db.LogEvent(experiment.Event{
			SessionID: sid, Type: experiment.EventNoteAccess, VaultPath: "/vault",
			Data: map[string]any{"note_id": nid, "source": "note_get"},
		})
	}

	batch, err := db.BatchNoteAccessTimes([]string{"note-a", "note-b", "note-c"})
	require.NoError(t, err)
	assert.Len(t, batch["note-a"], 2)
	assert.Len(t, batch["note-b"], 1)
	assert.Empty(t, batch["note-c"])
}

func TestAccessedNoteIDs(t *testing.T) {
	db := openTestExpDB(t)
	sid, _ := db.StartSession("/vault")
	for _, nid := range []string{"note-a", "note-a", "note-b"} {
		_, _ = db.LogEvent(experiment.Event{
			SessionID: sid, Type: experiment.EventNoteAccess, VaultPath: "/vault",
			Data: map[string]any{"note_id": nid, "source": "note_get"},
		})
	}

	ids, err := db.AccessedNoteIDs()
	require.NoError(t, err)
	assert.Len(t, ids, 2)
	assert.Contains(t, ids, "note-a")
	assert.Contains(t, ids, "note-b")
}
