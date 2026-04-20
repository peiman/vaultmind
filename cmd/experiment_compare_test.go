package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/xdg"
)

func TestExperimentCompare_JSONOutput(t *testing.T) {
	tmp := t.TempDir()
	// Set both HOME and XDG_DATA_HOME so xdg.DataFile points into tmp on
	// macOS (uses HOME) and Linux (uses XDG_DATA_HOME).
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)

	dbPath, err := xdg.DataFile("experiments.db")
	if err != nil {
		t.Fatalf("xdg.DataFile: %v", err)
	}
	db, err := experiment.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sid, err := db.StartSession("/tmp/v")
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	blob, _ := json.Marshal(map[string]any{
		"variants": map[string]any{
			"hybrid":        map[string]any{"results": []any{map[string]any{"note_id": "n1", "rank": 1}, map[string]any{"note_id": "n2", "rank": 2}}},
			"activation_v1": map[string]any{"results": []any{map[string]any{"note_id": "n2", "rank": 1}, map[string]any{"note_id": "n1", "rank": 2}}},
		},
	})
	if _, err := db.Exec(`INSERT INTO events
		(event_id, session_id, event_type, timestamp, vault_path, query_text, query_mode, primary_variant, event_data)
		VALUES ('ev-1', ?, 'ask', ?, '/tmp/v', 'q', 'hybrid', 'hybrid', ?)`, sid, ts, string(blob)); err != nil {
		t.Fatalf("insert: %v", err)
	}
	_ = db.Close()

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	RootCmd.SetOut(out)
	RootCmd.SetErr(errOut)
	RootCmd.SetArgs([]string{"experiment", "compare", "--json", "--k", "5"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v (stderr=%s)", err, errOut.String())
	}

	var env struct {
		Status string `json:"status"`
		Result struct {
			Aggregates []experiment.AggregateRow `json:"aggregates"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v raw=%s", err, out.String())
	}
	if env.Status != "ok" {
		t.Fatalf("expected status ok, got %s", env.Status)
	}
	if len(env.Result.Aggregates) != 1 {
		t.Fatalf("expected 1 aggregate row, got %d", len(env.Result.Aggregates))
	}
	row := env.Result.Aggregates[0]
	if row.ShadowVariant != "activation_v1" || row.PrimaryVariant != "hybrid" {
		t.Fatalf("unexpected labels: %+v", row)
	}
	if row.EventCount != 1 {
		t.Fatalf("expected EventCount=1, got %d", row.EventCount)
	}
}
