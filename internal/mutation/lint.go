package mutation

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/peiman/vaultmind/internal/index"
)

// FixWikilinksResult is the output of a FixWikilinks run.
type FixWikilinksResult struct {
	FilesScanned int             `json:"files_scanned"`
	FilesChanged int             `json:"files_changed"`
	LinksFixed   int             `json:"links_fixed"`
	Details      []LinkFixDetail `json:"details,omitempty"`
}

// LinkFixDetail describes a single wikilink rewrite.
type LinkFixDetail struct {
	Path    string `json:"path"`
	OldLink string `json:"old_link"`
	NewLink string `json:"new_link"`
}

// wikilinkRe matches [[Target]] and [[Target|Display]]. Group 1 is the link
// target (the part before any |); group 2 (optional) wraps |Display and group
// 3 is the display text. Both forms are matched because doctor flags an
// id-form link as Obsidian-incompatible whether or not it carries a |display,
// so the healer must rewrite both to fix exactly what doctor flags. The #
// exclusion leaves heading/block-anchor links ([[Target#Heading]]) untouched.
var wikilinkRe = regexp.MustCompile(`\[\[([^\]|#]+?)(\|([^\]]*))?\]\]`)

// FixWikilinks scans all .md files in vaultPath and rewrites [[Target]] to
// [[filename|Target]] (and [[Target|Display]] to [[filename|Display]])
// wherever Target resolves to a note whose filename stem differs from the link
// target. When fix is false the function performs a dry-run: it counts and
// records changes but does not write any files.
func FixWikilinks(db *index.DB, vaultPath string, fix bool) (*FixWikilinksResult, error) {
	titleToStem, err := buildTitleStemMap(db)
	if err != nil {
		return nil, fmt.Errorf("building title→stem map: %w", err)
	}

	result := &FixWikilinksResult{}

	err = filepath.WalkDir(vaultPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			// Skip hidden directories (e.g. .obsidian, .vaultmind)
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		result.FilesScanned++

		raw, err := os.ReadFile(path) //nolint:gosec // path comes from WalkDir within vault
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		body, bodyStart := splitBody(raw)
		newBody, details := rewriteLinks(body, titleToStem)
		if len(details) == 0 {
			return nil
		}

		// Build detail records with vault-relative path
		relPath, _ := filepath.Rel(vaultPath, path)
		for i := range details {
			details[i].Path = relPath
		}
		result.Details = append(result.Details, details...)
		result.LinksFixed += len(details)
		result.FilesChanged++

		if fix {
			newContent := append(raw[:bodyStart], newBody...)             //nolint:gocritic // slice append is intentional
			if err := os.WriteFile(path, newContent, 0o640); err != nil { //nolint:gosec // path validated by WalkDir
				return fmt.Errorf("writing %s: %w", path, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// buildTitleStemMap queries the index and returns a map from any wikilink
// target form — note id, title, or alias — to the filename stem (basename
// without .md extension) of the note's file. ids are included so the healer
// rewrites the id-form links ([[reference-foo]]) that doctor flags as
// Obsidian-incompatible; without them the two disagreed and heal fixed 0.
// ids take precedence over titles/aliases on collision (Obsidian resolves a
// bare [[id]] to that note's file).
func buildTitleStemMap(db *index.DB) (map[string]string, error) {
	m := make(map[string]string)

	// Titles
	rows, err := db.Query("SELECT title, path FROM notes WHERE title != '' AND title IS NOT NULL")
	if err != nil {
		return nil, fmt.Errorf("querying notes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var title, path string
		if err := rows.Scan(&title, &path); err != nil {
			return nil, fmt.Errorf("scanning note: %w", err)
		}
		stem := strings.TrimSuffix(filepath.Base(path), ".md")
		m[title] = stem
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating notes: %w", err)
	}

	// Aliases
	aliasRows, err := db.Query(`
		SELECT a.alias, n.path
		FROM aliases a
		JOIN notes n ON n.id = a.note_id`)
	if err != nil {
		return nil, fmt.Errorf("querying aliases: %w", err)
	}
	defer func() { _ = aliasRows.Close() }()
	for aliasRows.Next() {
		var alias, path string
		if err := aliasRows.Scan(&alias, &path); err != nil {
			return nil, fmt.Errorf("scanning alias: %w", err)
		}
		stem := strings.TrimSuffix(filepath.Base(path), ".md")
		if _, exists := m[alias]; !exists {
			m[alias] = stem
		}
	}
	if err := aliasRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating aliases: %w", err)
	}

	if err := addIDStems(db, m); err != nil {
		return nil, err
	}

	return m, nil
}

// addIDStems maps every note id to its filename stem, overwriting any
// title/alias entry that collides. Obsidian resolves a bare [[id]] against the
// note's file, so the id form is authoritative — this is what reconciles the
// healer with doctor's incompatible-link flag (both key on the link target).
func addIDStems(db *index.DB, m map[string]string) error {
	idRows, err := db.Query("SELECT id, path FROM notes")
	if err != nil {
		return fmt.Errorf("querying note ids: %w", err)
	}
	defer func() { _ = idRows.Close() }()
	for idRows.Next() {
		var id, path string
		if err := idRows.Scan(&id, &path); err != nil {
			return fmt.Errorf("scanning note id: %w", err)
		}
		m[id] = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	if err := idRows.Err(); err != nil {
		return fmt.Errorf("iterating note ids: %w", err)
	}
	return nil
}

// splitBody splits raw file content into the frontmatter prefix and the body.
// bodyStart is the byte offset where the body begins (after the closing ---).
// If no frontmatter is detected, bodyStart is 0 and body is the full content.
func splitBody(raw []byte) (body []byte, bodyStart int) {
	s := string(raw)

	// Must start with ---
	if !strings.HasPrefix(s, "---") {
		return raw, 0
	}

	// Find closing ---
	rest := s[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return raw, 0
	}

	// bodyStart is after "---\n<fm>\n---\n"
	bodyStart = 3 + idx + len("\n---")
	// Consume the trailing newline of the closing delimiter if present
	if bodyStart < len(s) && s[bodyStart] == '\n' {
		bodyStart++
	}
	return []byte(s[bodyStart:]), bodyStart
}

// rewriteLinks rewrites incompatible wikilinks in body text. A bare
// [[Target]] becomes [[stem|Target]]; an aliased [[Target|Display]] becomes
// [[stem|Display]], preserving the display text (per the wikilink convention
// [[filename|Display Text]]). A link is rewritten only when Target resolves
// via the id/title/alias map to a note whose filename stem differs from
// Target. Returns the modified body and one detail per rewrite.
func rewriteLinks(body []byte, titleToStem map[string]string) ([]byte, []LinkFixDetail) {
	var details []LinkFixDetail

	result := wikilinkRe.ReplaceAllFunc(body, func(match []byte) []byte {
		// Re-match to recover the target (group 1) and optional display alias
		// (group 3). match is a single full match, so this always succeeds.
		groups := wikilinkRe.FindSubmatch(match)
		target := string(groups[1])
		stem, ok := titleToStem[target]
		if !ok || stem == target {
			return match // unknown or already compatible — leave as-is
		}
		// Preserve an explicit display alias; otherwise fall back to the
		// original target text as the display (keeps the link readable).
		display := target
		if groups[2] != nil {
			display = string(groups[3])
		}
		newLink := "[[" + stem + "|" + display + "]]"
		details = append(details, LinkFixDetail{
			OldLink: string(match),
			NewLink: newLink,
		})
		return []byte(newLink)
	})

	return result, details
}
