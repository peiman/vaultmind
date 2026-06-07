// Package experiment_test — coverage gap tests for branch/function paths not
// reached by existing tests.
//
// Targets (in order of impact):
//   - rollup.go: CountSessions, CountEvents, CountOutcomes, WriteRollup,
//     MarshalJSON (all at 0%)
//   - context.go: outcomeWindow default (OutcomeWindow == 0 branch)
//   - telemetry.go: PromptTelemetry when reader is empty (scanner.Scan=false)
//   - caller.go: GetSessionCaller with unknown session ID (ErrNoRows path),
//     SessionsByUserSession with unknown user-session (returns empty)
//   - export.go: sessionRecord full-tier fields (CallerMeta, EndedAt, Caller,
//     UserSessionID); eventRecord full-tier with QueryText; outcomeRecord
//     full-tier note_id
//   - compare_db.go: LoadComparableEvents with SessionID/Caller/SinceRFC3339
//     filter options
//   - usage.go: percentile sentinel (len==0 guard)
//   - calibration.go: StoreCalibration round-trip (both LatestCalibration and
//     LatestCalibrationForVault nil-return paths already covered; fill the
//     StoreCalibration success path that bumps coverage from 75%)
//   - report.go: Report with no variants listed (hits the early-return on
//     EventCount==0 branch via empty variants list when events exist)
package experiment_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// rollup.go — CountSessions / CountEvents / CountOutcomes / WriteRollup /
//             MarshalJSON (all previously 0%)
// ---------------------------------------------------------------------------

