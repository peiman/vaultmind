package cmd

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedExperimentDB points xdg data home at a tempdir, opens the experiments
// DB, and returns it for seeding. Callers should db.Close() before running
// the CLI command (cobra reopens it). Returns the tempdir too so tests can
// keep referring to it if needed.
func seedExperimentDB(t *testing.T) (*experiment.DB, string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	dbPath, err := xdg.DataFile("experiments.db")
	require.NoError(t, err)
	db, err := experiment.Open(dbPath)
	require.NoError(t, err)
	return db, tmp
}

// experiment trace with neither --session nor --note must error — otherwise
// a naked `experiment trace` would silently succeed and confuse the user.
func TestExperimentTrace_RequiresSessionOrNote(t *testing.T) {
	_, _ = seedExperimentDB(t)
	_, _, err := runRootCmd(t, "experiment", "trace")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--session")
}

// --session and --note are mutually exclusive; passing both must fail with
// a clear message so the user doesn't get surprised by arbitrary precedence.
func TestExperimentTrace_SessionAndNoteMutuallyExclusive(t *testing.T) {
	_, _ = seedExperimentDB(t)
	_, _, err := runRootCmd(t, "experiment", "trace", "--session", "s", "--note", "n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

// --session <unknown-id> must succeed with an empty events list. Returning
// an error for "no events yet" would break the common "trace a brand-new
// session" flow.
func TestExperimentTrace_UnknownSessionReturnsEmpty(t *testing.T) {
	_, _ = seedExperimentDB(t)
	out, _, err := runRootCmd(t, "experiment", "trace", "--session", "nope", "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			SessionID string `json:"session_id"`
			Events    []any  `json:"events"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "nope", env.Result.SessionID)
	assert.Empty(t, env.Result.Events)
}

// experiment summary on a fresh DB must succeed with zero retrieval events.
// (Every CLI invocation creates its own attribution session via the root
// PersistentPreRunE — that's expected real behavior and TotalSessions
// reflects it.)
func TestExperimentSummary_EmptyDBHasNoRetrievalEvents(t *testing.T) {
	_, _ = seedExperimentDB(t)
	out, _, err := runRootCmd(t, "experiment", "summary", "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Result struct {
			RetrievalEventCount int `json:"RetrievalEventCount"`
			UniqueNotesRecalled int `json:"UniqueNotesRecalled"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, 0, env.Result.RetrievalEventCount, "empty DB must have no retrievals")
	assert.Equal(t, 0, env.Result.UniqueNotesRecalled, "empty DB must recall no notes")
}

// After seeding one retrieval event referencing note n1, the summary must
// reflect that event + note. Regression: a counting bug that drops seeded
// data, or a join that misses note_id.
func TestExperimentSummary_ReflectsSeededRetrievalEvent(t *testing.T) {
	db, _ := seedExperimentDB(t)
	sid, err := db.StartSession("/vault")
	require.NoError(t, err)
	ts := time.Now().UTC().Format(time.RFC3339)
	blob, _ := json.Marshal(map[string]any{
		"variants": map[string]any{
			"hybrid": map[string]any{"results": []any{map[string]any{"note_id": "n1", "rank": 1}}},
		},
	})
	_, err = db.Exec(`INSERT INTO events
		(event_id, session_id, event_type, timestamp, vault_path, query_text, query_mode, primary_variant, event_data)
		VALUES ('ev-1', ?, 'ask', ?, '/vault', 'q', 'hybrid', 'hybrid', ?)`, sid, ts, string(blob))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	out, _, err := runRootCmd(t, "experiment", "summary", "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			RetrievalEventCount int `json:"RetrievalEventCount"`
			UniqueNotesRecalled int `json:"UniqueNotesRecalled"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, 1, env.Result.RetrievalEventCount, "seeded ev-1 must be counted")
	assert.Equal(t, 1, env.Result.UniqueNotesRecalled, "note n1 must be recalled-once")
}

// experiment report with an unknown --experiment name must return a
// structured not_found, not a generic failure. The CLI is called from
// scripts that branch on the error code.
func TestExperimentReport_UnknownExperimentIsNotFound(t *testing.T) {
	_, _ = seedExperimentDB(t)
	out, _, err := runRootCmd(t, "experiment", "report", "--experiment", "does-not-exist", "--json")
	require.NoError(t, err)

	var env struct {
		Status string `json:"status"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors)
	assert.Equal(t, "not_found", env.Errors[0].Code)
}

// experiment report with no data at all produces a human-readable "no data"
// line — not a panic, not a raw SQL error. This is the "first run" path.
func TestExperimentReport_NoDataMessage(t *testing.T) {
	_, _ = seedExperimentDB(t)
	out, _, err := runRootCmd(t, "experiment", "report")
	require.NoError(t, err)
	assert.True(t,
		strings.Contains(out.String(), "No experiment data") ||
			strings.Contains(out.String(), "no experiment data"),
		"empty DB must produce the no-data human message, got: %q", out.String())
}
