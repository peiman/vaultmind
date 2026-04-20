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

func TestFormatCompareResult_Empty(t *testing.T) {
	buf := &bytes.Buffer{}
	if err := formatCompareResult(buf, compareResult{}, 10, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("No comparable events")) {
		t.Fatalf("expected empty-result message, got %q", buf.String())
	}
}

func TestFormatCompareResult_WithRowsAndPerEvent(t *testing.T) {
	r := compareResult{
		Aggregates: []experiment.AggregateRow{
			{PrimaryVariant: "hybrid", ShadowVariant: "activation_v1", EventCount: 2, MeanJaccardAtK: 0.75, MeanKendallTau: 0.5, KendallEventCount: 2},
			{PrimaryVariant: "hybrid", ShadowVariant: "activation_v2", EventCount: 3, MeanJaccardAtK: 1.0, MeanKendallTau: nan(), KendallEventCount: 0},
		},
		PerEvent: []perEventRow{
			{EventID: "ev-1", PrimaryVariant: "hybrid", ShadowVariant: "activation_v1", JaccardAtK: 0.5, KendallTau: 1.0, SharedItems: 3},
			{EventID: "ev-2", PrimaryVariant: "hybrid", ShadowVariant: "activation_v2", JaccardAtK: 1.0, KendallTau: nan(), SharedItems: 1},
		},
	}
	buf := &bytes.Buffer{}
	if err := formatCompareResult(buf, r, 10, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Variant disagreement (K=10)", "activation_v1", "activation_v2", "Per-event:", "ev-1", "ev-2", "nan"} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Fatalf("expected output to contain %q, got %q", want, out)
		}
	}
}

func nan() float64 {
	z := 0.0
	return z / z
}