func TestCountSessions_EmptyDB(t *testing.T) {
	db := openTestExpDB(t)
	n, err := db.CountSessions()
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestCountSessions_AfterInsert(t *testing.T) {
	db := openTestExpDB(t)
	_, err := db.StartSession("/vault")
	require.NoError(t, err)
	_, err = db.StartSession("/vault")
	require.NoError(t, err)

	n, err := db.CountSessions()
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestCountEvents_EmptyDB(t *testing.T) {
	db := openTestExpDB(t)
	n, err := db.CountEvents()
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestCountEvents_AfterInsert(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID: sid, Type: experiment.EventAsk, VaultPath: "/vault",
		Data: map[string]any{},
	})
	require.NoError(t, err)

	n, err := db.CountEvents()
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

func TestCountOutcomes_EmptyDB(t *testing.T) {
	db := openTestExpDB(t)
	n, err := db.CountOutcomes()
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestCountOutcomes_AfterLinkOutcomes(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID:      sid,
		Type:           experiment.EventAsk,
		VaultPath:      "/vault",
		PrimaryVariant: "primary",
		Data: map[string]any{
			"variants": map[string]any{
				"primary": map[string]any{
					"results": []any{
						map[string]any{"note_id": "n1", "rank": float64(1)},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	count, err := db.LinkOutcomes(sid, "n1", 1)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	n, err := db.CountOutcomes()
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

// WriteRollup encodes a Rollup as indented JSON to the writer.
func TestWriteRollup_EmitsValidJSON(t *testing.T) {
	r := &experiment.Rollup{
		Kind:          "rollup",
		SchemaVersion: 1,
		Tier:          experiment.TelemetryAnonymous,
		Fingerprint:   "abc123",
		NoteCount:     42,
		SessionCount:  5,
		EventCount:    20,
		OutcomeCount:  3,
		ExportedAt:    "2026-01-01T00:00:00Z",
		VariantStats: map[string]*experiment.VariantStats{
			"primary": {
				Name:         "primary",
				EventCount:   10,
				OutcomeCount: 3,
				HitAt5:       0.9,
				HitAt10:      1.0,
				MRR:          0.75,
			},
		},
	}

	var buf bytes.Buffer
	err := experiment.WriteRollup(r, &buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"kind"`)
	assert.Contains(t, buf.String(), `"rollup"`)
	assert.Contains(t, buf.String(), `"note_count"`)

	// Output must be valid JSON.
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	assert.Equal(t, float64(42), decoded["note_count"])
	assert.Equal(t, "rollup", decoded["kind"])
}

// MarshalJSON on *Rollup must produce the same JSON as encoding/json would,
// since the method delegates to an alias type to avoid infinite recursion.
func TestRollupMarshalJSON_RoundTrips(t *testing.T) {
	r := &experiment.Rollup{
		Kind:          "rollup",
		SchemaVersion: 2,
		Tier:          experiment.TelemetryFull,
		Fingerprint:   "fp-xyz",
		NoteCount:     7,
	}

	b, err := json.Marshal(r)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, "rollup", got["kind"])
	assert.Equal(t, float64(7), got["note_count"])
	assert.Equal(t, float64(2), got["schema_version"])
}

// WriteRollup surfaces writer errors.
func TestWriteRollup_WriterErrorSurfaced(t *testing.T) {
	r := &experiment.Rollup{Kind: "rollup", SchemaVersion: 1}
	w := failingWriter{err: assertedWriteError}
	err := experiment.WriteRollup(r, w)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write rollup")
}

// ---------------------------------------------------------------------------
// context.go — outcomeWindow default branch (OutcomeWindow == 0)
// ---------------------------------------------------------------------------

// outcomeWindow defaults to 2 when OutcomeWindow is zero. We verify the
// behavior indirectly: with window=0 (default) the session looks back at
// the 2 most-recent sessions, so a note_access event links to an event in a
// prior session that falls within the default window.
func TestSession_OutcomeWindowDefaultsToTwo(t *testing.T) {
	db := openTestExpDB(t)

	// Session 1: log a retrieval event for note-z.
	s1, err := db.StartSession("/vault")
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID: s1, Type: experiment.EventSearch, VaultPath: "/vault",
		Data: map[string]any{
			"variants": map[string]any{
				"hybrid": map[string]any{
					"results": []any{
						map[string]any{"note_id": "note-z", "rank": float64(1)},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	// Session 2 (current): OutcomeWindow=0 → default=2, so it looks back
	// including s1.  Accessing note-z should create an outcome row.
	s2, err := db.StartSession("/vault")
	require.NoError(t, err)
	session := &experiment.Session{
		DB:            db,
		ID:            s2,
		VaultPath:     "/vault",
		OutcomeWindow: 0, // explicitly zero → must use default (2)
	}
	_, err = session.LogNoteAccessEvent("note-z", "note_get")
	require.NoError(t, err)

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM outcomes").Scan(&count))
	assert.Equal(t, 1, count, "default outcome window of 2 should link back to the prior session")
}

// ---------------------------------------------------------------------------
// context.go — LogContextPackEvent with nil data path (nil → empty map guard)
// ---------------------------------------------------------------------------

// LogContextPackEvent with a nil data map must not panic and must produce an
// event row in the DB.
func TestSession_LogContextPackEvent_NilDataDefaultsToEmptyMap(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	session := &experiment.Session{DB: db, ID: sid, VaultPath: "/vault"}
	eventID, err := session.LogContextPackEvent(nil)
	require.NoError(t, err)
	require.NotEmpty(t, eventID)

	var evType string
	require.NoError(t, db.QueryRow("SELECT event_type FROM events WHERE event_id = ?", eventID).Scan(&evType))
	assert.Equal(t, "context_pack", evType)
}

// ---------------------------------------------------------------------------
// telemetry.go — PromptTelemetry with empty reader (scanner.Scan returns false)
// ---------------------------------------------------------------------------

// When the reader is exhausted before a line is read, PromptTelemetry must
// return the anonymous default rather than panicking or returning the empty string.
func TestPromptTelemetry_EmptyReaderDefaultsToAnonymous(t *testing.T) {
	var out bytes.Buffer
	result := experiment.PromptTelemetry(strings.NewReader(""), &out)
	assert.Equal(t, experiment.TelemetryAnonymous, result,
		"empty reader should fall back to anonymous default")
}

// ---------------------------------------------------------------------------
// caller.go — GetSessionCaller with unknown session ID (ErrNoRows path)
// ---------------------------------------------------------------------------

// GetSessionCaller on a non-existent session must return zero-value
// SessionCaller and no error (consistent "not found" semantics).
func TestGetSessionCaller_UnknownSessionReturnsZeroValue(t *testing.T) {
	db := openTestExpDB(t)
	got, err := db.GetSessionCaller("does-not-exist")
	require.NoError(t, err)
	assert.Empty(t, got.Caller)
	assert.Nil(t, got.Meta)
	assert.Empty(t, got.UserSessionID)
}

// ---------------------------------------------------------------------------
// caller.go — SessionsByUserSession with unknown user-session ID
// ---------------------------------------------------------------------------

// SessionsByUserSession for an ID that matches no rows returns an empty slice
// (not an error).
func TestSessionsByUserSession_UnknownUserSessionReturnsEmpty(t *testing.T) {
	db := openTestExpDB(t)
	ids, err := db.SessionsByUserSession("unknown-user-session-id")
	require.NoError(t, err)
	assert.Empty(t, ids)
}

// ---------------------------------------------------------------------------
// export.go — sessionRecord: CallerMeta and EndedAt present under Full tier
// ---------------------------------------------------------------------------

// ExportToJSONL under Full tier must emit session rows with vault_path,
// ended_at, caller, caller_meta, and user_session_id when those fields are
// set.  The existing tests cover Anonymous and some Full paths, but not the
// full-field session record.
func TestExportToJSONL_FullTierSessionCarriesAllFields(t *testing.T) {
	db := openTestExpDB(t)

	sid, err := db.StartSessionWithCaller("/Users/me/vault", "cli",
		map[string]any{"user": "testuser", "host": "testhostname"})
	require.NoError(t, err)
	require.NoError(t, db.EndSession(sid))

	var buf bytes.Buffer
	require.NoError(t, experiment.ExportToJSONL(db, experiment.TelemetryFull, &buf))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	// Manifest + at least one session record.
	require.GreaterOrEqual(t, len(lines), 2)

	var sessionRec map[string]any
	for _, ln := range lines[1:] {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(ln), &rec))
		if rec["kind"] == "session" {
			sessionRec = rec
			break
		}
	}
	require.NotNil(t, sessionRec, "session record must be present")
	assert.Equal(t, "/Users/me/vault", sessionRec["vault_path"],
		"Full tier must include vault_path")
	assert.Contains(t, sessionRec, "ended_at",
		"ended_at must appear when session was closed")
	assert.Equal(t, "cli", sessionRec["caller"],
		"caller label must appear when set")
	assert.Contains(t, sessionRec, "caller_meta",
		"Full tier must include caller_meta when present")
}

// ---------------------------------------------------------------------------
// compare_db.go — LoadComparableEvents with Caller and SinceRFC3339 filters
// ---------------------------------------------------------------------------

// LoadComparableEvents with a Caller filter returns only events from sessions
// attributed to that caller, not from sessions with a different caller.
func TestLoadComparableEvents_CallerFilterIsolatesResults(t *testing.T) {
	db := openTestExpDB(t)

	variantPayload := map[string]any{
		"variants": map[string]any{
			"primary": map[string]any{
				"results": []any{map[string]any{"note_id": "n1", "rank": float64(1)}},
			},
			"shadow": map[string]any{
				"results": []any{map[string]any{"note_id": "n2", "rank": float64(1)}},
			},
		},
	}

	// Session attributed to "cli"
	s1, err := db.StartSessionWithCaller("/vault", "cli", nil)
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID: s1, Type: experiment.EventAsk, VaultPath: "/vault",
		PrimaryVariant: "primary",
		Data:           variantPayload,
	})
	require.NoError(t, err)

	// Session attributed to a different caller
	s2, err := db.StartSessionWithCaller("/vault", "hook", nil)
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID: s2, Type: experiment.EventAsk, VaultPath: "/vault",
		PrimaryVariant: "primary",
		Data:           variantPayload,
	})
	require.NoError(t, err)

	// Filter by Caller="cli" — only the first event should be returned.
	events, err := db.LoadComparableEvents(experiment.ComparableEventFilter{Caller: "cli"})
	require.NoError(t, err)
	assert.Len(t, events, 1, "Caller filter should exclude events from other callers")
}

// LoadComparableEvents with SinceRFC3339 returns only events at or after the cutoff.
func TestLoadComparableEvents_SinceFilterExcludesOlderEvents(t *testing.T) {
	db := openTestExpDB(t)

	variantPayload := map[string]any{
		"variants": map[string]any{
			"primary": map[string]any{
				"results": []any{map[string]any{"note_id": "n1", "rank": float64(1)}},
			},
			"shadow": map[string]any{
				"results": []any{map[string]any{"note_id": "n2", "rank": float64(1)}},
			},
		},
	}

	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	// Insert two events with explicit timestamps at different times.
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	dataJSON, err := json.Marshal(variantPayload)
	require.NoError(t, err)
	for i, ts := range []time.Time{base, base.Add(2 * time.Hour)} {
		_, err = db.Exec(
			`INSERT INTO events (event_id, session_id, event_type, timestamp, vault_path, primary_variant, event_data)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			"evt-"+string(rune('a'+i)), sid, experiment.EventAsk,
			ts.Format(time.RFC3339), "/vault", "primary", string(dataJSON),
		)
		require.NoError(t, err)
	}

	// Only events at or after +1h should be returned.
	cutoff := base.Add(1 * time.Hour).Format(time.RFC3339)
	events, err := db.LoadComparableEvents(experiment.ComparableEventFilter{SinceRFC3339: cutoff})
	require.NoError(t, err)
	assert.Len(t, events, 1, "SinceRFC3339 filter should exclude the earlier event")
}

// LoadComparableEvents with SessionID filter returns only events in that session.
func TestLoadComparableEvents_SessionIDFilterIsolates(t *testing.T) {
	db := openTestExpDB(t)

	variantPayload := map[string]any{
		"variants": map[string]any{
			"primary": map[string]any{
				"results": []any{map[string]any{"note_id": "n1", "rank": float64(1)}},
			},
			"shadow": map[string]any{
				"results": []any{map[string]any{"note_id": "n2", "rank": float64(1)}},
			},
		},
	}

	s1, err := db.StartSession("/vault")
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID: s1, Type: experiment.EventAsk, VaultPath: "/vault",
		PrimaryVariant: "primary", Data: variantPayload,
	})
	require.NoError(t, err)

	s2, err := db.StartSession("/vault")
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID: s2, Type: experiment.EventAsk, VaultPath: "/vault",
		PrimaryVariant: "primary", Data: variantPayload,
	})
	require.NoError(t, err)

	events, err := db.LoadComparableEvents(experiment.ComparableEventFilter{SessionID: s1})
	require.NoError(t, err)
	assert.Len(t, events, 1, "SessionID filter should return only the matching session's events")
}

// ---------------------------------------------------------------------------
// calibration.go — StoreCalibration round-trip (fills the 75% gap)
// ---------------------------------------------------------------------------

func TestStoreCalibration_RoundTrip(t *testing.T) {
	db := openTestExpDB(t)

	snap := &experiment.CalibrationSnapshot{
		CalibrationID:    "cal-001",
		CreatedAt:        "2026-01-01T00:00:00Z",
		EmbedderLabel:    "bge-m3",
		EmbeddingDims:    1024,
		NoteCount:        200,
		NoiseFloor:       0.15,
		NoiseFloorProbes: 50,
		ProbeSetVersion:  1,
		NTNCosineMu:      0.42,
		NTNCosineSigma:   0.08,
		NTNSampleCount:   100,
		VaultPath:        "/Users/me/vault",
	}
	require.NoError(t, db.StoreCalibration(snap))

	got, err := db.LatestCalibrationForVault("/Users/me/vault")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "cal-001", got.CalibrationID)
	assert.InDelta(t, 0.15, got.NoiseFloor, 1e-9)
	assert.Equal(t, 1024, got.EmbeddingDims)
	assert.Equal(t, "/Users/me/vault", got.VaultPath)
}

// LatestCalibrationForVault returns nil when no snapshot exists for a vault.
func TestLatestCalibrationForVault_NilWhenAbsent(t *testing.T) {
	db := openTestExpDB(t)
	got, err := db.LatestCalibrationForVault("/no/vault/here")
	require.NoError(t, err)
	assert.Nil(t, got)
}

// LatestCalibration returns nil on empty DB (fills the nil-return branch).
func TestLatestCalibration_NilOnEmptyDB(t *testing.T) {
	db := openTestExpDB(t)
	got, err := db.LatestCalibration()
	require.NoError(t, err)
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// report.go — Report with empty variants list when events exist (covers the
//             early-return on EventCount==0 — actually the loop body skips)
// ---------------------------------------------------------------------------

// Report with an empty variants slice but existing events returns the counts
// but an empty Variants map — the loop body never executes.
func TestReport_EmptyVariantsList(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID: sid, Type: experiment.EventSearch, VaultPath: "/vault",
		Data: map[string]any{},
	})
	require.NoError(t, err)

	result, err := db.Report([]string{}, 5)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.EventCount)
	assert.Empty(t, result.Variants, "no variants requested — map must be empty")
}

