package experiment_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SanitizeEventData under Anonymous tier must strip note_id and path
// from variants[*].results[*]. The contract is documented in
// telemetry.go and is the privacy promise we make to early adopters
// who pick the Anonymous tier — sending anything more than aggregate
// shape (ranks, scores, counts) would break that promise.
func TestSanitizeEventData_AnonymousStripsResultIdsAndPaths(t *testing.T) {
	raw := map[string]any{
		"variants": map[string]any{
			"primary": map[string]any{
				"results": []any{
					map[string]any{
						"note_id": "concept-act-r",
						"path":    "/Users/me/vault/concepts/act-r.md",
						"rank":    float64(1),
						"score":   0.87,
					},
					map[string]any{
						"note_id": "concept-spreading-activation",
						"path":    "concepts/spreading-activation.md",
						"rank":    float64(2),
						"score":   0.42,
					},
				},
			},
		},
	}

	got := experiment.SanitizeEventData(raw, experiment.TelemetryAnonymous)

	variants, ok := got["variants"].(map[string]any)
	require.True(t, ok)
	primary, ok := variants["primary"].(map[string]any)
	require.True(t, ok)
	results, ok := primary["results"].([]any)
	require.True(t, ok)
	require.Len(t, results, 2)

	for i, r := range results {
		row := r.(map[string]any)
		_, hasID := row["note_id"]
		_, hasPath := row["path"]
		assert.False(t, hasID, "result[%d] note_id must be stripped under Anonymous", i)
		assert.False(t, hasPath, "result[%d] path must be stripped under Anonymous", i)
		// Aggregate fields preserved
		assert.Contains(t, row, "rank", "result[%d] rank must survive", i)
		assert.Contains(t, row, "score", "result[%d] score must survive", i)
	}
}

// Full tier preserves everything — that's the point of the tier.
func TestSanitizeEventData_FullPreservesEverything(t *testing.T) {
	raw := map[string]any{
		"variants": map[string]any{
			"primary": map[string]any{
				"results": []any{
					map[string]any{
						"note_id": "concept-act-r",
						"path":    "/some/path.md",
						"rank":    float64(1),
					},
				},
			},
		},
	}

	got := experiment.SanitizeEventData(raw, experiment.TelemetryFull)

	variants := got["variants"].(map[string]any)
	primary := variants["primary"].(map[string]any)
	results := primary["results"].([]any)
	row := results[0].(map[string]any)
	assert.Equal(t, "concept-act-r", row["note_id"])
	assert.Equal(t, "/some/path.md", row["path"])
}

// Sanitize must not mutate the input map — callers may still need it.
func TestSanitizeEventData_DoesNotMutateInput(t *testing.T) {
	raw := map[string]any{
		"variants": map[string]any{
			"primary": map[string]any{
				"results": []any{
					map[string]any{"note_id": "abc", "rank": float64(1)},
				},
			},
		},
	}

	_ = experiment.SanitizeEventData(raw, experiment.TelemetryAnonymous)

	// Input retains note_id
	v := raw["variants"].(map[string]any)
	p := v["primary"].(map[string]any)
	r := p["results"].([]any)
	assert.Equal(t, "abc", r[0].(map[string]any)["note_id"], "input must not be mutated")
}

// ExportToJSONL emits one record per line: a manifest record followed
// by sessions, events, outcomes. Each line is a JSON object with a
// "kind" discriminator. Anonymous tier scrubs vault_path on sessions
// and events.
func TestExportToJSONL_WritesManifestAndRecords(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exp.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// Seed: one session with one ask event.
	sid, err := db.StartSession("/Users/me/vault")
	require.NoError(t, err)

	eventData := map[string]any{
		"variants": map[string]any{
			"primary": map[string]any{
				"results": []any{
					map[string]any{"note_id": "concept-act-r", "rank": float64(1), "score": 0.9},
				},
			},
		},
	}
	_, err = db.LogEvent(experiment.Event{
		SessionID:      sid,
		Type:           experiment.EventAsk,
		VaultPath:      "/Users/me/vault",
		QueryText:      "spreading activation",
		PrimaryVariant: "primary",
		Data:           eventData,
	})
	require.NoError(t, err)
	require.NoError(t, db.EndSession(sid))

	var buf bytes.Buffer
	require.NoError(t, experiment.ExportToJSONL(db, experiment.TelemetryAnonymous, &buf))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.GreaterOrEqual(t, len(lines), 3, "want manifest + at least 1 session + 1 event")

	var manifest map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &manifest))
	assert.Equal(t, "manifest", manifest["kind"])
	assert.Equal(t, experiment.TelemetryAnonymous, manifest["tier"])
	assert.Contains(t, manifest, "exported_at")
	assert.Contains(t, manifest, "session_count")
	assert.Contains(t, manifest, "event_count")

	// Find the session record and verify vault_path was scrubbed.
	var sawSession, sawEvent bool
	for _, ln := range lines[1:] {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(ln), &rec))
		switch rec["kind"] {
		case "session":
			sawSession = true
			_, hasVaultPath := rec["vault_path"]
			assert.False(t, hasVaultPath, "Anonymous: session.vault_path must be stripped")
		case "event":
			sawEvent = true
			_, hasVaultPath := rec["vault_path"]
			assert.False(t, hasVaultPath, "Anonymous: event.vault_path must be stripped")
			_, hasQueryText := rec["query_text"]
			assert.False(t, hasQueryText, "Anonymous: event.query_text must be stripped")
			data, ok := rec["data"].(map[string]any)
			require.True(t, ok, "event.data must be present")
			variants := data["variants"].(map[string]any)
			primary := variants["primary"].(map[string]any)
			results := primary["results"].([]any)
			row := results[0].(map[string]any)
			_, hasNoteID := row["note_id"]
			assert.False(t, hasNoteID, "Anonymous: event.data.results[].note_id must be stripped")
		}
	}
	assert.True(t, sawSession, "session record must be emitted")
	assert.True(t, sawEvent, "event record must be emitted")
}

