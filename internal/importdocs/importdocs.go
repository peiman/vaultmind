// Package importdocs copies a folder of markdown docs, or one web page, into
// a vault as notes, and keeps the copies in step on every re-run.
//
// The doc is the source of truth; its note is a projection. Each note carries
// the doc as `paths:` (so reading the doc brings the note) and as `source:`
// plus `source_hash:` (so a re-run knows what changed). Copying rather than
// mounting keeps every write inside the vault's confinement — see
// docs/specs in the research repo, plan item 6.
package importdocs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/peiman/vaultmind/internal/vault"
)

// Action is what an import did, or would do, with one doc or note.
type Action string

// The actions an import reports.
const (
	Added     Action = "added"
	Updated   Action = "updated"
	Unchanged Action = "unchanged"
	// Orphaned is a note whose doc is gone. It stays until --prune.
	Orphaned Action = "orphaned"
	Pruned   Action = "pruned"
	// Conflict is a note edited by hand whose doc also changed. It is kept;
	// --force overwrites it.
	Conflict Action = "conflict"
	// Skipped is a doc or note the import will not touch: a symlink, a file
	// it did not write, or an id another doc already took.
	Skipped Action = "skipped"
)

// ImportedDir is the vault folder imported notes live under.
const ImportedDir = "imported"

// Source names the docs to import. The command layer resolves Repo and Prefix
// from the repository around Dir, as `paths:` does.
type Source struct {
	Dir string
	// Repo is the repository name `paths:` entries use.
	Repo string
	// Prefix is Dir relative to the repository root, "" at the root.
	Prefix string
}

// Options change what an import writes.
type Options struct {
	DryRun bool
	Prune  bool
	Force  bool
	// Excludes is the vault's exclude list: a note it would hide from the
	// index is not written.
	Excludes []string
}

