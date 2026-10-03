package importdocs

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Office files (docx, pptx, xlsx) are zips of XML parts. They are read with
// the standard library only: no converter service, no new dependency.

const (
	// maxOfficeParts bounds the entries a file may hold, and maxPartBytes the
	// size one part may inflate to: a zip bomb is refused, not expanded.
	maxOfficeParts = 10000
	maxPartBytes   = 64 << 20
	// maxSheetRows and maxSheetCols bound how much of one sheet a note shows.
	maxSheetRows = 200
	maxSheetCols = 50
)

// officeKinds are the Office formats an import reads, by extension.
var officeKinds = map[string]bool{"docx": true, "pptx": true, "xlsx": true}

// maxOfficeBytes is the largest Office file an import opens. It is far above
// the PDF cap: a deck's size is its pictures, and only the XML parts (each
// capped at maxPartBytes) are read, straight from the file.
const maxOfficeBytes = 512 << 20

// officeTextFrom turns an Office file into markdown and its title ("" when
// the file names none). It reads through r, so a large file is never read
// whole.
func officeTextFrom(kind string, r io.ReaderAt, size int64) (string, string, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return "", "", fmt.Errorf("not a readable %s file: %w", kind, err)
	}
	if len(zr.File) > maxOfficeParts {
		return "", "", fmt.Errorf("%s holds more than %d parts", kind, maxOfficeParts)
	}
	parts := officeParts{files: map[string]*zip.File{}}
	for _, f := range zr.File {
		parts.files[f.Name] = f
	}
	var text string
	switch kind {
	case "docx":
		text, err = docxText(parts)
	case "pptx":
		text, err = pptxText(parts)
	case "xlsx":
		text, err = xlsxText(parts)
	default:
		return "", "", fmt.Errorf("not an Office format: %s", kind)
	}
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(text) + "\n", parts.title(), nil
}

type officeParts struct{ files map[string]*zip.File }

