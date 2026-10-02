package importdocs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klippa-app/go-pdfium/requests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain points the pdfium compile cache at one temp dir for the run: the
// real one is the user's cache, and an uncached compile costs seconds per test.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "vm-pdfium-cache")
	if err != nil {
		panic(err)
	}
	pdfiumCacheDir = func() (string, error) { return dir, nil }
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name)) //nolint:gosec // test fixture path
	require.NoError(t, err)
	return b
}

func TestPDFText_PagesAndTheMetadataTitle(t *testing.T) {
	text, title, err := pdfText(t.Context(), fixture(t, "paper.pdf"))
	require.NoError(t, err)
	assert.Equal(t, "Retrieval for Agent Memory: A Study", title)
	assert.Contains(t, text, "We study embedding long notes", "a hyphen broken across lines is joined")
	assert.Contains(t, text, "Sparse retrieval beats dense retrieval")
	pages := strings.Index(text, "Results")
	require.Positive(t, pages)
	assert.Contains(t, text[:pages], "\n\n", "pages are separated by a blank line")
}

func TestPDFText_APlaceholderTitleFallsBackToTheFirstLine(t *testing.T) {
	_, title, err := pdfText(t.Context(), fixture(t, "placeholder-title.pdf"))
	require.NoError(t, err)
	assert.Equal(t, "Spreading Activation in Practice", title)
}

// An empty note would look like a successful import of a scanned PDF.
func TestPDFText_NoTextLayerIsRefused(t *testing.T) {
	_, _, err := pdfText(t.Context(), fixture(t, "no-text.pdf"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no text layer (scanned PDF?)")
}

func TestPDFText_NotAPDFIsAnError(t *testing.T) {
	_, _, err := pdfText(t.Context(), []byte("%PDF-1.4 but nothing else"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading the PDF")
}

// A cache that cannot be used costs a slower run, never a failed one.
func TestPDFText_WorksWithoutTheCompileCache(t *testing.T) {
	saved := pdfiumCacheDir
	t.Cleanup(func() { pdfiumCacheDir = saved })
	pdfiumCacheDir = func() (string, error) { return "", os.ErrPermission }
	text, _, err := pdfText(t.Context(), fixture(t, "paper.pdf"))
	require.NoError(t, err)
	assert.Contains(t, text, "Sparse retrieval")
}

func TestPDFText_StopsWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	_, _, err := pdfText(ctx, fixture(t, "paper.pdf"))
	require.Error(t, err)
}

// pdfium gets the PDF as bytes and needs no files. go-pdfium mounts the host's
// "/" into the module unless told otherwise, which would let a pdfium exploit
// in a crafted PDF read ~/.ssh and write the result into the note.
func TestPDFium_CannotReachTheHostFilesystem(t *testing.T) {
	pool, err := newPDFiumPool(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	inst, err := pool.GetInstance(time.Minute)
	require.NoError(t, err)
	t.Cleanup(func() { _ = inst.Close() })
	path, err := filepath.Abs(filepath.Join("testdata", "paper.pdf"))
	require.NoError(t, err)
	doc, err := inst.OpenDocument(&requests.OpenDocument{FilePath: &path})
	if err == nil {
		_, _ = inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	}
	require.Error(t, err, "pdfium opened a host file by path")
}

func TestCleanPDFText(t *testing.T) {
	got := cleanPDFText([]string{
		"Title  \nWe study embed-\nding notes.\nA well-\nKnown term.\n\n\n\n\nEnd",
		"  \n",
		"Page two",
	})
	assert.Equal(t, "Title\nWe study embedding notes.\nA well-\nKnown term.\n\nEnd\n\nPage two\n", got,
		"hyphen joined only before a lowercase letter; blank runs collapse; empty pages dropped")
}

// pdfium reports a hyphen at a line end as U+0002 and removes the break, so
// "embed-ding" arrives as "embed\x02ding". Other control characters are not
// text either.
func TestCleanPDFText_PdfiumLineEndHyphenAndControls(t *testing.T) {
	got := cleanPDFText([]string{"We study embed\x02ding long\x00 notes.\tTab kept.\fForm feed"})
	assert.Equal(t, "We study embedding long notes.\tTab kept.Form feed\n", got)
}

func TestPDFTitle(t *testing.T) {
	for _, c := range []struct{ meta, first, want string }{
		{"Late Chunking", "ignored", "Late Chunking"},
		{"  ", "Late Chunking: Contextual Chunk Embeddings", "Late Chunking: Contextual Chunk Embeddings"},
		{"untitled", "First Line", "First Line"},
		{"Untitled Document", "First Line", "First Line"},
		{"Microsoft Word - draft.docx", "First Line", "First Line"},
		{"paper-final.pdf", "First Line", "First Line"},
		{"notes.doc", "First Line", "First Line"},
		{"", "ab", ""},
		{"", strings.Repeat("x", 201), ""},
	} {
		assert.Equal(t, c.want, pdfTitle(c.meta, c.first), "meta %q first %q", c.meta, c.first)
	}
}
