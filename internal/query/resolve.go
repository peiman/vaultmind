package query

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
)

// RunResolve executes the resolve command logic.
func RunResolve(db *index.DB, input, vaultPath string, jsonOut bool, w io.Writer) error {
	resolver := graph.NewResolver(db)
	result, err := resolver.Resolve(input)
	if err != nil {
		return fmt.Errorf("resolving: %w", err)
	}

	if jsonOut {
		env := envelope.OK("resolve", result)
		if result.Ambiguous {
			env.Status = "error"
			env.Errors = append(env.Errors, envelope.Issue{
				Code: "ambiguous_resolution", Message: "multiple matches",
			})
		}
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(w).Encode(env)
	}

	if !result.Resolved {
		_, err = fmt.Fprintf(w, "No match for %q\n", input)
		return err
	}
	for _, m := range result.Matches {
		if _, err := fmt.Fprintf(w, "%s  %s  %s  (%s)\n", m.ID, m.Type, m.Title, m.Path); err != nil {
			return err
		}
	}
	return nil
}
