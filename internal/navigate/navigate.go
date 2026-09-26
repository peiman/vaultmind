// Package navigate builds the map of a vault: what it holds, by folder, one
// line per note. An agent reads a file when it knows where the answer lives;
// with a knowledge base it can only know that if it can see what is there.
// Search answers a question already formed; the map lets the question form.
package navigate

import (
	"database/sql"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/peiman/vaultmind/internal/memory"
)

// MaxLineRunes caps a note's one-line description so the map stays a map.
const MaxLineRunes = 120

// excerptTokens bounds the excerpt the one line is cut from; only its first
// sentence survives, so this only needs to reach past a long opening sentence.
const excerptTokens = 80

// ellipsis marks a line that was cut.
const ellipsis = "…"

// Note is one entry in the map.
type Note struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Line  string `json:"line,omitempty"`
	// CodeChanged is set by `tree --for` when the file was committed to after
	// the note was.
	CodeChanged *CodeChange `json:"code_changed,omitempty"`
}

// Dir is a folder in the map. Total counts every note beneath it.
type Dir struct {
	Path  string `json:"path"`
	Total int    `json:"total"`
	Dirs  []*Dir `json:"dirs,omitempty"`
	Notes []Note `json:"notes,omitempty"`
}

// Filter narrows what Load returns. Empty fields do not filter.
type Filter struct {
	PathPrefix string
	Type       string
}

// Querier is the part of the index DB that Load needs.
type Querier interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// Load reads every indexed note matching f, with its one-line description.
func Load(q Querier, f Filter) ([]Note, error) {
	stmt := `SELECT id, path, COALESCE(title, ''), COALESCE(type, ''), COALESCE(body_text, '') FROM notes WHERE 1=1`
	var args []interface{}
	if f.PathPrefix != "" {
		stmt += ` AND path LIKE ? ESCAPE '\'`
		args = append(args, escapeLike(f.PathPrefix)+"%")
	}
	if f.Type != "" {
		stmt += ` AND type = ?`
		args = append(args, f.Type)
	}
	stmt += ` ORDER BY path`

	rows, err := q.Query(stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("listing notes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var notes []Note
	for rows.Next() {
		var n Note
		var body string
		if err := rows.Scan(&n.ID, &n.Path, &n.Title, &n.Type, &body); err != nil {
			return nil, fmt.Errorf("reading note row: %w", err)
		}
		if n.Title == "" {
			n.Title = n.ID
		}
		n.Line = OneLine(body)
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// escapeLike makes a path prefix literal inside a LIKE pattern.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Build groups notes into folders. Folders and notes are sorted by path.
func Build(notes []Note) *Dir {
	root := &Dir{}
	dirs := map[string]*Dir{"": root}
	var ensure func(p string) *Dir
	ensure = func(p string) *Dir {
		if d, ok := dirs[p]; ok {
			return d
		}
		d := &Dir{Path: p}
		dirs[p] = d
		parent := ensure(parentDir(p))
		parent.Dirs = append(parent.Dirs, d)
		return d
	}
	for _, n := range notes {
		d := ensure(parentDir(n.Path))
		d.Notes = append(d.Notes, n)
		for p := d.Path; ; p = parentDir(p) {
			dirs[p].Total++
			if p == "" {
				break
			}
		}
	}
	for _, d := range dirs {
		sort.Slice(d.Dirs, func(i, j int) bool { return d.Dirs[i].Path < d.Dirs[j].Path })
		sort.Slice(d.Notes, func(i, j int) bool { return d.Notes[i].Path < d.Notes[j].Path })
	}
	return root
}

// parentDir is the folder a path sits in, "" for the vault root.
func parentDir(p string) string {
	d := path.Dir(p)
	if d == "." || d == "/" {
		return ""
	}
	return d
}

// RenderOptions shape the printed map.
type RenderOptions struct {
	// Depth limits how many folder levels are listed; deeper folders are folded
	// into their parent's count. Notes are listed only inside listed folders
	// above the limit. 0 lists everything.
	Depth int
	// Brief prints titles and ids without the one-line descriptions.
	Brief bool
}

// Render prints the map: each folder with its note count, each note as
// "Title (id) — line", indented by depth.
func Render(w io.Writer, root *Dir, o RenderOptions) error {
	return renderDir(w, root, 0, o)
}

func renderDir(w io.Writer, d *Dir, level int, o RenderOptions) error {
	indent := strings.Repeat("  ", level)
	if o.Depth == 0 || level < o.Depth {
		for _, n := range d.Notes {
			if _, err := fmt.Fprintf(w, "%s%s\n", indent, noteLine(n, o.Brief)); err != nil {
				return err
			}
			if n.CodeChanged != nil {
				if _, err := fmt.Fprintf(w, "%s    %s\n", indent, n.CodeChanged.Summary); err != nil {
					return err
				}
			}
		}
	}
	for _, sub := range d.Dirs {
		if o.Depth != 0 && level+1 > o.Depth {
			continue
		}
		if _, err := fmt.Fprintf(w, "%s%s/ (%d)\n", indent, sub.Path, sub.Total); err != nil {
			return err
		}
		if err := renderDir(w, sub, level+1, o); err != nil {
			return err
		}
	}
	return nil
}

func noteLine(n Note, brief bool) string {
	s := fmt.Sprintf("%s (%s)", n.Title, n.ID)
	if !brief && n.Line != "" {
		s += " — " + n.Line
	}
	return s
}

// OneLine is a note's description for the map: the first sentence of its
// excerpt (the Principle section when it has one, else its opening prose),
// on one line and capped at MaxLineRunes.
func OneLine(body string) string {
	text := strings.Join(strings.Fields(memory.Excerpt(body, excerptTokens)), " ")
	text = firstSentence(text)
	runes := []rune(text)
	if len(runes) > MaxLineRunes {
		cut := strings.TrimRightFunc(string(runes[:MaxLineRunes-1]), unicode.IsSpace)
		return cut + ellipsis
	}
	return text
}

// abbreviations end in a period without ending a sentence. Lowercased, without
// the final period.
var abbreviations = map[string]bool{
	"e.g": true, "i.e": true, "etc": true, "vs": true, "cf": true, "al": true,
	"dr": true, "mr": true, "mrs": true, "ms": true, "prof": true, "fig": true, "no": true,
}

// firstSentence returns text up to and including its first sentence end
// (". ", "! ", "? " or the end of the text). A period after an abbreviation
// or a single initial ("E.") does not end the sentence.
func firstSentence(text string) string {
	for i := 0; i+1 < len(text); i++ {
		switch text[i] {
		case '!', '?':
			if text[i+1] == ' ' {
				return text[:i+1]
			}
		case '.':
			if text[i+1] == ' ' && !isAbbreviation(text[:i]) {
				return text[:i+1]
			}
		}
	}
	return text
}

// isAbbreviation reports whether the word just before a period is an
// abbreviation or a single-letter initial.
func isAbbreviation(before string) bool {
	word := before[strings.LastIndexAny(before, " (")+1:]
	if len([]rune(word)) == 1 {
		return true
	}
	return abbreviations[strings.ToLower(word)]
}
