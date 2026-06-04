package experiment

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func insertTestAskEvent(t *testing.T, db *DB, sessionID, eventID, ts string, variants map[string]any) {
	t.Helper()
	data := map[string]any{"variants": variants}
	blob, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	_, err = db.Exec(
		`INSERT INTO events
		 (event_id, session_id, event_type, timestamp, vault_path, query_text, query_mode, primary_variant, event_data)
		 VALUES (?, ?, 'ask', ?, '/tmp/v', 'q', 'hybrid', 'hybrid', ?)`,
		eventID, sessionID, ts, string(blob),
	)
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
}

func TestLoadComparableEvents_ReturnsEventsWithShadows(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	sid, err := db.StartSession("/tmp/v")
	if err != nil {
		t.Fatalf("start session: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	insertTestAskEvent(t, db, sid, "ev-1", now, map[string]any{
		"hybrid": map[string]any{
			"results": []any{
				map[string]any{"note_id": "n1", "rank": 1},
				map[string]any{"note_id": "n2", "rank": 2},
			},
		},
		"activation_v1": map[string]any{
			"results": []any{
				map[string]any{"note_id": "n2", "rank": 1},
				map[string]any{"note_id": "n1", "rank": 2},
			},
		},
	})

	events, err := db.LoadComparableEvents(ComparableEventFilter{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventID != "ev-1" {
		t.Fatalf("expected ev-1, got %s", events[0].EventID)
	}
	if len(events[0].Pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(events[0].Pairs))
	}
	if events[0].Pairs[0].ShadowVariant != "activation_v1" {
		t.Fatalf("unexpected shadow: %s", events[0].Pairs[0].ShadowVariant)
	}
}

func TestLoadComparableEvents_SkipsEventsWithoutShadows(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	sid, _ := db.StartSession("/tmp/v")
	now := time.Now().UTC().Format(time.RFC3339)

	insertTestAskEvent(t, db, sid, "ev-lonely", now, map[string]any{
		"hybrid": map[string]any{
			"results": []any{map[string]any{"note_id": "n1", "rank": 1}},
		},
	})

	events, err := db.LoadComparableEvents(ComparableEventFilter{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events (primary-only filtered), got %d", len(events))
	}
}

func TestLoadComparableEvents_FilterBySession(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	sid1, _ := db.StartSession("/tmp/v")
	sid2, _ := db.StartSession("/tmp/v")
	now := time.Now().UTC().Format(time.RFC3339)

	variants := map[string]any{
		"hybrid":        map[string]any{"results": []any{map[string]any{"note_id": "n1", "rank": 1}, map[string]any{"note_id": "n2", "rank": 2}}},
		"activation_v1": map[string]any{"results": []any{map[string]any{"note_id": "n2", "rank": 1}, map[string]any{"note_id": "n1", "rank": 2}}},
	}
	insertTestAskEvent(t, db, sid1, "ev-a", now, variants)
	insertTestAskEvent(t, db, sid2, "ev-b", now, variants)

	events, err := db.LoadComparableEvents(ComparableEventFilter{SessionID: sid1})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(events) != 1 || events[0].EventID != "ev-a" {
		t.Fatalf("session filter failed: got %+v", events)
	}
}
