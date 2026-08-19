package query

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/rs/zerolog/log"
)

// NoteGetConfig holds parameters for the note get operation.
type NoteGetConfig struct {
	Input           string
	FrontmatterOnly bool
	JSONOutput      bool
	VaultPath       string
}

// NoteGetOutcome reports what a note get actually did, so the caller can log it
// instead of guessing.
//
// The caller used to log args[0] — the RAW INPUT — because RunNoteGet resolved
// internally and returned only an error. A title- or path-resolved read
// therefore landed on a key no lookup can match, and `note get` is the
// highest-signal event the tool emits. It also hardcoded "a body was
// delivered", which --frontmatter-only and every miss falsified.
type NoteGetOutcome struct {
	// NoteID is the RESOLVED id, empty when the input did not resolve.
	NoteID string
	// BodyDelivered is whether note text was actually rendered — false for
	// --frontmatter-only, for misses, and for ambiguous input.
	BodyDelivered bool
}

// RunNoteGet executes the note get logic and reports what it delivered.
func RunNoteGet(db *index.DB, cfg NoteGetConfig, w io.Writer) (NoteGetOutcome, error) {
	resolver := graph.NewResolver(db)
	resolved, err := resolver.Resolve(cfg.Input)
	if err != nil {
		return NoteGetOutcome{}, fmt.Errorf("resolving: %w", err)
	}

	if !resolved.Resolved {
		if cfg.JSONOutput {
			return NoteGetOutcome{}, envelope.WriteError(w, envelope.Error("note get", "not_found",
				fmt.Sprintf("no note matches %q", cfg.Input), ""))
		}
		// Text mode fails too. A missing id is a failure in both modes, and text
		// is the half that looked like a judgement call while still being the
		// wrong success: `vaultmind note get "$id" || fallback` silently took the
		// found path on every typo. The friendly line is still printed; the
		// sentinel sets the exit status without describing the failure twice.
		if _, ferr := fmt.Fprintf(w, "No note found for %q\n", cfg.Input); ferr != nil {
			return NoteGetOutcome{}, ferr
		}
		return NoteGetOutcome{}, envelope.ErrAlreadyWritten
	}

	if resolved.Ambiguous {
		if cfg.JSONOutput {
			env := envelope.Error("note get", "ambiguous_resolution", "multiple matches", "")
			env.Errors[0].Candidates = make([]string, len(resolved.Matches))
			for i, m := range resolved.Matches {
				env.Errors[0].Candidates[i] = m.ID
			}
			env.Result = resolved
			return NoteGetOutcome{}, envelope.WriteError(w, env)
		}
		return NoteGetOutcome{}, fmt.Errorf("ambiguous: %d matches", len(resolved.Matches))
	}

	note, err := db.QueryFullNote(resolved.Matches[0].ID)
	if err != nil {
		return NoteGetOutcome{}, fmt.Errorf("querying note: %w", err)
	}
	if note == nil {
		return NoteGetOutcome{}, fmt.Errorf("note %q not found in index", resolved.Matches[0].ID)
	}

	// Plasticity roadmap step 5 (Track A.2): explicit `note get <id>` is
	// the highest-signal retrieval-access event vaultmind emits — an
	// agent or user named this note by id and got back its body. Record
	// before rendering so the increment is observable to any downstream
	// reader of access_count. Best-effort: a tracking miss is logged at
	// debug and never fails the user query. CallerAgent because direct
	// id-naming is the most deliberate retrieval signal we have.
	// Strip BEFORE deciding delivery. --frontmatter-only clears the body a few
	// lines down, and recording the access above that meant a flag on this very
	// command falsified the "a body was delivered" it recorded.
	if cfg.FrontmatterOnly {
		note.Body = ""
		note.Headings = nil
		note.Blocks = nil
	}
	outcome := NoteGetOutcome{NoteID: note.ID, BodyDelivered: note.Body != ""}

	if recErr := index.RecordNoteAccessDelivered(db, note.ID, index.CallerAgent, outcome.BodyDelivered); recErr != nil {
		log.Debug().Err(recErr).Str("note_id", note.ID).Msg("recording note-get access failed (non-fatal)")
	}

	if cfg.JSONOutput {
		env := envelope.OK("note get", note)
		env.Meta.VaultPath = cfg.VaultPath
		return outcome, json.NewEncoder(w).Encode(env)
	}

	if _, err = fmt.Fprintf(w, "%s (%s) — %s\n", note.ID, note.Type, note.Title); err != nil {
		return outcome, err
	}
	// Render the body in human mode unless the caller asked for
	// frontmatter-only. Pre-2026-04-30 human mode returned only the
	// header; agents fell back to the Read tool for bodies, which
	// silently bypassed the access tracker. Printing the body here
	// makes `note get` both the cleanest and the tracked read path.
	if note.Body != "" {
		if _, err = fmt.Fprintf(w, "\n%s\n", note.Body); err != nil {
			return outcome, err
		}
	}
	return outcome, nil
}