// read returns a part, refusing one that inflates past maxPartBytes.
func (o officeParts) read(name string) ([]byte, error) {
	f, ok := o.files[name]
	if !ok {
		return nil, fmt.Errorf("missing part %s", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, maxPartBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxPartBytes {
		return nil, fmt.Errorf("part %s is larger than %d MiB uncompressed", name, maxPartBytes>>20)
	}
	return b, nil
}

// title is the document title from docProps/core.xml, "" when unset.
func (o officeParts) title() string {
	b, err := o.read("docProps/core.xml")
	if err != nil {
		return ""
	}
	var core struct {
		Title string `xml:"title"`
	}
	if xml.Unmarshal(b, &core) != nil {
		return ""
	}
	return strings.TrimSpace(core.Title)
}

// rels maps relationship ids to part names for the part at owner.
func (o officeParts) rels(owner string) map[string]string {
	dir, base := path.Split(owner)
	b, err := o.read(dir + "_rels/" + base + ".rels")
	if err != nil {
		return nil
	}
	var r struct {
		Rel []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if xml.Unmarshal(b, &r) != nil {
		return nil
	}
	out := map[string]string{}
	for _, x := range r.Rel {
		t := x.Target
		if strings.HasPrefix(t, "/") {
			t = strings.TrimPrefix(t, "/")
		} else {
			t = path.Join(dir, t)
		}
		out[x.ID] = t
	}
	return out
}

// attr returns the attribute with the local name, whatever its namespace.
func attr(se xml.StartElement, local string) string {
	for _, a := range se.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// ---- docx ----

var headingName = regexp.MustCompile(`^heading ([1-6])$`)

// docxHeadingLevels maps style ids to heading levels through styles.xml: the
// built-in names are English ("heading 1", "title") whatever the id is
// called in the document's language.
func docxHeadingLevels(o officeParts) map[string]int {
	levels := map[string]int{}
	b, err := o.read("word/styles.xml")
	if err != nil {
		return levels
	}
	var s struct {
		Styles []struct {
			ID   string `xml:"styleId,attr"`
			Name struct {
				Val string `xml:"val,attr"`
			} `xml:"name"`
		} `xml:"style"`
	}
	if xml.Unmarshal(b, &s) != nil {
		return levels
	}
	for _, st := range s.Styles {
		name := strings.ToLower(st.Name.Val)
		if m := headingName.FindStringSubmatch(name); m != nil {
			levels[st.ID], _ = strconv.Atoi(m[1])
		} else if name == "title" {
			levels[st.ID] = 1
		}
	}
	return levels
}

func docxText(o officeParts) (string, error) {
	b, err := o.read("word/document.xml")
	if err != nil {
		return "", errors.New("not a Word document: no word/document.xml")
	}
	levels := docxHeadingLevels(o)
	var out strings.Builder
	var para strings.Builder
	var style string
	var list, inText bool
	tableDepth := 0
	var table [][]string
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading word/document.xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				para.Reset()
				style, list = "", false
			case "pStyle":
				style = attr(t, "val")
			case "numPr":
				list = true
			case "t":
				inText = true
			case "tab":
				para.WriteString(" ")
			case "br":
				para.WriteString(" ")
			case "tbl":
				tableDepth++
				if tableDepth == 1 {
					table = nil
				}
			case "tr":
				if tableDepth == 1 {
					table = append(table, nil)
				}
			case "tc":
				if tableDepth == 1 && len(table) > 0 {
					table[len(table)-1] = append(table[len(table)-1], "")
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				line := strings.TrimSpace(para.String())
				if tableDepth > 0 {
					appendCell(table, line)
					continue
				}
				if line == "" {
					continue
				}
				switch {
				case levels[style] > 0:
					fmt.Fprintf(&out, "%s %s\n\n", strings.Repeat("#", levels[style]), line)
				case list:
					fmt.Fprintf(&out, "- %s\n", line)
				default:
					out.WriteString(line + "\n\n")
				}
			case "tbl":
				tableDepth--
				if tableDepth == 0 {
					out.WriteString(markdownTable(table))
				}
			}
		case xml.CharData:
			if inText {
				para.Write(t)
			}
		}
	}
	return out.String(), nil
}

// appendCell adds a paragraph's text to the last cell of the table.
func appendCell(table [][]string, text string) {
	if text == "" || len(table) == 0 {
		return
	}
	row := table[len(table)-1]
	if len(row) == 0 {
		return
	}
	if row[len(row)-1] != "" {
		text = row[len(row)-1] + " " + text
	}
	row[len(row)-1] = text
}

// markdownTable renders rows as a markdown table, the first row as its
// header.
func markdownTable(rows [][]string) string {
	width := 0
	for _, r := range rows {
		width = max(width, len(r))
	}
	if width == 0 {
		return ""
	}
	cell := func(s string) string {
		s = strings.Join(strings.Fields(s), " ")
		return strings.ReplaceAll(s, "|", `\|`)
	}
	line := func(r []string) string {
		cells := make([]string, width)
		for i := range cells {
			if i < len(r) {
				cells[i] = cell(r[i])
			}
		}
		return "| " + strings.Join(cells, " | ") + " |\n"
	}
	var b strings.Builder
	b.WriteString(line(rows[0]))
	b.WriteString("|" + strings.Repeat(" --- |", width) + "\n")
	for _, r := range rows[1:] {
		b.WriteString(line(r))
	}
	return b.String() + "\n"
}

// ---- pptx ----

func pptxText(o officeParts) (string, error) {
	b, err := o.read("ppt/presentation.xml")
	if err != nil {
		return "", errors.New("not a PowerPoint file: no ppt/presentation.xml")
	}
	var pres struct {
		Slides []struct {
			RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sldIdLst>sldId"`
	}
	if err := xml.Unmarshal(b, &pres); err != nil {
		return "", fmt.Errorf("reading ppt/presentation.xml: %w", err)
	}
	rels := o.rels("ppt/presentation.xml")
	var out strings.Builder
	for i, s := range pres.Slides {
		part, ok := rels[s.RID]
		if !ok {
			continue
		}
		raw, err := o.read(part)
		if err != nil {
			continue
		}
		fmt.Fprintf(&out, "## Slide %d\n\n", i+1)
		for _, line := range drawingText(raw, nil) {
			out.WriteString(line + "\n\n")
		}
		for _, target := range o.rels(part) {
			if !strings.Contains(target, "notesSlide") {
				continue
			}
			if nb, err := o.read(target); err == nil {
				for _, line := range drawingText(nb, map[string]bool{"body": true}) {
					out.WriteString("> " + line + "\n")
				}
				out.WriteString("\n")
			}
		}
	}
	return out.String(), nil
}

// drawingText returns the paragraphs of a slide part, one line each. With
// only set, it keeps shapes whose placeholder type is in it (a notes page
// holds the slide number and a picture of the slide besides the notes).
func drawingText(raw []byte, only map[string]bool) []string {
	var lines []string
	var para strings.Builder
	var inText bool
	keep := only == nil
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "sp":
				keep = only == nil
			case "ph":
				if only != nil {
					keep = only[attr(t, "type")]
				}
			case "p":
				para.Reset()
			case "t":
				inText = true
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				if line := strings.TrimSpace(para.String()); line != "" && keep {
					lines = append(lines, line)
				}
			}
		case xml.CharData:
			if inText {
				para.Write(t)
			}
		}
	}
	return lines
}