// ---------------------------------------------------------------------------
// usage.go — percentile edge: single-element slice hits both rank<1 and
//            rank>len guards in different branches
// ---------------------------------------------------------------------------

// UsageSummary with exactly two sessions (one gap) exercises percentile with
// a single-element sorted slice — covers the rank==len(sorted) clamp path.
func TestUsageSummary_SingleGapPercentileClamp(t *testing.T) {
	db := openTestExpDB(t)
	base := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	for i, s := range []string{"s1", "s2"} {
		_, err := db.Exec(
			`INSERT INTO sessions (session_id, vault_path, started_at) VALUES (?, ?, ?)`,
			s, "/vault", base.Add(time.Duration(i)*time.Hour).Format(time.RFC3339),
		)
		require.NoError(t, err)
	}

	got, err := db.UsageSummary(0)
	require.NoError(t, err)
	// One gap of 3600 seconds.
	assert.Equal(t, 1, got.GapStats.Count)
	assert.Equal(t, int64(3600), got.GapStats.MedianSeconds)
	assert.Equal(t, int64(3600), got.GapStats.P90Seconds)
	assert.Equal(t, int64(3600), got.GapStats.MaxSeconds)
}

// ---------------------------------------------------------------------------
// VariantPerformance — outcome references variant with no events row
//                      (the defensive "continue" branch inside the outcomes loop)
// ---------------------------------------------------------------------------