// Full tier preserves vault_path, query_text, and result note_ids.
func TestExportToJSONL_FullPreservesPII(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exp.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	sid, err := db.StartSession("/Users/me/vault")
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID: sid,
		Type:      experiment.EventAsk,
		VaultPath: "/Users/me/vault",
		QueryText: "spreading activation",
		Data: map[string]any{
			"variants": map[string]any{
				"primary": map[string]any{
					"results": []any{
						map[string]any{"note_id": "concept-act-r", "rank": float64(1)},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, experiment.ExportToJSONL(db, experiment.TelemetryFull, &buf))

	out := buf.String()
	assert.Contains(t, out, "/Users/me/vault", "Full: vault_path must be present")
	assert.Contains(t, out, "spreading activation", "Full: query_text must be present")
	assert.Contains(t, out, "concept-act-r", "Full: result note_id must be present")
}

// Off tier: ExportToJSONL must refuse — the user opted out, and
// producing a file would imply we collected data we shouldn't have.
func TestExportToJSONL_OffTierRefuses(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exp.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	var buf bytes.Buffer
	err = experiment.ExportToJSONL(db, experiment.TelemetryOff, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "off")
	assert.Empty(t, buf.String(), "no output should be written when refusing")
}

// Outcomes are linked retrospectively when a note is accessed. The export
// must surface them as their own record kind, with note_id stripped under
// Anonymous and preserved under Full.
func TestExportToJSONL_OutcomeRecord(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exp.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	sid, err := db.StartSession("/some/vault")
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID:      sid,
		Type:           experiment.EventAsk,
		VaultPath:      "/some/vault",
		PrimaryVariant: "primary",
		Data: map[string]any{
			"variants": map[string]any{
				"primary": map[string]any{
					"results": []any{
						map[string]any{"note_id": "concept-act-r", "rank": float64(1)},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	count, err := db.LinkOutcomes(sid, "concept-act-r", 1)
	require.NoError(t, err)
	require.Equal(t, 1, count, "one outcome should be linked for the matching note_id")

	// Anonymous: outcome is exported but note_id is stripped.
	var anonBuf bytes.Buffer
	require.NoError(t, experiment.ExportToJSONL(db, experiment.TelemetryAnonymous, &anonBuf))
	anonOut := anonBuf.String()
	assert.Contains(t, anonOut, `"kind":"outcome"`)
	assert.NotContains(t, anonOut, "concept-act-r", "Anonymous: outcome.note_id must be stripped")

	// Full: outcome carries note_id.
	var fullBuf bytes.Buffer
	require.NoError(t, experiment.ExportToJSONL(db, experiment.TelemetryFull, &fullBuf))
	assert.Contains(t, fullBuf.String(), "concept-act-r", "Full: outcome.note_id must be present")
}

// SanitizeEventData under Anonymous must pass through non-map values for
// the variants key without crashing. Real event_data may have shape
// drift, and the sanitizer's job is to strip what it knows; not panic on
// the rest.
func TestSanitizeEventData_AnonymousPreservesNonMapVariants(t *testing.T) {
	raw := map[string]any{
		"variants":   "unexpected-string-instead-of-map",
		"other_key":  42,
		"timestamps": []any{"2026-05-01T00:00:00Z"},
	}
	got := experiment.SanitizeEventData(raw, experiment.TelemetryAnonymous)
	assert.Equal(t, "unexpected-string-instead-of-map", got["variants"])
	assert.EqualValues(t, 42, got["other_key"])
}

// sanitizeVariants must pass through non-map variant values. Same shape-
// drift safety as the parent test, one level deeper.
func TestSanitizeEventData_AnonymousPreservesNonMapVariantBody(t *testing.T) {
	raw := map[string]any{
		"variants": map[string]any{
			"primary": "not-a-map", // shape drift — sanitizer must not panic
			"shadow1": map[string]any{
				"results": "also-not-a-list", // shape drift inside variant
			},
		},
	}
	got := experiment.SanitizeEventData(raw, experiment.TelemetryAnonymous)
	v := got["variants"].(map[string]any)
	assert.Equal(t, "not-a-map", v["primary"])
	shadow := v["shadow1"].(map[string]any)
	assert.Equal(t, "also-not-a-list", shadow["results"])
}

// sanitizeResults must pass through non-map result rows.
func TestSanitizeEventData_AnonymousPreservesNonMapResultRow(t *testing.T) {
	raw := map[string]any{
		"variants": map[string]any{
			"primary": map[string]any{
				"results": []any{
					"not-a-map",
					map[string]any{"note_id": "x", "rank": float64(1)},
				},
			},
		},
	}
	got := experiment.SanitizeEventData(raw, experiment.TelemetryAnonymous)
	v := got["variants"].(map[string]any)
	p := v["primary"].(map[string]any)
	r := p["results"].([]any)
	assert.Equal(t, "not-a-map", r[0])
	row := r[1].(map[string]any)
	_, hasID := row["note_id"]
	assert.False(t, hasID)
}

// failingWriter returns the same error on every Write — used to exercise
// the encode-error branches in ExportToJSONL.
type failingWriter struct{ err error }

func (f failingWriter) Write(_ []byte) (int, error) { return 0, f.err }

// When the underlying writer fails, ExportToJSONL surfaces the error
// rather than silently producing a partial file. Manifest is the first
// thing written, so a failing writer trips that branch.
func TestExportToJSONL_WriterErrorSurfacedOnManifest(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exp.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	w := failingWriter{err: assertedWriteError}
	err = experiment.ExportToJSONL(db, experiment.TelemetryAnonymous, w)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write manifest")
}

// boundedFailingWriter accepts the first N writes, then fails. This lets
// us cover the encode-session / encode-event / encode-outcome branches
// that come AFTER the manifest writes successfully.
type boundedFailingWriter struct {
	allowed int
	written int
}

func (b *boundedFailingWriter) Write(p []byte) (int, error) {
	b.written++
	if b.written > b.allowed {
		return 0, assertedWriteError
	}
	return len(p), nil
}

// After the manifest succeeds, a failing writer trips on the session row.
func TestExportToJSONL_WriterErrorSurfacedOnSession(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exp.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.StartSession("/v")
	require.NoError(t, err)

	w := &boundedFailingWriter{allowed: 1}
	err = experiment.ExportToJSONL(db, experiment.TelemetryAnonymous, w)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write session")
}

// After session writes succeed, a failing writer trips on the event row.
func TestExportToJSONL_WriterErrorSurfacedOnEvent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exp.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	sid, err := db.StartSession("/v")
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{SessionID: sid, Type: experiment.EventAsk, VaultPath: "/v", Data: map[string]any{}})
	require.NoError(t, err)

	// Allow manifest + session, fail on event.
	w := &boundedFailingWriter{allowed: 2}
	err = experiment.ExportToJSONL(db, experiment.TelemetryAnonymous, w)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write event")
}

// After session+event writes succeed, a failing writer trips on outcome.
func TestExportToJSONL_WriterErrorSurfacedOnOutcome(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exp.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	sid, err := db.StartSession("/v")
	require.NoError(t, err)
	_, err = db.LogEvent(experiment.Event{
		SessionID: sid, Type: experiment.EventAsk, VaultPath: "/v",
		Data: map[string]any{
			"variants": map[string]any{
				"primary": map[string]any{
					"results": []any{map[string]any{"note_id": "n1", "rank": float64(1)}},
				},
			},
		},
	})
	require.NoError(t, err)
	_, err = db.LinkOutcomes(sid, "n1", 1)
	require.NoError(t, err)

	// Allow manifest + session + event, fail on outcome.
	w := &boundedFailingWriter{allowed: 3}
	err = experiment.ExportToJSONL(db, experiment.TelemetryAnonymous, w)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write outcome")
}

var assertedWriteError = fmt.Errorf("simulated write failure")

// Unknown tier returns an error (not silently anonymous).
func TestExportToJSONL_UnknownTierRefuses(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "exp.db")
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	var buf bytes.Buffer
	err = experiment.ExportToJSONL(db, "garbage", &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tier")
	assert.Empty(t, buf.String())
}