// ---- xlsx ----

func xlsxText(o officeParts) (string, error) {
	b, err := o.read("xl/workbook.xml")
	if err != nil {
		return "", errors.New("not an Excel file: no xl/workbook.xml")
	}
	var wb struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := xml.Unmarshal(b, &wb); err != nil {
		return "", fmt.Errorf("reading xl/workbook.xml: %w", err)
	}
	shared := sharedStrings(o)
	rels := o.rels("xl/workbook.xml")
	var out strings.Builder
	for _, s := range wb.Sheets {
		part, ok := rels[s.RID]
		if !ok {
			continue
		}
		raw, err := o.read(part)
		if err != nil {
			continue
		}
		sh := readSheet(raw, shared)
		if len(sh.rows) == 0 {
			continue
		}
		fmt.Fprintf(&out, "## %s\n\n%s", s.Name, markdownTable(sh.rows))
		if sh.moreRows > 0 {
			fmt.Fprintf(&out, "(%d more rows not shown)\n\n", sh.moreRows)
		}
		if sh.moreCols > 0 {
			fmt.Fprintf(&out, "(%d more columns not shown)\n\n", sh.moreCols)
		}
	}
	return out.String(), nil
}

func sharedStrings(o officeParts) []string {
	b, err := o.read("xl/sharedStrings.xml")
	if err != nil {
		return nil
	}
	var out []string
	var cur strings.Builder
	var inT bool
	phonetic := 0 // inside rPh: a reading guide, not the text
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				cur.Reset()
			case "t":
				inT = true
			case "rPh":
				phonetic++
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inT = false
			case "rPh":
				phonetic--
			case "si":
				out = append(out, cur.String())
			}
		case xml.CharData:
			if inT && phonetic == 0 {
				cur.Write(t)
			}
		}
	}
	return out
}

// sheet is what a note shows of one worksheet: at most maxSheetRows+1 rows
// (the header and the body) of at most maxSheetCols columns, and how much
// more there was.
type sheet struct {
	rows               [][]string
	moreRows, moreCols int
}

// excelMaxCols is the widest a sheet can be (column XFD). A reference past it
// is malformed and counts no further.
const excelMaxCols = 16384

// readSheet reads a worksheet, placing each cell in its column so an empty
// cell between two values keeps its place. A cell with no value (formatting
// only) adds nothing, and a row with no values is dropped: real sheets style
// cells far past their data. Nothing past the caps is stored, so what a
// sheet costs is bounded by the caps, not by what its XML claims.
func readSheet(raw []byte, shared []string) sheet {
	var sh sheet
	var row []string
	var col, next int
	var kind string
	var val strings.Builder
	var inV bool
	endRow := func() {
		if len(row) == 0 {
			return
		}
		if len(sh.rows) <= maxSheetRows {
			sh.rows = append(sh.rows, row)
		} else {
			sh.moreRows++
		}
		row = nil
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				row, next = nil, 0
			case "c":
				col, kind = columnIndex(attr(t, "r")), attr(t, "t")
				if col < 0 {
					col = next // no reference: the next column
				}
				next = col + 1
				val.Reset()
			case "v", "t":
				inV = true
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v", "t":
				inV = false
			case "row":
				endRow()
			case "c":
				v := val.String()
				if kind == "s" {
					if i, err := strconv.Atoi(v); err == nil && i >= 0 && i < len(shared) {
						v = shared[i]
					}
				}
				if strings.TrimSpace(v) == "" {
					continue
				}
				if col >= maxSheetCols {
					sh.moreCols = max(sh.moreCols, min(col+1, excelMaxCols)-maxSheetCols)
					continue
				}
				for len(row) <= col {
					row = append(row, "")
				}
				row[col] = v
			}
		case xml.CharData:
			if inV {
				val.Write(t)
			}
		}
	}
	return sh
}

// columnIndex turns a cell reference's letters (A1, AB12) into a 0-based
// column; -1 when there are none. A run past Excel's widest column stops
// counting there rather than overflowing.
func columnIndex(ref string) int {
	n := 0
	for _, c := range ref {
		if c < 'A' || c > 'Z' {
			break
		}
		n = n*26 + int(c-'A'+1)
		if n > excelMaxCols {
			return excelMaxCols
		}
	}
	return n - 1
}
