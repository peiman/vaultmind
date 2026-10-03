package importdocs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/vault"
	"gopkg.in/yaml.v3"
)

// note is what a previous import left at a path.
type note struct {
	exists bool
	// managed is true for a note the import wrote: it has source and
	// source_hash. Anything else is never overwritten or pruned.
	managed    bool
	id         string
	source     string
	sourceHash string
	// bodyHash differs from sourceHash when the body was edited by hand.
	bodyHash string
	// body is the note's body as it stands, kept when only the source moved.
	body  string
	title string
	// extra is frontmatter the import does not own; an update keeps it.
	extra map[string]interface{}
}

// edited reports a body changed by hand since the import wrote it: the body
// is the doc verbatim, so its hash is the source hash until someone edits it.
func (n note) edited() bool { return n.bodyHash != n.sourceHash }

// ownedKeys are the frontmatter keys the import writes and refreshes.
var ownedKeys = map[string]bool{
	"id": true, "type": true, "title": true, "url": true, "paths": true, "source": true, "source_hash": true,
	"part_of": true,
}

// readNotes reads the notes under dir, keyed by vault-relative path.
// vaultRoot is absolute. A link anywhere from the vault root down to dir is
// refused before anything is read through it. A note that cannot be read
// stops the import only inside base, this import's own folder; elsewhere it
// is another import's note, passed over.
func readNotes(vaultRoot, dir, base string) (map[string]note, []Entry, error) {
	notes := map[string]note{}
	if err := noLinkOnTheWay(vaultRoot, dir, false); err != nil {
		return nil, nil, err
	}
	abs, err := vault.ResolveInside(vaultRoot, dir)
	if err != nil {
		return nil, nil, err
	}
	var skipped []Entry
	err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, walkErr error) error {
		rel, err := filepath.Rel(vaultRoot, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		mine := rel == base || strings.HasPrefix(rel, base+"/") || strings.HasPrefix(base, rel+"/") || p == abs
		if walkErr != nil {
			return passOver(walkErr, mine, d)
		}
		if _, skip := vault.SkipSymlink(vaultRoot, p, d); skip {
			skipped = append(skipped, Entry{Action: Skipped, Note: rel, Reason: "a symlink in the vault; not followed"})
			return nil
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".md") {
			return nil
		}
		n, err := readNote(p)
		if err != nil {
			return passOver(err, mine, d)
		}
		notes[rel] = n
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, fmt.Errorf("reading imported notes: %w", err)
	}
	return notes, skipped, nil
}

// passOver returns err for a path in this import's folder (or above it), and
// skips the path anywhere else.
func passOver(err error, mine bool, d fs.DirEntry) error {
	if mine {
		return err
	}
	if d != nil && d.IsDir() {
		return filepath.SkipDir
	}
	return nil
}

func readNote(p string) (note, error) {
	// nosemgrep: go-path-traversal -- a file found by walking the vault's imported folder; symlinks were skipped
	raw, err := os.ReadFile(p) //nolint:gosec // same
	if err != nil {
		return note{}, err
	}
	return parseNote(raw), nil
}

// parseNote reads what an import needs from a note. Frontmatter that does not
// parse is not a note the import wrote: it exists, and is never touched.
func parseNote(raw []byte) note {
	n := note{exists: true}
	fm, body, err := parser.ExtractFrontmatter(raw)
	if err != nil {
		return n
	}
	n.id, _ = fm["id"].(string)
	n.source, _ = fm["source"].(string)
	n.sourceHash, _ = fm["source_hash"].(string)
	n.title, _ = fm["title"].(string)
	n.managed = n.source != "" && n.sourceHash != ""
	n.bodyHash = hashOf(body)
	n.body = body
	n.extra = map[string]interface{}{}
	for k, v := range fm {
		if !ownedKeys[k] {
			n.extra[k] = v
		}
	}
	return n
}

