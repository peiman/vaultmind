package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/spf13/cobra"
)

var experimentTraceCmd = MustNewCommand(commands.ExperimentTraceMetadata, runExperimentTrace)

func init() {
	experimentCmd.AddCommand(experimentTraceCmd)
	setupCommandConfig(experimentTraceCmd)
}

func runExperimentTrace(cmd *cobra.Command, _ []string) error {
	sessionID := getConfigValueWithFlags[string](cmd, "session", config.KeyAppExperimenttraceSession)
	noteID := getConfigValueWithFlags[string](cmd, "note", config.KeyAppExperimenttraceNote)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppExperimenttraceJson)

	if sessionID == "" && noteID == "" {
		return errors.New("specify --session <id> or --note <id>")
	}
	if sessionID != "" && noteID != "" {
		return errors.New("--session and --note are mutually exclusive")
	}

	dbPath, err := xdg.DataFile("experiments.db")
	if err != nil {
		return fmt.Errorf("resolving experiment db path: %w", err)
	}
	expDB, err := experiment.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening experiment db: %w", err)
	}
	defer func() { _ = expDB.Close() }()

	if sessionID != "" {
		return traceSession(cmd.OutOrStdout(), expDB, sessionID, jsonOut)
	}
	return traceNote(cmd.OutOrStdout(), expDB, noteID, jsonOut)
}

// sessionTrace is the JSON shape for --session output.
type sessionTrace struct {
	SessionID string                             `json:"session_id"`
	Caller    string                             `json:"caller,omitempty"`
	Meta      map[string]any                     `json:"meta,omitempty"`
	Events    []experiment.RetrievalEventSummary `json:"events"`
}

// noteTrace is the JSON shape for --note output.
type noteTrace struct {
	NoteID string         `json:"note_id"`
	Hits   []noteTraceHit `json:"hits"`
}

type noteTraceHit struct {
	experiment.SessionHit
	Caller string         `json:"caller,omitempty"`
	Meta   map[string]any `json:"meta,omitempty"`
}

func traceSession(w io.Writer, db *experiment.DB, sessionID string, jsonOut bool) error {
	caller, err := db.GetSessionCaller(sessionID)
	if err != nil {
		return fmt.Errorf("fetching session caller: %w", err)
	}
	events, err := db.SessionRetrievals(sessionID)
	if err != nil {
		return fmt.Errorf("fetching session retrievals: %w", err)
	}

	out := sessionTrace{SessionID: sessionID, Caller: caller.Caller, Meta: caller.Meta, Events: events}
	if jsonOut {
		return json.NewEncoder(w).Encode(envelope.OK("experiment trace", out))
	}
	return formatSessionTrace(w, out)
}

func traceNote(w io.Writer, db *experiment.DB, noteID string, jsonOut bool) error {
	hits, err := db.NoteRetrievals(noteID)
	if err != nil {
		return fmt.Errorf("fetching note retrievals: %w", err)
	}

	// Enrich each hit with the session's caller attribution. Small N in
	// practice; loop lookup is fine here.
	enriched := make([]noteTraceHit, len(hits))
	for i, h := range hits {
		c, cerr := db.GetSessionCaller(h.SessionID)
		if cerr == nil {
			enriched[i] = noteTraceHit{SessionHit: h, Caller: c.Caller, Meta: c.Meta}
		} else {
			enriched[i] = noteTraceHit{SessionHit: h}
		}
	}

	out := noteTrace{NoteID: noteID, Hits: enriched}
	if jsonOut {
		return json.NewEncoder(w).Encode(envelope.OK("experiment trace", out))
	}
	return formatNoteTrace(w, out)
}

// formatSessionTrace, formatNoteTrace, callerLine, joinSpaced moved to
// cmd/experiment_format.go (consolidated with summary + compare formatters).
