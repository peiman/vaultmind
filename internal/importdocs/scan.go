package importdocs

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/vault"
)

// doc is one markdown file in the source folder, or the one page a URL import
// turned into markdown.
type doc struct {
	// Rel is the doc's path under the source folder.
	Rel string
	// Path is the doc's path under the repository root, as `paths:` uses.
	// For a page it is the slug the note id is built from.
	Path   string
	Repo   string
	Source string
	// PathsEntry is the `paths:` entry when it is not Source: a book's
	// chapters have the book and the chapter as source, and the book as
	// paths.
	PathsEntry string
	// URL is the page address when the doc came from a URL import. Empty for
	// a folder import, which keeps `paths:` and must stay byte-identical.
	URL   string
	Title string
	Body  string
	// Hash is the sha256 of Body, the content the note carries.
	Hash string
}

// skippedFolders are dependency folders: their READMEs and CHANGELOGs are
// other projects' docs, and a repository root holds thousands of them.
var skippedFolders = map[string]bool{"node_modules": true, "vendor": true, "bower_components": true}

// scan reads every *.md under root, the source folder with links resolved.
// Symlinks are reported, not followed; hidden and dependency folders are left
// out, and so is the vault when it lives inside the source (otherwise a
// re-run imports its own notes). In a git work tree, what git ignores is left
// out too, and reported once.
func scan(src Source, root, vaultReal string) ([]doc, []Entry, error) {
	var docs []doc
	var skipped []Entry
	git := newGitFilter(root)
	var ignored []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if _, skip := vault.SkipSymlink(root, p, d); skip {
			skipped = append(skipped, Entry{Action: Skipped, Note: rel, Reason: "a symlink in the source; not followed"})
			return nil
		}
		if d.IsDir() {
			if p != root && (p == vaultReal || strings.HasPrefix(d.Name(), ".") || skippedFolders[d.Name()]) {
				return filepath.SkipDir
			}
			if p != root && git.ignored(rel, true) {
				ignored = append(ignored, rel+"/")
				return filepath.SkipDir
			}
			return nil
		}
		if !importable(p) {
			return nil
		}
		if git.ignored(rel, false) {
			ignored = append(ignored, rel)
			return nil
		}
		if hasControl(rel) {
			skipped = append(skipped, Entry{Action: Skipped, Note: strconv.Quote(rel), Reason: "a control character in the doc's path"})
			return nil
		}
		if convertedKind(p) != "" {
			// One converted file that cannot become notes is reported; the
			// folder goes on.
			if dcs, perr := readConvertedDocs(src, rel, p); perr == nil {
				docs = append(docs, dcs...)
			} else {
				skipped = append(skipped, Entry{Action: Skipped, Note: rel, Reason: perr.Error()})
			}
			return nil
		}
		dc, err := readDoc(src, rel, p)
		if err != nil {
			return err
		}
		docs = append(docs, dc)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("scanning %s: %w", src.Dir, err)
	}
	if len(ignored) > 0 {
		skipped = append(skipped, ignoredEntry(ignored))
	}
	return docs, skipped, nil
}

// ignoredEntry reports what git ignores as one line, naming the first few
// paths: a generated-output folder can hold hundreds of docs.
func ignoredEntry(paths []string) Entry {
	const shown = 3
	names := strings.Join(paths[:min(len(paths), shown)], ", ")
	if len(paths) > shown {
		names += ", …"
	}
	return Entry{Action: Skipped, Note: fmt.Sprintf("%d path(s)", len(paths)),
		Reason: "git ignores them, so they were not imported (" + names + ")"}
}

// importable reports a file an import reads: markdown, a PDF, or an Office
// document.
func importable(p string) bool {
	return strings.EqualFold(filepath.Ext(p), ".md") || convertedKind(p) != ""
}

// convertedKind is "pdf", "docx", "pptx", "xlsx", "html" (for .html and
// .htm), "csv", "tsv" or "epub" for a file whose text an import extracts,
// else "".
func convertedKind(p string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(p), "."))
	switch {
	case ext == "pdf" || officeKinds[ext] || ext == "csv" || ext == "tsv" || ext == "html" || ext == "epub":
		return ext
	case ext == "htm":
		return "html"
	}
	return ""
}

// readPDFText reads a PDF of at most 20 MiB whole and extracts its text.
func readPDFText(p string) (string, string, error) {
	info, err := os.Stat(p)
	if err != nil {
		return "", "", err
	}
	if info.Size() > maxPDFBytes {
		return "", "", fmt.Errorf("PDF is larger than 20 MiB")
	}
	// nosemgrep: go-path-traversal -- a file found by walking the folder the operator named; symlinks were skipped
	raw, err := os.ReadFile(p) //nolint:gosec // same
	if err != nil {
		return "", "", err
	}
	return pdfText(context.Background(), raw)
}