// owned is the frontmatter the import writes, in the order it is written.
// url is omitted for a folder import and paths for a page, so a folder note
// stays the bytes it was before pages existed.
type owned struct {
	ID         string   `yaml:"id"`
	Type       string   `yaml:"type"`
	Title      string   `yaml:"title"`
	URL        string   `yaml:"url,omitempty"`
	Paths      []string `yaml:"paths,omitempty"`
	Source     string   `yaml:"source"`
	SourceHash string   `yaml:"source_hash"`
	PartOf     string   `yaml:"part_of,omitempty"`
}

// render is the note for d: the owned frontmatter, then the keys a person
// added to the previous copy, then the doc's body unchanged.
func render(d doc, id string, prev note) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	// A comment line: a control character in the source would end it and
	// write frontmatter of its own, so none gets through.
	fmt.Fprintf(&b, "# Imported by `vaultmind import` from %s.\n", strings.Map(dropControl, d.Source))
	head := owned{
		ID: id, Type: "reference", Title: d.Title,
		Source: d.Source, SourceHash: d.Hash, PartOf: d.PartOf,
	}
	if d.URL != "" {
		b.WriteString("# The page is the source: re-run the import to refresh it.\n")
		head.URL = d.URL
	} else {
		b.WriteString("# The doc is the source: edit it, then re-run the import.\n")
		if d.PartOf == "" { // a section: the doc's index note carries paths:
			head.Paths = []string{d.Source}
		}
	}
	raw, _ := yaml.Marshal(head)
	b.Write(raw)
	b.Write(extraYAML(prev.extra))
	b.WriteString("---\n")
	b.WriteString(d.Body)
	return []byte(b.String())
}

func dropControl(r rune) rune {
	if unicode.IsControl(r) {
		return -1
	}
	return r
}

// extraYAML renders the kept keys sorted, so a re-run writes the same bytes.
func extraYAML(extra map[string]interface{}) []byte {
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []byte
	for _, k := range keys {
		chunk, err := yaml.Marshal(map[string]interface{}{k: extra[k]})
		if err == nil {
			out = append(out, chunk...)
		}
	}
	return out
}

// writer writes and removes notes inside the vault, never through a link.
type writer struct {
	vaultRoot string
	dryRun    bool
}

func (w writer) write(rel string, content []byte) error {
	if w.dryRun {
		return nil
	}
	abs, err := w.target(rel)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".import-*.md")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	// CreateTemp makes 0600; a note is as readable as one note create writes.
	if err := tmp.Chmod(0o640); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Rename replaces a link at abs rather than writing through it, and
	// target has already refused one.
	return os.Rename(tmp.Name(), abs)
}

// rename moves a note to the name its doc now gives it — on a
// case-insensitive filesystem, the same file under a new case.
func (w writer) rename(from, to string) error {
	if w.dryRun {
		return nil
	}
	src, err := w.target(from)
	if err != nil {
		return err
	}
	dst, err := w.target(to)
	if err != nil {
		return err
	}
	return os.Rename(src, dst)
}

func (w writer) remove(rel string) error {
	if w.dryRun {
		return nil
	}
	abs, err := w.target(rel)
	if err != nil {
		return err
	}
	return os.Remove(abs)
}

// target confines rel to the vault and creates its folders, refusing a link
// anywhere on the way: at the note itself or at any folder above it.
func (w writer) target(rel string) (string, error) {
	abs, err := vault.ResolveInside(w.vaultRoot, rel)
	if err != nil {
		return "", err
	}
	if err := noLinkOnTheWay(w.vaultRoot, rel, true); err != nil {
		return "", err
	}
	return abs, nil
}

// noLinkOnTheWay walks rel down from the absolute vault root and refuses a
// symlink at any step. With create, missing folders above the last step are
// made; without, a missing step ends the walk (there is nothing to read).
func noLinkOnTheWay(root, rel string, create bool) error {
	cur := root
	parts := strings.Split(path.Clean(rel), "/")
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		switch {
		case err == nil && info.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("%s is a symlink; not followed", path.Join(parts[:i+1]...))
		case errors.Is(err, fs.ErrNotExist) && !create:
			return nil
		case errors.Is(err, fs.ErrNotExist) && i < len(parts)-1:
			if err := os.Mkdir(cur, 0o750); err != nil {
				return err
			}
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return err
		}
	}
	return nil
}
