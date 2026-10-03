package navigate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/pathglob"
)

// PathsField is the frontmatter field in which a note names the code it is
// about: globs relative to the repository root, `**` for any depth, a trailing
// `/` for everything under a folder, and `repo:` in front for another repo.
const PathsField = "paths"

// repoSeparator splits `repo:path/glob` into the repository and the glob.
const repoSeparator = ":"

// CodeFile is the file an agent is reading: its path relative to its
// repository root, and the repository's name (the root folder's name).
type CodeFile struct {
	Rel  string
	Repo string
	// Root is the repository's folder, empty outside a repository. It is where
	// the file's history is read.
	Root string
}

// Covering returns the notes whose `paths:` cover f, each with its line.
func Covering(q Querier, f CodeFile) ([]Note, error) {
	rows, err := q.Query(`SELECT n.id, n.path, COALESCE(n.title, ''), COALESCE(n.type, ''),
		COALESCE(n.body_text, ''), kv.value_json
		FROM frontmatter_kv kv JOIN notes n ON n.id = kv.note_id
		WHERE kv.key = ? ORDER BY n.path`, PathsField)
	if err != nil {
		return nil, fmt.Errorf("listing notes with %s: %w", PathsField, err)
	}
	defer func() { _ = rows.Close() }()

	var notes []Note
	for rows.Next() {
		var n Note
		var body, patterns string
		if err := rows.Scan(&n.ID, &n.Path, &n.Title, &n.Type, &body, &patterns); err != nil {
			return nil, fmt.Errorf("reading note row: %w", err)
		}
		if !coversAny(decodePatterns(patterns), f) {
			continue
		}
		if n.Title == "" {
			n.Title = n.ID
		}
		n.Line = OneLine(body)
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// decodePatterns reads the stored field: a list of globs or a single glob.
func decodePatterns(js string) []string {
	var list []string
	if json.Unmarshal([]byte(js), &list) == nil {
		return list
	}
	var one string
	if json.Unmarshal([]byte(js), &one) == nil && one != "" {
		// A list written as a quoted string (`frontmatter set` did, #159).
		if json.Unmarshal([]byte(one), &list) == nil {
			return list
		}
		return []string{one}
	}
	return nil
}

func coversAny(patterns []string, f CodeFile) bool {
	for _, p := range patterns {
		if repo, rest, ok := strings.Cut(p, repoSeparator); ok && !strings.Contains(repo, "/") {
			if repo != f.Repo {
				continue
			}
			p = rest
		}
		if pathglob.Match(p, f.Rel) {
			return true
		}
	}
	return false
}

// gitMarker marks a repository root: a directory in a clone, a file in a
// worktree.
const gitMarker = ".git"

// ResolveCodeFile names a file the way `paths:` does: relative to the nearest
// enclosing repository root, and the repository by its origin remote (the
// root's folder name when it has none).
// Outside a repository the Rel is the file name alone and Repo is empty.
func ResolveCodeFile(file string) CodeFile {
	abs, err := filepath.Abs(file)
	if err != nil {
		return CodeFile{Rel: filepath.Base(file)}
	}
	abs = realPath(abs)
	for dir := filepath.Dir(abs); ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, gitMarker)); err == nil {
			rel, relErr := filepath.Rel(dir, abs)
			if relErr != nil {
				break
			}
			return CodeFile{Rel: filepath.ToSlash(rel), Repo: repoName(dir), Root: dir}
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	return CodeFile{Rel: filepath.Base(abs)}
}

// realPath resolves symlinks, so a file reached through a link names the same
// file as its target. A file that does not exist yet (a Write creating it) has
// its folder resolved instead.
func realPath(abs string) string {
	if p, err := filepath.EvalSymlinks(abs); err == nil {
		return p
	}
	if d, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		return filepath.Join(d, filepath.Base(abs))
	}
	return abs
}