// readOfficeText extracts an Office file's text straight from the file: only
// its XML parts are read, so a deck full of pictures costs little.
func readOfficeText(kind, p string) (string, string, error) {
	// nosemgrep: go-path-traversal -- a file found by walking the folder the operator named; symlinks were skipped
	f, err := os.Open(p) //nolint:gosec // same
	if err != nil {
		return "", "", err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return "", "", err
	}
	if info.Size() > maxOfficeBytes {
		return "", "", fmt.Errorf("%s file is larger than %d MiB", kind, maxOfficeBytes>>20)
	}
	text, title, err := officeTextFrom(kind, f, info.Size())
	if err == nil && strings.TrimSpace(text) == "" {
		err = fmt.Errorf("the %s file holds no text", kind)
	}
	return text, title, err
}

// readConvertedDocs reads one converted file as its docs: a book is a doc
// per chapter and one for the book, any other file one doc.
func readConvertedDocs(src Source, rel, p string) ([]doc, error) {
	if convertedKind(p) == "epub" {
		return readEPUBDocs(src, rel, p)
	}
	d, err := readConvertedDoc(src, rel, p)
	if err != nil {
		return nil, err
	}
	return []doc{d}, nil
}

// readConvertedDoc reads one PDF, Office, HTML or CSV file as a doc: its extracted text
// is the body, and its title is the file's own (metadata, or a PDF's first
// line), else the file name. The hash is of the text, as for a PDF fetched by
// URL. A file over the size cap, one that cannot be read, and a PDF scan with
// no text are errors with the reason.
func readConvertedDoc(src Source, rel, p string) (doc, error) {
	kind := convertedKind(p)
	var text, pdfTitle string
	var err error
	docPath := path.Join(src.Prefix, rel)
	switch kind {
	case "pdf":
		text, pdfTitle, err = readPDFText(p)
	case "html":
		text, pdfTitle, err = readHTMLText(docPath, p)
	case "csv", "tsv":
		text, pdfTitle, err = readCSVText(kind, p)
	default:
		text, pdfTitle, err = readOfficeText(kind, p)
	}
	if err != nil {
		return doc{}, err
	}
	if pdfTitle == "" {
		pdfTitle = strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	}
	return doc{
		Rel: rel, Path: docPath, Repo: src.Repo,
		Source: src.Repo + ":" + docPath,
		Title:  pdfTitle,
		Body:   text, Hash: hashOf(text),
	}, nil
}

// readDoc reads one doc. A doc's own frontmatter is not body: its `title`
// names the note, the rest is dropped. Frontmatter that does not parse is
// kept as text, as the doc's reader would see it.
func readDoc(src Source, rel, p string) (doc, error) {
	// nosemgrep: go-path-traversal -- a file found by walking the folder the operator named; symlinks were skipped
	raw, err := os.ReadFile(p) //nolint:gosec // same
	if err != nil {
		return doc{}, err
	}
	docPath := path.Join(src.Prefix, rel)
	fm, body := splitFrontmatter(raw)
	return doc{
		Rel: rel, Path: docPath, Repo: src.Repo,
		Source: src.Repo + ":" + docPath,
		Title:  title(fm, body, rel),
		Body:   body, Hash: hashOf(body),
	}, nil
}

// splitFrontmatter separates a doc's frontmatter from its body. Frontmatter
// that does not parse is kept as text, as the doc's reader would see it.
func splitFrontmatter(raw []byte) (map[string]interface{}, string) {
	fm, body, err := parser.ExtractFrontmatter(raw)
	if err != nil {
		return nil, string(raw)
	}
	return fm, body
}

// title is the doc's frontmatter title, else its first `# ` heading, else
// its file name.
func title(fm map[string]interface{}, body, rel string) string {
	if t := frontmatterTitle(fm); t != "" {
		return t
	}
	if h, ok := headingTitle(body); ok {
		return h
	}
	return strings.TrimSuffix(path.Base(rel), path.Ext(rel))
}

// frontmatterTitle is a doc's frontmatter title, trimmed. Empty when there
// is none, so a heading or the file name can follow.
func frontmatterTitle(fm map[string]interface{}) string {
	if t, ok := fm["title"].(string); ok {
		return strings.TrimSpace(t)
	}
	return ""
}

// headingTitle is the first `# ` heading outside a fence. The bool is false
// when there is none. A heading that is only `# ` is found, and empty: the
// file name is not a substitute for a heading the author wrote.
func headingTitle(body string) (string, bool) {
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence && strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# ")), true
		}
	}
	return "", false
}
