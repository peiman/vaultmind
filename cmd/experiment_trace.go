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

func formatSessionTrace(w io.Writer, t sessionTrace) error {
	who := callerLine(t.Caller, t.Meta)
	if _, err := fmt.Fprintf(w, "Session %s\n  %s\n  events: %d\n\n", t.SessionID, who, len(t.Events)); err != nil {
		return err
	}
	for _, e := range t.Events {
		query := e.Query
		if query == "" {
			query = "(no query text)"
		}
		if _, err := fmt.Fprintf(w, "  %s  %-12s  %q  → %d hits\n", e.Timestamp, e.EventType, query, len(e.Hits)); err != nil {
			return err
		}
		for _, h := range e.Hits {
			if _, err := fmt.Fprintf(w, "        rank %d  %s\n", h.Rank, h.NoteID); err != nil {
				return err
			}
		}
	}
	return nil
}

func formatNoteTrace(w io.Writer, t noteTrace) error {
	if _, err := fmt.Fprintf(w, "Note %s — %d retrievals\n\n", t.NoteID, len(t.Hits)); err != nil {
		return err
	}
	for _, h := range t.Hits {
		who := callerLine(h.Caller, h.Meta)
		if _, err := fmt.Fprintf(w, "  %s  rank %d  %-13s  session %s\n    %s\n",
			h.Timestamp, h.Rank, h.EventType, h.SessionID, who); err != nil {
			return err
		}
	}
	return nil
}

// callerLine renders the caller + operator as a single compact attribution
// string: "caller=workhorse-persona-hook  operator=peiman@MaxDaddy.local".
// Empty fields are omitted so unknown-caller rows don't show dangling "=".
func callerLine(caller string, meta map[string]any) string {
	parts := make([]string, 0, 3)
	if caller != "" {
		parts = append(parts, fmt.Sprintf("caller=%s", caller))
	} else {
		parts = append(parts, "caller=unknown")
	}
	user, _ := meta["user"].(string)
	host, _ := meta["host"].(string)
	if user != "" || host != "" {
		op := user
		if host != "" {
			if op != "" {
				op += "@" + host
			} else {
				op = host
			}
		}
		parts = append(parts, fmt.Sprintf("operator=%s", op))
	}
	if pd, _ := meta["claude_project_dir"].(string); pd != "" {
		parts = append(parts, fmt.Sprintf("project=%s", pd))
	}
	return joinSpaced(parts)
}

// joinSpaced joins strings with two spaces for compact terminal output.
func joinSpaced(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "  "
		}
		out += p
	}
	return out
}
