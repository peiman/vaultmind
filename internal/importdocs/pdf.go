package importdocs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/rs/zerolog/log"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
)

// PDF text comes from pdfium compiled to WebAssembly and run by wazero: pure
// Go, so the binary stays self-contained, and a malformed PDF cannot reach
// process memory. Chosen by measurement against pdftotext on three papers:
// recall >= 0.955, where the pure-Go alternative recovered a tenth of the
// words (experiments/2026-10-02-pdf-extraction in the research repo).

// maxPDFBytes is the most of a PDF that is read; text pages keep maxPageBytes.
const maxPDFBytes = 20 << 20

// pdfTimeout bounds one extraction. A PDF that keeps pdfium busy longer is
// refused rather than allowed to hang the import. A var so a test can shorten
// it.
var pdfTimeout = 60 * time.Second

// pdfStartTimeout bounds starting pdfium: compiling the ~20 MB module takes
// seconds on a normal machine, far longer on a slow or instrumented one, and
// only once when the compile cache can be written.
const pdfStartTimeout = 5 * time.Minute

// pdfMemoryPages caps pdfium's memory at 1 GiB (64 KiB WebAssembly pages).
const pdfMemoryPages = 16384

// minPDFLetters is the least text a PDF must yield. Below it the PDF is a
// scan or images, and an empty note would look like a successful import.
const minPDFLetters = 50

// pdfiumCacheDir is where wazero keeps the compiled pdfium module (~20 MB).
// Compiling takes seconds; a cached module starts in about a tenth of one.
// Tests point it at a temp dir.
var pdfiumCacheDir = func() (string, error) {
	dir, err := xdg.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pdfium-wasm"), nil
}

// pdfText returns a PDF's text, pages separated by a blank line, and its
// title ("" when it has none worth using).
func pdfText(ctx context.Context, data []byte) (text, title string, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			text, title, err = "", "", fmt.Errorf("reading the PDF: %v", rec)
		}
	}()
	// Starting pdfium (compiling it, when no cached module can be used) and
	// reading the PDF have separate bounds. One bound for both timed out first
	// imports where compiling alone ran past it: under the race detector on CI
	// it took 64 s against 60.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	start := time.AfterFunc(pdfStartTimeout, cancel)
	pool, err := newPDFiumPool(ctx)
	if err != nil {
		start.Stop()
		return "", "", fmt.Errorf("reading the PDF: starting pdfium: %w", err)
	}
	defer func() { _ = pool.Close() }()
	inst, err := pool.GetInstance(pdfStartTimeout)
	if !start.Stop() && err == nil {
		err = fmt.Errorf("starting pdfium took longer than %s", pdfStartTimeout)
	}
	if err != nil {
		return "", "", fmt.Errorf("reading the PDF: %w", err)
	}
	defer func() { _ = inst.Close() }()
	read := time.AfterFunc(pdfTimeout, cancel)
	defer read.Stop()
	pages, meta, err := readPDF(inst, data)
	if err != nil {
		return "", "", fmt.Errorf("reading the PDF: %w", err)
	}
	text = cleanPDFText(pages)
	if letters(text) < minPDFLetters {
		return "", "", errors.New("no text layer (scanned PDF?)")
	}
	return text, pdfTitle(meta, firstLine(text)), nil
}

// newPDFiumPool starts pdfium with one worker and no filesystem. The empty
// FSConfig matters: left nil, go-pdfium mounts the host's "/" into the
// module, and a pdfium exploit in a crafted PDF could read any file the user
// can and carry it into the note's text. pdfium gets the PDF as bytes and
// needs no files.
func newPDFiumPool(ctx context.Context) (pdfium.Pool, error) {
	return webassembly.Init(webassembly.Config{
		Context: ctx, MinIdle: 1, MaxIdle: 1, MaxTotal: 1,
		RuntimeConfig: pdfiumRuntime(),
		FSConfig:      wazero.NewFSConfig(),
	})
}

