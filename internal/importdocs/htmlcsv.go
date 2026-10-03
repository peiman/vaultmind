package importdocs

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// localBase is the base a local HTML file's relative links are resolved
// against. Readability makes every link absolute; this host is then turned
// back into a path from the source root, so a note never points at a web
// host it was not from, nor at the machine's own folders.
const localBase = "https://local.invalid/"

// readHTMLText reads a local HTML file of at most 5 MiB the way a fetched
// page is read: its <meta charset>, readability with the <main> fallback,
// and the leading navigation dropped. docPath is the file's path from the
// source root.
func readHTMLText(docPath, p string) (string, string, error) {
	raw, err := readCapped(p, maxPageBytes, "HTML file is larger than 5 MiB")
	if err != nil {
		return "", "", err
	}
	decoded, err := decodeCharset(raw, "text/html")
	if err != nil {
		return "", "", err
	}
	md, title, err := safeHTMLMarkdown(string(decoded), localBase+docPath)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(md) == "" {
		return "", "", errors.New("the html file holds no text")
	}
	return strings.ReplaceAll(md, localBase, "/"), title, nil
}

// readCapped reads a whole file, refusing one over limit bytes.
func readCapped(p string, limit int64, tooBig string) ([]byte, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, errors.New(tooBig)
	}
	// nosemgrep: go-path-traversal -- a file found by walking the folder the operator named; symlinks were skipped
	return os.ReadFile(p) //nolint:gosec // same
}

// maxCSVBytes caps the CSV a note is made from, as for an Office file. Only
// the first rows are kept; the rest is counted as it streams past.
const maxCSVBytes = maxOfficeBytes

// csvSniffBytes is how much of a CSV is read to tell its encoding and
// delimiter.
const csvSniffBytes = 64 << 10

// readCSVText makes a CSV or TSV a markdown table, as an Excel sheet is:
// the first row as the header, at most maxSheetRows rows and maxSheetCols
// columns shown, and what is left out said.
func readCSVText(kind, p string) (string, string, error) {
	// nosemgrep: go-path-traversal -- a file found by walking the folder the operator named; symlinks were skipped
	f, err := os.Open(p) //nolint:gosec // same
	if err != nil {
		return "", "", err
	}
	defer func() { _ = f.Close() }()
	if info, err := f.Stat(); err != nil || info.Size() > maxCSVBytes {
		return "", "", fmt.Errorf("%s file is larger than %d MiB", kind, maxCSVBytes>>20)
	}
	r, head, err := csvReader(f)
	if err != nil {
		return "", "", err
	}
	comma := '\t'
	if kind == "csv" {
		comma = sniffDelimiter(head)
	}
	sh, err := readCSVRows(r, comma)
	if err != nil {
		return "", "", err
	}
	if len(sh.rows) == 0 {
		return "", "", fmt.Errorf("the %s file holds no text", kind)
	}
	return sheetMarkdown(sh), "", nil
}

// csvReader is f as UTF-8 without a byte-order mark: a file whose start is
// not valid UTF-8 is read as Windows-1252, what Excel writes. head is the
// decoded start, for sniffing.
func csvReader(f io.Reader) (io.Reader, []byte, error) {
	br := bufio.NewReaderSize(f, csvSniffBytes)
	peek, err := br.Peek(csvSniffBytes)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return nil, nil, err
	}
	var r io.Reader = br
	if !validUTF8Prefix(peek) {
		r = charmap.Windows1252.NewDecoder().Reader(br)
		peek, _ = charmap.Windows1252.NewDecoder().Bytes(peek)
	}
	bom := []byte("\ufeff")
	if bytes.HasPrefix(peek, bom) {
		if _, err := io.ReadFull(r, make([]byte, len(bom))); err != nil {
			return nil, nil, err
		}
		peek = peek[len(bom):]
	}
	return r, peek, nil
}

// validUTF8Prefix reports b valid UTF-8, allowing a character cut off at
// its end by the peek.
func validUTF8Prefix(b []byte) bool {
	for cut := 0; cut < utf8.UTFMax && cut <= len(b); cut++ {
		if utf8.Valid(b[:len(b)-cut]) {
			return true
		}
	}
	return false
}

// sniffDelimiter picks the delimiter that splits the first lines into the
// same number of fields most often, needing more than one field: a comma
// wins a tie, so a one-column file stays one column.
func sniffDelimiter(head []byte) rune {
	best, bestScore := ',', 0
	for _, c := range []rune{',', ';', '\t'} {
		if score := consistentRows(head, c); score > bestScore {
			best, bestScore = c, score
		}
	}
	return best
}

// consistentRows counts how many of the first 20 records have the most
// common field count, when that count is above one.
func consistentRows(head []byte, comma rune) int {
	r := csv.NewReader(bytes.NewReader(head))
	r.Comma, r.LazyQuotes, r.FieldsPerRecord = comma, true, -1
	counts := map[int]int{}
	for i := 0; i < 20; i++ {
		rec, err := r.Read()
		if err != nil {
			break
		}
		counts[len(rec)]++
	}
	best := 0
	for fields, n := range counts {
		if fields > 1 && n > best {
			best = n
		}
	}
	return best
}

// readCSVRows keeps the header and maxSheetRows rows of at most
// maxSheetCols columns, and counts the rest. A row of empty fields is
// dropped, as an empty sheet row is.
func readCSVRows(src io.Reader, comma rune) (sheet, error) {
	r := csv.NewReader(src)
	r.Comma, r.LazyQuotes, r.FieldsPerRecord, r.ReuseRecord = comma, true, -1, true
	var sh sheet
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			return sh, nil
		}
		if err != nil {
			return sh, fmt.Errorf("reading the CSV: %w", err)
		}
		if blankRecord(rec) {
			continue
		}
		sh.moreCols = max(sh.moreCols, len(rec)-maxSheetCols)
		if len(sh.rows) > maxSheetRows {
			sh.moreRows++
			continue
		}
		sh.rows = append(sh.rows, append([]string(nil), rec[:min(len(rec), maxSheetCols)]...))
	}
}

func blankRecord(rec []string) bool {
	for _, f := range rec {
		if strings.TrimSpace(f) != "" {
			return false
		}
	}
	return true
}

// sheetMarkdown is a sheet's table and what it leaves out.
func sheetMarkdown(sh sheet) string {
	var out strings.Builder
	out.WriteString(markdownTable(sh.rows))
	if sh.moreRows > 0 {
		fmt.Fprintf(&out, "(%d more rows not shown)\n\n", sh.moreRows)
	}
	if sh.moreCols > 0 {
		fmt.Fprintf(&out, "(%d more columns not shown)\n\n", sh.moreCols)
	}
	return out.String()
}
