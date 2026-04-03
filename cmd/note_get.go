package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
)

var noteGetCmd = MustNewCommand(commands.NoteGetMetadata, runNoteGet)

func init() {
	noteCmd.AddCommand(noteGetCmd)
	setupCommandConfig(noteGetCmd)
}

func runNoteGet(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind note get <id-or-path>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppNoteVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppNoteJson)
	fmOnly := getConfigValueWithFlags[bool](cmd, "frontmatter-only", config.KeyAppNoteFrontmatterOnly)

	cfg, err := vault.LoadConfig(vaultPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	dbPath := filepath.Join(vaultPath, cfg.Index.DBPath)
	db, err := index.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening index: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Resolve the input to a note ID
	resolver := graph.NewResolver(db)
	resolved, err := resolver.Resolve(args[0])
	if err != nil {
		return fmt.Errorf("resolving: %w", err)
	}
	if !resolved.Resolved {
		if jsonOut {
			env := envelope.Error("note get", "not_found", fmt.Sprintf("no note matches %q", args[0]), "")
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "No note found for %q\n", args[0])
		return err
	}
	if resolved.Ambiguous {
		if jsonOut {
			env := envelope.Error("note get", "ambiguous_resolution", "multiple matches", "")
			env.Errors[0].Candidates = make([]string, len(resolved.Matches))
			for i, m := range resolved.Matches {
				env.Errors[0].Candidates[i] = m.ID
			}
			env.Result = resolved
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		}
		return fmt.Errorf("ambiguous: %d matches", len(resolved.Matches))
	}

	noteID := resolved.Matches[0].ID
	note, err := db.QueryFullNote(noteID)
	if err != nil {
		return fmt.Errorf("querying note: %w", err)
	}
	if note == nil {
		return fmt.Errorf("note %q not found in index", noteID)
	}

	if fmOnly {
		note.Body = ""
		note.Headings = nil
		note.Blocks = nil
	}

	if jsonOut {
		env := envelope.OK("note get", note)
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s (%s) — %s\n", note.ID, note.Type, note.Title)
	return err
}