// pdfiumRuntime keeps the features pdfium needs (a custom config replaces
// go-pdfium's default, and without exception handling the module does not
// compile), caps memory, stops on a cancelled context, and caches the
// compiled module when it can. A cache that cannot be opened costs a slower
// run, never a failed one.
func pdfiumRuntime() wazero.RuntimeConfig {
	cfg := wazero.NewRuntimeConfig().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).
		WithMemoryLimitPages(pdfMemoryPages).
		WithCloseOnContextDone(true)
	dir, err := pdfiumCacheDir()
	if err == nil {
		var cache wazero.CompilationCache
		if cache, err = wazero.NewCompilationCacheWithDir(dir); err == nil {
			return cfg.WithCompilationCache(cache)
		}
	}
	log.Debug().Err(err).Msg("pdfium compile cache unavailable; compiling uncached")
	return cfg
}

// readPDF returns each page's text and the document's Title metadata.
func readPDF(inst pdfium.Pdfium, data []byte) ([]string, string, error) {
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return nil, "", err
	}
	defer func() { _, _ = inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document}) }()
	count, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, "", err
	}
	pages := make([]string, 0, count.PageCount)
	for i := 0; i < count.PageCount; i++ {
		page, err := inst.GetPageText(&requests.GetPageText{Page: requests.Page{
			ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i},
		}})
		if err != nil {
			return nil, "", fmt.Errorf("page %d: %w", i+1, err)
		}
		pages = append(pages, page.Text)
	}
	meta := ""
	if m, err := inst.FPDF_GetMetaText(&requests.FPDF_GetMetaText{Document: doc.Document, Tag: "Title"}); err == nil {
		meta = m.Value
	}
	return pages, meta, nil
}

var (
	// hyphenBreak is a word broken across lines: "embed-\nding".
	hyphenBreak = regexp.MustCompile(`(\p{L})-\n(\p{Ll})`)
	blankRun    = regexp.MustCompile(`\n{3,}`)
)

// pdfiumLineHyphen is how pdfium reports a hyphen at a line end: the line
// break is removed and this character stands where the hyphen was, so
// "embed-ding" arrives as "embed\x02ding". Dropping it rejoins the word.
const pdfiumLineHyphen = "\x02"

// cleanPDFText joins pages with a blank line, rejoins words hyphenated across
// lines, drops control characters, trims trailing space from each line, and
// collapses runs of blank lines. Nothing is reflowed beyond that: guessing
// paragraphs mis-joins headings, captions and references.
func cleanPDFText(pages []string) string {
	var kept []string
	for _, p := range pages {
		p = strings.ReplaceAll(p, pdfiumLineHyphen, "")
		p = strings.ReplaceAll(p, "\r\n", "\n")
		p = strings.ReplaceAll(p, "\r", "\n")
		p = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return -1
			}
			return r
		}, p)
		lines := strings.Split(p, "\n")
		for i, l := range lines {
			lines[i] = strings.TrimRightFunc(l, unicode.IsSpace)
		}
		p = strings.TrimSpace(strings.Join(lines, "\n"))
		if p != "" {
			kept = append(kept, p)
		}
	}
	text := strings.Join(kept, "\n\n")
	text = hyphenBreak.ReplaceAllString(text, "$1$2")
	text = blankRun.ReplaceAllString(text, "\n\n")
	return normalizeMarkdown(text)
}

// placeholderTitle matches the Title metadata that names a file or a tool
// rather than the document.
var placeholderTitle = regexp.MustCompile(`(?i)^(untitled\b.*|microsoft (word|powerpoint) - .*|.*\.(pdf|docx?|tex|dvi))$`)

// pdfTitle is the Title metadata when it names the document, else the first
// line of text when it is the length of a title, else "".
func pdfTitle(meta, first string) string {
	if m := strings.TrimSpace(meta); m != "" && !placeholderTitle.MatchString(m) {
		return m
	}
	first = strings.TrimSpace(first)
	if n := utf8.RuneCountInString(first); n >= 3 && n <= 200 {
		return first
	}
	return ""
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

func letters(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n
}
