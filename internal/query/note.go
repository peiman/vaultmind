package query

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
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

	_, err = fmt.Fprintf(w, "%s (%s) — %s\n", note.ID, note.Type, note.Title)
	return err
}
