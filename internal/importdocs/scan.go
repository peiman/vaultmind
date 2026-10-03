package importdocs

import (
	"context"
	"errors"
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
// re-run imports its own notes).
func scan(src Source, root, vaultReal string) ([]doc, []Entry, error) {
	var docs []doc
	var skipped []Entry
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
			return nil
		}
		if !importable(p) {
			return nil
		}
		if hasControl(rel) {
			skipped = append(skipped, Entry{Action: Skipped, Note: strconv.Quote(rel), Reason: "a control character in the doc's path"})
			return nil
		}
		if isPDF(p) {
			// One PDF that cannot become a note is reported; the folder goes on.
			if dc, perr := readPDFDoc(src, rel, p); perr == nil {
				docs = append(docs, dc)
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
	return docs, skipped, nil
}

// importable reports a file an import reads: markdown, or a PDF.
func importable(p string) bool {
	return strings.EqualFold(filepath.Ext(p), ".md") || isPDF(p)
}

func isPDF(p string) bool { return strings.EqualFold(filepath.Ext(p), ".pdf") }

// readPDFDoc reads one PDF as a doc: its extracted text is the body, and its
// title is the PDF's own (metadata or first line), else the file name. The
// hash is of the text, as for a PDF fetched by URL. A PDF over the size cap,
// one pdfium cannot read, and a scan with no text are errors with the reason.
func readPDFDoc(src Source, rel, p string) (doc, error) {
	info, err := os.Stat(p)
	if err != nil {
		return doc{}, err
	}
	if info.Size() > maxPDFBytes {
		return doc{}, errors.New("PDF is larger than 20 MiB")
	}
	// nosemgrep: go-path-traversal -- a file found by walking the folder the operator named; symlinks were skipped
	raw, err := os.ReadFile(p) //nolint:gosec // same
	if err != nil {
		return doc{}, err
	}
	text, pdfTitle, err := pdfText(context.Background(), raw)
	if err != nil {
		return doc{}, err
	}
	if pdfTitle == "" {
		pdfTitle = strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	}
	docPath := path.Join(src.Prefix, rel)
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