// If an outcome row references a variant that has no matching event row
// (data integrity gap), VariantPerformance must skip it rather than panic or
// error. We produce this condition by inserting an outcome row whose variant
// does not exist in the events table's primary_variant.
func TestVariantPerformance_OutcomeWithNoMatchingEventVariantIsSkipped(t *testing.T) {
	db := openTestExpDB(t)

	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	// Log an event with primary_variant "known".
	eventID, err := db.LogEvent(experiment.Event{
		SessionID:      sid,
		Type:           experiment.EventAsk,
		VaultPath:      "/vault",
		PrimaryVariant: "known",
		Data:           map[string]any{},
	})
	require.NoError(t, err)

	// Insert an outcome that references the event but with a variant name
	// ("orphan-variant") that has no corresponding primary_variant event row.
	_, err = db.Exec(
		`INSERT INTO outcomes (outcome_id, event_id, note_id, variant, rank, accessed_at, session_id)
		 VALUES ('oc-orphan', ?, 'n1', 'orphan-variant', 1, ?, ?)`,
		eventID, time.Now().UTC().Format(time.RFC3339), sid,
	)
	require.NoError(t, err)

	stats, err := experiment.VariantPerformance(db)
	require.NoError(t, err)
	// "known" should appear (event exists), "orphan-variant" must not.
	assert.Contains(t, stats, "known")
	assert.NotContains(t, stats, "orphan-variant",
		"orphan-variant outcome must be silently skipped")
	// "known" had no matching outcome → OutcomeCount stays 0 and rates are 0.
	assert.Equal(t, 0, stats["known"].OutcomeCount)
}

