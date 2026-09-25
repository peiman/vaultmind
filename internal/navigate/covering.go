package navigate

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// PathsField is the frontmatter field in which a note names the code it is
// about: globs relative to the repository root, `**` for any depth, a trailing
// `/` for everything under a folder, and `repo:` in front for another repo.
const PathsField = "paths"

// repoSeparator splits `repo:path/glob` into the repository and the glob.
const repoSeparator = ":"

// anyDepth matches zero or more path segments.
const anyDepth = "**"

// CodeFile is the file an agent is reading: its path relative to its
// repository root, and the repository's name (the root folder's name).
type CodeFile struct {
	Rel  string
	Repo string
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
		if MatchPath(p, f.Rel) {
			return true
		}
	}
	return false
}

// gitMarker marks a repository root: a directory in a clone, a file in a
// worktree.
const gitMarker = ".git"

// ResolveCodeFile names a file the way `paths:` does: relative to the nearest
// enclosing repository root, and the repository by that root's folder name.
// Outside a repository the Rel is the file name alone and Repo is empty.
func ResolveCodeFile(file string) CodeFile {
	abs, err := filepath.Abs(file)
	if err != nil {
		return CodeFile{Rel: filepath.Base(file)}
	}
	for dir := filepath.Dir(abs); ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, gitMarker)); err == nil {
			rel, relErr := filepath.Rel(dir, abs)
			if relErr != nil {
				break
			}
			return CodeFile{Rel: filepath.ToSlash(rel), Repo: filepath.Base(dir)}
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	return CodeFile{Rel: filepath.Base(abs)}
}

// MatchPath reports whether rel (slash-separated, relative to the repository
// root) matches pattern. Segments match as in path.Match; `**` matches any
// number of segments, and a trailing `/` covers everything beneath.
func MatchPath(pattern, rel string) bool {
	pattern = strings.TrimPrefix(strings.TrimSpace(pattern), "./")
	if strings.HasSuffix(pattern, "/") {
		pattern += anyDepth
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

func matchSegments(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == anyDepth {
			for i := 0; i <= len(segs); i++ {
				if matchSegments(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], segs[0]); !ok {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}
