package query

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
)

// Citation trust — is a note's stated evidence allowed to BE evidence?
//
// `source_ids` is documented as citations, and validation only ever checked
// that the cited note EXISTS. So a curated arc could cite an unreviewed
// scratch journal as its source and `doctor` reported the vault healthy: the
// citation looked rigorous and carried no weight (issue #137).
//
// Existence and authority are different questions. This asks the second one,
// against what each type DECLARES about itself — never inferred from naming or
// folder, because a guess about trust is worse than no answer.

// WarnNonAuthoritativeCitation is emitted when evidence is not allowed to be
// evidence. It names both notes, because "you have 3 bad citations" is not
// something anyone can act on.
const WarnNonAuthoritativeCitationFmt = "%s cites %s as evidence, but type %q is declared non-authoritative — " +
	"raw material, not a source. Distil it into a reviewed note and cite that."

// BadCitation is one source_ids edge pointing at a type that may not be cited.
type BadCitation struct {
	NoteID     string `json:"note_id"`
	SourceID   string `json:"source_id"`
	SourceType string `json:"source_type"`
}

// FindNonAuthoritativeCitations returns every source_ids edge whose target is a
// type the vault declares non-authoritative.
//
// Returns an empty slice when no type is restricted, so a vault that has not
// opted in does no work and sees no warnings.
func FindNonAuthoritativeCitations(db *index.DB, cfg *vault.Config) ([]BadCitation, error) {
	if cfg == nil || len(cfg.Types) == 0 {
		return nil, nil
	}
	restricted := make(map[string]bool)
	for name := range cfg.Types {
		if !cfg.IsAuthoritative(name) {
			restricted[name] = true
		}
	}
	if len(restricted) == 0 {
		return nil, nil
	}

	// Only frontmatter.source_ids: related_ids and parent_id are associations,
	// not claims of evidence, and share the same edge type. Filtering on origin
	// is what keeps this check about citation rather than about linking.
	rows, err := db.Query(`
		SELECT l.src_note_id, n.id, n.type
		FROM links l
		JOIN notes n ON n.id = l.dst_note_id
		WHERE l.origin = 'frontmatter.source_ids'
		  AND l.resolved = TRUE
		  AND l.dst_note_id IS NOT NULL
		ORDER BY l.src_note_id, n.id`)
	if err != nil {
		return nil, fmt.Errorf("citation check: querying source_ids edges: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []BadCitation
	for rows.Next() {
		var c BadCitation
		if err := rows.Scan(&c.NoteID, &c.SourceID, &c.SourceType); err != nil {
			return nil, fmt.Errorf("citation check: scanning edge: %w", err)
		}
		if restricted[c.SourceType] {
			out = append(out, c)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("citation check: iterating edges: %w", err)
	}
	return out, nil
}