// ---------------------------------------------------------------------------
// config.go — ParseExperiments with non-map value skips that key
// ---------------------------------------------------------------------------

// ParseExperiments must silently skip entries that are not maps (e.g. a
// string value under a non-reserved key).
func TestParseExperiments_NonMapValueSkipped(t *testing.T) {
	raw := map[string]any{
		"telemetry":     "anonymous", // reserved — skipped
		"my-experiment": "not-a-map", // non-map — skipped
		"activation":    map[string]any{"enabled": true, "primary": "hybrid"},
	}
	got := experiment.ParseExperiments(raw)
	assert.NotContains(t, got, "telemetry", "reserved keys must be skipped")
	assert.NotContains(t, got, "my-experiment", "non-map values must be skipped")
	require.Contains(t, got, "activation")
	assert.True(t, got["activation"].Enabled)
	assert.Equal(t, "hybrid", got["activation"].Primary)
}

// ---------------------------------------------------------------------------
// scorer.go — Dispatcher.RunAll with unknown variant returns error
// ---------------------------------------------------------------------------

// RunAll must surface the error from Score when an unknown variant is requested,
// exercising the error-return path in RunAll that the existing scorer tests miss.
func TestDispatcher_RunAll_UnknownVariantReturnsError(t *testing.T) {
	d := experiment.NewDispatcher()
	_, err := d.RunAll([]string{"none", "does-not-exist"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
}

// ---------------------------------------------------------------------------
// context.go — outcomeWindow returns s.OutcomeWindow when > 0
// ---------------------------------------------------------------------------

// The > 0 branch of outcomeWindow returns s.OutcomeWindow. Verify indirectly:
// a session with OutcomeWindow=1 only looks back at the current session, so a
// note accessed now links to a current-session event but NOT to one in a prior
// session beyond the window.
func TestSession_OutcomeWindowCustomValueLimitsLookback(t *testing.T) {
	db := openTestExpDB(t)

	// Session 1: retrieval event for note-q.
	s1, err := db.StartSession("/vault")
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID: s1, Type: experiment.EventSearch, VaultPath: "/vault",
		Data: map[string]any{
			"variants": map[string]any{
				"hybrid": map[string]any{
					"results": []any{
						map[string]any{"note_id": "note-q", "rank": float64(1)},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	// Session 2 (current): OutcomeWindow=1 → only look at current session.
	// note-q is NOT in any current-session event, so no outcome is linked.
	s2, err := db.StartSession("/vault")
	require.NoError(t, err)
	session := &experiment.Session{
		DB:            db,
		ID:            s2,
		VaultPath:     "/vault",
		OutcomeWindow: 1, // > 0, so outcomeWindow() returns 1
	}
	_, err = session.LogNoteAccessEvent("note-q", "note_get")
	require.NoError(t, err)

	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM outcomes").Scan(&count))
	assert.Equal(t, 0, count, "window=1 should not look back at prior session's events")
}

// ---------------------------------------------------------------------------
// usage.go — percentile with a P=0 result  (rank < 1 clamp path)
// ---------------------------------------------------------------------------

// UsageSummary with multiple equal gaps exercises percentile near p=0 for
// the P90 computation, which exercises the ceil-rank formula with a larger
// dataset to verify no off-by-one.
func TestUsageSummary_MultipleEqualGapsP90(t *testing.T) {
	db := openTestExpDB(t)
	base := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	// 10 sessions, each 60s apart → 9 gaps of 60s each.
	for i := 0; i < 10; i++ {
		_, err := db.Exec(
			`INSERT INTO sessions (session_id, vault_path, started_at) VALUES (?, ?, ?)`,
			"s"+string(rune('a'+i)), "/vault",
			base.Add(time.Duration(i)*time.Minute).Format(time.RFC3339),
		)
		require.NoError(t, err)
	}

	got, err := db.UsageSummary(0)
	require.NoError(t, err)
	assert.Equal(t, 9, got.GapStats.Count)
	assert.Equal(t, int64(60), got.GapStats.MedianSeconds)
	assert.Equal(t, int64(60), got.GapStats.P90Seconds)
}

// ---------------------------------------------------------------------------
// trace.go — parseEventHits with empty/malformed JSON returns empty slice
// ---------------------------------------------------------------------------

// SessionRetrievals on a session that has no events returns an empty slice.
// This also verifies that parseEventHits returns nil on malformed JSON without
// panicking.
func TestSessionRetrievals_SessionWithMalformedEventData(t *testing.T) {
	db := openTestExpDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)

	// Insert a search event with malformed event_data (not valid JSON).
	_, err = db.Exec(
		`INSERT INTO events (event_id, session_id, event_type, timestamp, vault_path, event_data)
		 VALUES ('evt-bad', ?, 'search', ?, '/vault', 'not-json')`,
		sid, time.Now().UTC().Format(time.RFC3339),
	)
	require.NoError(t, err)

	summaries, err := db.SessionRetrievals(sid)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	// Malformed event_data → parseEventHits returns nil → Hits is empty.
	assert.Empty(t, summaries[0].Hits)
}

// ---------------------------------------------------------------------------
// calibration.go — LatestCalibration returns the most recent across vaults
// ---------------------------------------------------------------------------

// LatestCalibration (vault-agnostic) must return the most recent snapshot when
// multiple vaults have calibration rows — exercises the non-nil return of
// scanCalibration via the vault-agnostic query path.
func TestLatestCalibration_ReturnsNewestAcrossVaults(t *testing.T) {
	db := openTestExpDB(t)

	// Two snapshots for different vaults; the second is newer.
	snaps := []*experiment.CalibrationSnapshot{
		{
			CalibrationID: "cal-old", CreatedAt: "2025-01-01T00:00:00Z",
			EmbedderLabel: "bge-m3", EmbeddingDims: 768, NoteCount: 50,
			NoiseFloor: 0.1, NoiseFloorProbes: 10, ProbeSetVersion: 1,
			NTNCosineMu: 0.3, NTNCosineSigma: 0.05, NTNSampleCount: 20,
			VaultPath: "/vault/a",
		},
		{
			CalibrationID: "cal-new", CreatedAt: "2026-01-01T00:00:00Z",
			EmbedderLabel: "bge-m3", EmbeddingDims: 1024, NoteCount: 200,
			NoiseFloor: 0.15, NoiseFloorProbes: 50, ProbeSetVersion: 1,
			NTNCosineMu: 0.42, NTNCosineSigma: 0.08, NTNSampleCount: 100,
			VaultPath: "/vault/b",
		},
	}
	for _, s := range snaps {
		require.NoError(t, db.StoreCalibration(s))
	}

	got, err := db.LatestCalibration()
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "cal-new", got.CalibrationID,
		"LatestCalibration must return the most-recently-created snapshot")
}

// ---------------------------------------------------------------------------
// report.go — DistinctVariants returns nil slice on empty DB (not an error)
// ---------------------------------------------------------------------------

// DistinctVariants on an empty outcomes table must return a nil/empty slice
// without error — callers use this to populate the variants list for Report.
// The existing test covers non-empty; this is the nil-return branch.
func TestDistinctVariants_NilSliceOnEmptyOutcomes(t *testing.T) {
	db := openTestExpDB(t)
	// Start a session and log an event, but link no outcomes.
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID: sid, Type: experiment.EventSearch, VaultPath: "/vault",
		Data: map[string]any{},
	})
	require.NoError(t, err)

	variants, err := db.DistinctVariants()
	require.NoError(t, err)
	assert.Nil(t, variants, "no outcomes → variants slice must be nil (not an error)")
}

// ---------------------------------------------------------------------------
// session_gaps.go — SessionGaps with multiple sessions verifies scan path
// ---------------------------------------------------------------------------

// SessionGaps returns nil for the first-session's PrevSessionID and a valid
// gap for subsequent sessions — exercises the full scan loop beyond the first
// row, covering the sql.NullString/NullInt64 valid-value branches.
func TestSessionGaps_MultipleSessions(t *testing.T) {
	db := openTestExpDB(t)
	base := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	for i, s := range []string{"g1", "g2", "g3"} {
		_, err := db.Exec(
			`INSERT INTO sessions (session_id, vault_path, started_at) VALUES (?, ?, ?)`,
			s, "/vault", base.Add(time.Duration(i)*time.Hour).Format(time.RFC3339),
		)
		require.NoError(t, err)
	}

	gaps, err := db.SessionGaps()
	require.NoError(t, err)
	require.Len(t, gaps, 3)

	// First session has no predecessor.
	assert.False(t, gaps[0].PrevSessionID.Valid)
	assert.False(t, gaps[0].GapSeconds.Valid)

	// Second and third sessions have valid gaps (3600s each).
	assert.True(t, gaps[1].GapSeconds.Valid)
	assert.Equal(t, int64(3600), gaps[1].GapSeconds.Int64)
	assert.True(t, gaps[2].GapSeconds.Valid)
	assert.Equal(t, int64(3600), gaps[2].GapSeconds.Int64)
}
