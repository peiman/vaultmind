package experiment

import (
	"testing"
)

// A typo in a source string does not fail — it silently makes the event
// invisible to activation, because IsActivationSignal default-denies anything it
// does not recognise. That already happened: a fixture seeding "search"
// asserted against an empty score map while appearing to exercise the path, and
// nothing anywhere reported a problem.
//
// AccessSource being a defined type turns that class into a compile error. The
// VALUES still have to be pinned by a test: they are on disk in every existing
// log, so changing one silently reclassifies history.
func TestAccessSourceValues(t *testing.T) {
	cases := map[AccessSource]string{
		AccessSourceRead:      "note_get",
		AccessSourceAsk:       "ask",
		AccessSourceNeighbors: "neighbors",
		AccessSourceRecall:    "recall",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("source value drifted: got %q, want %q — these strings are in every log on disk", got, want)
		}
	}
}

// The consumption meter and the activation scorer both answer "was this a real
// read", and they answered it from two separate places: the meter hardcoded the
// SQL literal 'note_get' while the scorer used a constant. Renaming the constant
// would have desynchronised them with nothing failing.
//
// Asserted through BEHAVIOUR rather than by string-matching the SQL, because the
// question is what the meter counts, not how it spells it.
func TestMemoryUse_CountsOnlyDeliberateReads(t *testing.T) {
	db, err := OpenMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	sessionID, err := db.StartSession("/v")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	session := &Session{DB: db, ID: sessionID, VaultPath: "/v"}

	// One deliberate read, and one of every source that must NOT count.
	for _, tc := range []struct {
		note   string
		source AccessSource
	}{
		{"read-note", AccessSourceRead},
		{"ask-note", AccessSourceAsk},
		{"neighbor-note", AccessSourceNeighbors},
	} {
		if _, err := session.LogNoteAccessEvent(tc.note, tc.source, true); err != nil {
			t.Fatalf("log %s: %v", tc.source, err)
		}
	}

	var reads int
	err = db.QueryRow(
		`SELECT COUNT(*) FROM events
		 WHERE event_type = 'note_access'
		   AND json_extract(event_data, '$.source') = ?`,
		string(AccessSourceRead),
	).Scan(&reads)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if reads != 1 {
		t.Errorf("the meter's source predicate matched %d rows, want 1 — if this drifts from "+
			"IsActivationSignal, the meter and the scorer disagree about what a read is", reads)
	}
}
