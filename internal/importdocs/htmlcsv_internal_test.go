package importdocs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingReader yields data, then err.
type failingReader struct {
	data string
	err  error
}

func (f *failingReader) Read(p []byte) (int, error) {
	if f.data == "" {
		return 0, f.err
	}
	n := copy(p, f.data)
	f.data = f.data[n:]
	return n, nil
}

func TestCSVReading_AReadErrorIsReportedNotSwallowed(t *testing.T) {
	boom := errors.New("disk gone")
	_, _, err := csvReader(&failingReader{err: boom})
	require.ErrorIs(t, err, boom, "while sniffing")

	_, err = readCSVRows(&failingReader{data: "a,b\n1,2\n", err: boom}, ',')
	require.ErrorIs(t, err, boom, "while reading rows")
}

func TestReadCSVText_RefusesAFileOverTheCap(t *testing.T) {
	prev := maxCSVBytes
	t.Cleanup(func() { maxCSVBytes = prev })
	maxCSVBytes = 10
	p := filepath.Join(t.TempDir(), "big.csv")
	require.NoError(t, os.WriteFile(p, []byte("name,city\nAda,London\n"), 0o600))
	_, _, err := readCSVText("csv", p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "csv file is larger than")
}

func TestReadCSVText_AMissingFileIsAnError(t *testing.T) {
	_, _, err := readCSVText("csv", filepath.Join(t.TempDir(), "gone.csv"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestReadCSVText_AFileOfEmptyCellsHoldsNoText(t *testing.T) {
	p := filepath.Join(t.TempDir(), "empty.csv")
	require.NoError(t, os.WriteFile(p, []byte(",,\n , \n"), 0o600))
	_, _, err := readCSVText("csv", p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "holds no text")
}

func TestReadHTMLText_RefusesAFileOverTheCapAndAConverterPanic(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.html")
	require.NoError(t, os.WriteFile(big, []byte(strings.Repeat("a", maxPageBytes+1)), 0o600))
	_, _, err := readHTMLText("docs/big.html", big)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "larger than 5 MiB")

	_, _, err = readHTMLText("docs/missing.html", filepath.Join(dir, "missing.html"))
	require.Error(t, err)

	prev := htmlConverter
	t.Cleanup(func() { htmlConverter = prev })
	htmlConverter = func(string, string) (string, string, error) { panic("parser blew up") }
	ok := filepath.Join(dir, "ok.html")
	require.NoError(t, os.WriteFile(ok, []byte("<p>text</p>"), 0o600))
	_, _, err = readHTMLText("docs/ok.html", ok)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parser blew up")
}

var _ io.Reader = (*failingReader)(nil)
