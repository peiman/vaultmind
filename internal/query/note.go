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

// RunNoteGet executes the note get logic.
func RunNoteGet(db *index.DB, cfg NoteGetConfig, w io.Writer) error {
	resolver := graph.NewResolver(db)
	resolved, err := resolver.Resolve(cfg.Input)
	if err != nil {
		return fmt.Errorf("resolving: %w", err)
	}

	if !resolved.Resolved {
		if cfg.JSONOutput {
			return json.NewEncoder(w).Encode(envelope.Error("note get", "not_found",
				fmt.Sprintf("no note matches %q", cfg.Input), ""))
		}
		_, err = fmt.Fprintf(w, "No note found for %q\n", cfg.Input)
		return err
	}

	if resolved.Ambiguous {
		if cfg.JSONOutput {
			env := envelope.Error("note get", "ambiguous_resolution", "multiple matches", "")
			env.Errors[0].Candidates = make([]string, len(resolved.Matches))
			for i, m := range resolved.Matches {
				env.Errors[0].Candidates[i] = m.ID
			}
			env.Result = resolved
			return json.NewEncoder(w).Encode(env)
		}
		return fmt.Errorf("ambiguous: %d matches", len(resolved.Matches))
	}

	note, err := db.QueryFullNote(resolved.Matches[0].ID)
	if err != nil {
		return fmt.Errorf("querying note: %w", err)
	}
	if note == nil {
		return fmt.Errorf("note %q not found in index", resolved.Matches[0].ID)
	}

	// Plasticity roadmap step 5 (Track A.2): explicit `note get <id>` is
	// the highest-signal retrieval-access event vaultmind emits — an
	// agent or user named this note by id and got back its body. Record
	// before rendering so the increment is observable to any downstream
	// reader of access_count. Best-effort: a tracking miss is logged at
	// debug and never fails the user query. CallerAgent because direct
	// id-naming is the most deliberate retrieval signal we have.
	if recErr := index.RecordNoteAccessAs(db, note.ID, index.CallerAgent); recErr != nil {
		log.Debug().Err(recErr).Str("note_id", note.ID).Msg("recording note-get access failed (non-fatal)")
	}

	if cfg.FrontmatterOnly {
		note.Body = ""
		note.Headings = nil
		note.Blocks = nil
	}

	if cfg.JSONOutput {
		env := envelope.OK("note get", note)
		env.Meta.VaultPath = cfg.VaultPath
		return json.NewEncoder(w).Encode(env)
	}

	if _, err = fmt.Fprintf(w, "%s (%s) — %s\n", note.ID, note.Type, note.Title); err != nil {
		return err
	}
	// Render the body in human mode unless the caller asked for
	// frontmatter-only. Pre-2026-04-30 human mode returned only the
	// header; agents fell back to the Read tool for bodies, which
	// silently bypassed the access tracker. Printing the body here
	// makes `note get` both the cleanest and the tracked read path.
	if !cfg.FrontmatterOnly && note.Body != "" {
		if _, err = fmt.Fprintf(w, "\n%s\n", note.Body); err != nil {
			return err
		}
	}
	return nil
}