// Entry is one line of the report.
type Entry struct {
	Action Action `json:"action"`
	// Note is the note's vault-relative path.
	Note string `json:"note"`
	// Source is the doc as `<repo>:<path>`, empty for a skipped note.
	Source string `json:"source,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Result is the whole report.
type Result struct {
	Entries []Entry `json:"entries"`
	DryRun  bool    `json:"dry_run"`
}

// Count returns how many entries took action a.
func (r *Result) Count(a Action) int {
	n := 0
	for _, e := range r.Entries {
		if e.Action == a {
			n++
		}
	}
	return n
}

// Changed returns the vault-relative paths written or removed, which the
// caller re-indexes.
func (r *Result) Changed() []string {
	var out []string
	for _, e := range r.Entries {
		switch e.Action {
		case Added, Updated, Pruned:
			out = append(out, e.Note)
		}
	}
	return out
}

// ErrNoMarkdown reports a source folder with nothing to import.
var ErrNoMarkdown = errors.New("no markdown files")

// Import brings the docs under src into vaultRoot and reports what it did.
func Import(src Source, vaultRoot string, opts Options) (*Result, error) {
	if err := validRepo(src.Repo); err != nil {
		return nil, err
	}
	// Every path below is absolute: a relative vault root made the notes a
	// previous import wrote unrecognisable (review 2026-09-27, finding 1).
	vaultAbs, vaultReal, srcRoot, err := roots(src.Dir, vaultRoot)
	if err != nil {
		return nil, err
	}
	docs, skipped, err := scan(src, srcRoot, vaultReal)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("%s: %w to import", src.Dir, ErrNoMarkdown)
	}
	base := path.Join(ImportedDir, src.Repo, src.Prefix)
	// Every imported note is read, not only this folder's: an id must be
	// unique across all imports in the vault.
	all, noteSkips, err := readNotes(vaultAbs, ImportedDir, base)
	if err != nil {
		return nil, err
	}
	s := newSyncer(src, srcRoot, base, all, opts, writer{vaultRoot: vaultAbs, dryRun: opts.DryRun})
	res := &Result{DryRun: opts.DryRun, Entries: append(skipped, under(noteSkips, base)...)}
	res.Entries = append(res.Entries, s.syncDocs(docs)...)
	res.Entries = append(res.Entries, s.orphans()...)
	sort.SliceStable(res.Entries, func(i, j int) bool { return res.Entries[i].Note < res.Entries[j].Note })
	return res, nil
}

// absVault resolves the vault to an absolute path with links resolved.
// A relative vault root made the notes a previous import wrote unrecognisable.
func absVault(vaultRoot string) (vaultAbs, vaultReal string, err error) {
	if vaultAbs, err = filepath.Abs(vaultRoot); err != nil {
		return "", "", err
	}
	if vaultReal, err = filepath.EvalSymlinks(vaultAbs); err != nil {
		return "", "", fmt.Errorf("vault %s: %w", vaultRoot, err)
	}
	return vaultAbs, vaultReal, nil
}

// roots resolves the vault and the source to absolute paths with links
// resolved, and refuses a source inside the vault: its notes are already
// there, and importing the vault into itself nests a copy on every run.
func roots(srcDir, vaultRoot string) (vaultAbs, vaultReal, srcRoot string, err error) {
	if vaultAbs, vaultReal, err = absVault(vaultRoot); err != nil {
		return "", "", "", err
	}
	srcAbs, err := filepath.Abs(srcDir)
	if err != nil {
		return "", "", "", err
	}
	if srcRoot, err = filepath.EvalSymlinks(srcAbs); err != nil {
		return "", "", "", fmt.Errorf("source %s: %w", srcDir, err)
	}
	if srcRoot == vaultReal || strings.HasPrefix(srcRoot, vaultReal+string(filepath.Separator)) {
		return "", "", "", fmt.Errorf("%s is inside the vault: its notes are already there", srcDir)
	}
	return vaultAbs, vaultReal, srcRoot, nil
}

// validRepo refuses a repository name that is not one plain folder name: it
// becomes a folder in the vault and a prefix in every note id.
func validRepo(repo string) error {
	if repo == "" || repo == "." || repo == ".." || strings.ContainsAny(repo, `/\`) || hasControl(repo) {
		return fmt.Errorf("repository name %q is not a single folder name", repo)
	}
	return nil
}

// hasControl reports a control character — a newline in a path would write
// its own lines into the note's frontmatter.
func hasControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

// syncer carries one import's state through planning and writing.
//
// Note paths are matched without regard to case: on the default macOS
// filesystem Foo.md and foo.md are one file, and a lookup that tells them
// apart writes over a note it never checked (re-review 2026-09-27).
type syncer struct {
	src     Source
	srcRoot string
	base    string
	// notes are this import's notes; folded maps a lowercased path to every
	// path on disk that lowercases to it — more than one only on a
	// case-sensitive filesystem (#189).
	notes  map[string]note
	folded map[string][]string
	// taken maps every imported note's id, in any folder, to its path.
	taken map[string]string
	opts  Options
	w     writer
	// claimed holds the note paths on disk this run matched or wrote, exact:
	// a note not claimed may be an orphan, whatever its case variants did.
	claimed map[string]bool
	// written holds the lowercased note paths this run produced.
	written map[string]bool
}

func newSyncer(src Source, srcRoot, base string, all map[string]note, opts Options, w writer) *syncer {
	s := &syncer{src: src, srcRoot: srcRoot, base: base, opts: opts, w: w,
		notes: map[string]note{}, folded: map[string][]string{}, taken: map[string]string{},
		claimed: map[string]bool{}, written: map[string]bool{}}
	for rel, n := range all {
		if n.id != "" {
			s.taken[n.id] = rel
		}
		if strings.HasPrefix(rel, base+"/") {
			s.notes[rel] = n
			key := strings.ToLower(rel)
			s.folded[key] = append(s.folded[key], rel)
		}
	}
	for _, paths := range s.folded {
		sort.Strings(paths) // map order is random; a report must not be
	}
	return s
}

// under keeps the entries inside base.
func under(entries []Entry, base string) []Entry {
	var out []Entry
	for _, e := range entries {
		if strings.HasPrefix(e.Note, base+"/") {
			out = append(out, e)
		}
	}
	return out
}

// syncDocs plans and applies add / update / unchanged / conflict per doc.
func (s *syncer) syncDocs(docs []doc) []Entry {
	var out []Entry
	for _, d := range docs {
		rel := path.Join(s.base, noteName(d.Rel))
		key := strings.ToLower(rel)
		e := Entry{Note: rel, Source: d.Source}
		switch {
		case s.written[key]:
			e.Action, e.Reason = Skipped, "another doc maps to the same note (names differ only in case)"
		case vault.Excluded(rel, s.opts.Excludes):
			e.Action, e.Reason = Skipped, "the vault's exclude list hides this path from the index"
		case s.ambiguous(rel):
			e.Action = Skipped
			e.Reason = "several notes differ from this one only in case (" + strings.Join(s.folded[key], ", ") + "); remove all but one"
			s.claimAll(key)
		default:
			existing := s.existing(rel)
			s.claimed[existing], s.claimed[rel] = true, true
			e = s.syncDoc(d, rel, existing)
		}
		s.written[key] = true
		out = append(out, e)
	}
	return out
}

// existing is the path on disk of the note at rel: the exact path, else the
// one note whose name differs only in case, else "".
func (s *syncer) existing(rel string) string {
	if s.notes[rel].exists {
		return rel
	}
	if paths := s.folded[strings.ToLower(rel)]; len(paths) == 1 {
		return paths[0]
	}
	return ""
}

// ambiguous reports several notes differing from rel only in case, none of
// them rel itself — which one the doc meant cannot be told, so none is used.
func (s *syncer) ambiguous(rel string) bool {
	return !s.notes[rel].exists && len(s.folded[strings.ToLower(rel)]) > 1
}

// claimAll marks every case variant of key as accounted for, so an ambiguous
// set is reported once and none of it is pruned as an orphan.
func (s *syncer) claimAll(key string) {
	for _, p := range s.folded[key] {
		s.claimed[p] = true
	}
}

// syncDoc plans one doc and writes it when it needs writing. A note found
// under another case of the name is checked like any other, and renamed to
// the doc's name when it is written.
func (s *syncer) syncDoc(d doc, rel, existing string) Entry {
	prev := s.notes[existing]
	e := Entry{Note: rel, Source: d.Source}
	e.Action, e.Reason = plan(d, prev, s.opts)
	if e.Action != Added && e.Action != Updated {
		if existing != "" {
			e.Note = existing
		}
		return e
	}
	if prev.managed && prev.sourceHash == d.Hash && prev.edited() {
		// A rename of an unchanged doc: the hand-edited body stays, and so does
		// the source hash, so the note still counts as edited.
		d.Body = prev.body
	}
	id := assignID(d, existing, prev, s.taken)
	s.taken[id] = rel
	if existing != "" && existing != rel {
		if err := s.w.rename(existing, rel); err != nil {
			e.Action, e.Reason = Skipped, err.Error()
			return e
		}
	}
	if err := s.w.write(rel, render(d, id, prev)); err != nil {
		e.Action, e.Reason = Skipped, err.Error()
	}
	return e
}

// assignID keeps the id a note already has. A new note gets the slug of its
// doc's path, or — when another note holds that — the slug plus a short hash
// of the path, so no two notes share an id and none changes on a re-run.
func assignID(d doc, rel string, prev note, taken map[string]string) string {
	if prev.managed && prev.id != "" {
		return prev.id
	}
	id := noteID(d.Repo, d.Path)
	if holder, ok := taken[id]; ok && holder != rel {
		id += "-" + hashOf(d.Path)[:8]
	}
	return id
}

// plan decides what one doc needs, given the note a previous import wrote.
func plan(d doc, n note, opts Options) (Action, string) {
	switch {
	case !n.exists:
		return Added, ""
	case !n.managed:
		return Skipped, "a note the import did not write is in the way"
	case d.URL != "" && n.source != d.Source:
		// Two URLs can slug to one path. A folder import treats that as a
		// renamed doc; a page import would erase the other page. --force
		// does not override it.
		return Skipped, "another page maps to this note (" + n.source + ")"
	case n.sourceHash == d.Hash && n.title == d.Title && n.source == d.Source:
		return Unchanged, ""
	case n.sourceHash == d.Hash && n.title == d.Title:
		// Only the doc's name changed: refresh source and paths. A hand-edited
		// body is kept (see syncDoc) — the doc did not change, so nothing is
		// lost either way.
		return Updated, "its doc was renamed"
	case n.edited() && !opts.Force:
		return Conflict, "edited by hand since the last import; --force overwrites"
	}
	return Updated, ""
}

// orphans reports the notes this import wrote whose doc is gone from disk,
// and removes them under --prune. A note is this import's only when its
// source lies under this source; a doc still on disk that the scan passed
// over (a hidden folder, a link) does not orphan its note.
func (s *syncer) orphans() []Entry {
	var out []Entry
	for rel, n := range s.notes {
		if s.claimed[rel] || !n.managed || !s.docGone(n.source) {
			continue
		}
		e := Entry{Action: Orphaned, Note: rel, Source: n.source, Reason: "its doc is gone; --prune removes it"}
		if s.opts.Prune {
			e = s.prune(rel, n, e)
		}
		out = append(out, e)
	}
	return out
}

// docGone reports whether source names a doc under this import's source
// folder that no longer exists. A source containing :// is a web page, not a
// file in any repository — a repository named http or https must not report
// or prune those notes. The check is on the string, before any disk read:
// filepath.Join would treat the "//host/..." rest as an absolute path.
func (s *syncer) docGone(source string) bool {
	if strings.Contains(source, "://") {
		return false
	}
	under := s.src.Repo + ":"
	if s.src.Prefix != "" {
		under += s.src.Prefix + "/"
	}
	if !strings.HasPrefix(source, under) {
		return false
	}
	_, err := os.Lstat(filepath.Join(s.srcRoot, filepath.FromSlash(strings.TrimPrefix(source, under))))
	return errors.Is(err, fs.ErrNotExist)
}

// prune removes an orphan — unless it was edited by hand, when the note is
// the only copy of that edit and needs --force.
func (s *syncer) prune(rel string, n note, e Entry) Entry {
	if n.edited() && !s.opts.Force {
		e.Action, e.Reason = Conflict, "its doc is gone, and it was edited by hand; --force removes it"
		return e
	}
	e.Action, e.Reason = Pruned, ""
	if err := s.w.remove(rel); err != nil {
		e.Action, e.Reason = Skipped, err.Error()
	}
	return e
}

// noteName is the note's path for a doc's: a lowercase .md, which the index
// reads, and README renamed — vaults exclude README.md as their own meta
// file, and a docs folder's README is its overview. A PDF's note is
// <name>-pdf.md, so paper.md and paper.pdf in one folder stay two notes.
func noteName(rel string) string {
	dir, file := path.Split(rel)
	stem := strings.TrimSuffix(file, path.Ext(file))
	if strings.EqualFold(stem, "readme") {
		stem = "readme"
	}
	if isPDF(file) {
		stem += "-pdf"
	}
	return dir + stem + vault.NoteExtension
}

// ImportFile imports one .md or .pdf file, rel under src.Dir, with the
// repository and prefix of that folder. There is no orphan pass: this import
// has one doc, and the folder's other notes are not its to report or prune.
// A PDF that cannot become a note is an error here, not a skip — it is the
// one thing the operator asked for.
func ImportFile(src Source, rel, vaultRoot string, opts Options) (*Result, error) {
	if err := validRepo(src.Repo); err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if !importable(rel) {
		return nil, fmt.Errorf("%s: only a .md or a .pdf file can be imported", rel)
	}
	if strings.Contains(rel, "/") || rel == "." || rel == ".." {
		return nil, fmt.Errorf("%s: name a file in the source folder", rel)
	}
	vaultAbs, _, srcRoot, err := roots(src.Dir, vaultRoot)
	if err != nil {
		return nil, err
	}
	p := filepath.Join(srcRoot, rel)
	info, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", rel)
	}
	var d doc
	if isPDF(rel) {
		d, err = readPDFDoc(src, rel, p)
	} else {
		d, err = readDoc(src, rel, p)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	base := path.Join(ImportedDir, src.Repo, src.Prefix)
	all, noteSkips, err := readNotes(vaultAbs, ImportedDir, base)
	if err != nil {
		return nil, err
	}
	s := newSyncer(src, srcRoot, base, all, opts, writer{vaultRoot: vaultAbs, dryRun: opts.DryRun})
	res := &Result{DryRun: opts.DryRun, Entries: under(noteSkips, path.Join(base, noteName(rel)))}
	res.Entries = append(res.Entries, s.syncDocs([]doc{d})...)
	return res, nil
}

// noteID is the slug of the repository and the doc's path.
func noteID(repo, docPath string) string {
	return "imported-" + slug(repo) + "-" + slug(strings.TrimSuffix(docPath, path.Ext(docPath)))
}

// slug lowercases s and turns every run of characters other than letters and
// digits (in any script) into one dash.
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
