package importdocs

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// officeText reads an Office file held in memory.
func officeText(kind string, raw []byte) (string, string, error) {
	return officeTextFrom(kind, bytes.NewReader(raw), int64(len(raw)))
}

// ooxml zips parts into an Office file, so fixtures are readable in the test
// rather than opaque binaries in testdata.
func ooxml(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	var errs []error
	for name, body := range parts {
		w, err := zw.Create(name)
		if err == nil {
			_, err = w.Write([]byte(body))
		}
		errs = append(errs, err)
	}
	require.NoError(t, errors.Join(append(errs, zw.Close())...))
	return buf.Bytes()
}

const (
	nsW   = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`
	nsA   = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	nsX   = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	nsRel = `xmlns="http://schemas.openxmlformats.org/package/2006/relationships"`
)

func core(title string) string {
	return `<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>` + title + `</dc:title></cp:coreProperties>`
}

func para(style, text string, list bool) string {
	ppr := ""
	if style != "" || list {
		ppr = "<w:pPr>"
		if style != "" {
			ppr += `<w:pStyle w:val="` + style + `"/>`
		}
		if list {
			ppr += `<w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr>`
		}
		ppr += "</w:pPr>"
	}
	return "<w:p>" + ppr + "<w:r><w:t>" + text + "</w:t></w:r></w:p>"
}

// A Word document keeps its structure: styles resolve to headings through
// styles.xml (a Swedish document names them Rubrik1), lists stay lists, a
// table becomes a markdown table, and the title comes from the properties.
func TestOffice_DocxKeepsHeadingsListsAndTables(t *testing.T) {
	styles := `<w:styles ` + nsW + `><w:style w:styleId="Rubrik1"><w:name w:val="heading 1"/></w:style><w:style w:styleId="Rubrik2"><w:name w:val="heading 2"/></w:style></w:styles>`
	body := para("Rubrik1", "Overview", false) +
		para("", "Plain text here.", false) +
		para("", "first item", true) +
		para("Rubrik2", "Details", false) +
		`<w:tbl><w:tr><w:tc>` + para("", "Name", false) + `</w:tc><w:tc>` + para("", "Value", false) + `</w:tc></w:tr>` +
		`<w:tr><w:tc>` + para("", "alpha", false) + `</w:tc><w:tc>` + para("", "a|b", false) + `</w:tc></w:tr></w:tbl>`
	raw := ooxml(t, map[string]string{
		"word/document.xml": `<w:document ` + nsW + `><w:body>` + body + `</w:body></w:document>`,
		"word/styles.xml":   styles,
		"docProps/core.xml": core("Project Plan"),
	})

	text, title, err := officeText("docx", raw)
	require.NoError(t, err)
	assert.Equal(t, "Project Plan", title)
	for _, want := range []string{"# Overview", "Plain text here.", "- first item", "## Details", "| Name | Value |", "| --- | --- |", `| alpha | a\|b |`} {
		assert.Contains(t, text, want)
	}
	assert.Less(t, strings.Index(text, "# Overview"), strings.Index(text, "## Details"), "document order is kept")
}

// Slides come in presentation order (from presentation.xml, not file names),
// each under its own heading, with its speaker notes.
func TestOffice_PptxSlidesInOrderWithNotes(t *testing.T) {
	slide := func(text string) string {
		return `<p:sld ` + nsA + `><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`
	}
	notes := `<p:notes ` + nsA + `><p:cSld><p:spTree><p:sp><p:nvSpPr><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>say this aloud</a:t></a:r></a:p></p:txBody></p:sp>` +
		`<p:sp><p:nvSpPr><p:nvPr><p:ph type="sldNum"/></p:nvPr></p:nvSpPr><p:txBody><a:p><a:r><a:t>7</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:notes>`
	raw := ooxml(t, map[string]string{
		"ppt/presentation.xml":             `<p:presentation ` + nsA + `><p:sldIdLst><p:sldId id="256" r:id="rId2"/><p:sldId id="257" r:id="rId1"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels":  `<Relationships ` + nsRel + `><Relationship Id="rId1" Target="slides/slide1.xml"/><Relationship Id="rId2" Target="slides/slide2.xml"/></Relationships>`,
		"ppt/slides/slide1.xml":            slide("Second in the deck"),
		"ppt/slides/slide2.xml":            slide("Opening slide"),
		"ppt/slides/_rels/slide2.xml.rels": `<Relationships ` + nsRel + `><Relationship Id="rId9" Target="../notesSlides/notesSlide1.xml"/></Relationships>`,
		"ppt/notesSlides/notesSlide1.xml":  notes,
		"docProps/core.xml":                core(""),
	})

	text, _, err := officeText("pptx", raw)
	require.NoError(t, err)
	assert.Less(t, strings.Index(text, "Opening slide"), strings.Index(text, "Second in the deck"), "presentation order, not file order")
	assert.Contains(t, text, "## Slide 1")
	assert.Contains(t, text, "## Slide 2")
	assert.Contains(t, text, "> say this aloud")
	assert.NotContains(t, text, "> 7", "the slide-number placeholder is not a note")
}

// A spreadsheet becomes one table per sheet, shared strings resolved, with a
// row cap that says how much was left out.
func TestOffice_XlsxSheetsAsTablesWithARowCap(t *testing.T) {
	var rows strings.Builder
	rows.WriteString(`<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c></row>`)
	for i := 2; i <= maxSheetRows+5; i++ {
		fmt.Fprintf(&rows, `<row r="%d"><c r="A%d" t="inlineStr"><is><t>item %d</t></is></c><c r="C%d"><v>%d</v></c></row>`, i, i, i, i, i)
	}
	raw := ooxml(t, map[string]string{
		"xl/workbook.xml":            `<workbook ` + nsX + `><sheets><sheet name="Budget" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships ` + nsRel + `><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst ` + nsX + `><si><t>Item</t></si><si><r><t>Co</t></r><r><t>st</t></r></si></sst>`,
		"xl/worksheets/sheet1.xml":   `<worksheet ` + nsX + `><sheetData>` + rows.String() + `</sheetData></worksheet>`,
	})

	text, _, err := officeText("xlsx", raw)
	require.NoError(t, err)
	assert.Contains(t, text, "## Budget")
	assert.Contains(t, text, "| Item | Cost |  |", "shared strings, rich runs joined; the header spans the used columns")
	assert.Contains(t, text, "| item 2 |  | 2 |", "an empty cell keeps its column")
	assert.NotContains(t, text, fmt.Sprintf("item %d |", maxSheetRows+5))
	assert.Contains(t, text, "more rows not shown")
}

// Real sheets carry formatting far past their data: a styled empty cell must
// not widen the table, an empty row is not a row, and a sheet that really is
// wide shows its first columns and says how many more there were (found on
// real files: one note had 1,024 columns and 641,662 words).
func TestOffice_XlsxIgnoresStyledEmptyCellsAndCapsColumns(t *testing.T) {
	var wide strings.Builder
	wide.WriteString(`<row r="1">`)
	for c := 0; c < maxSheetCols+3; c++ {
		fmt.Fprintf(&wide, `<c r="%s1" t="inlineStr"><is><t>h%d</t></is></c>`, columnName(c), c)
	}
	wide.WriteString(`</row>`)
	raw := ooxml(t, map[string]string{
		"xl/workbook.xml":            `<workbook ` + nsX + `><sheets><sheet name="Styled" sheetId="1" r:id="rId1"/><sheet name="Wide" sheetId="2" r:id="rId2"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships ` + nsRel + `><Relationship Id="rId1" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Target="worksheets/sheet2.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml": `<worksheet ` + nsX + `><sheetData>` +
			`<row r="1"><c r="A1" t="inlineStr"><is><t>only</t></is></c><c r="AMJ1" s="3"/></row>` +
			`<row r="2"><c r="B2" s="3"/></row>` +
			`<row r="3"><c r="A3"><v>42</v></c></row></sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml": `<worksheet ` + nsX + `><sheetData>` + wide.String() + `</sheetData></worksheet>`,
	})

	text, _, err := officeText("xlsx", raw)
	require.NoError(t, err)
	assert.Contains(t, text, "## Styled\n\n| only |\n| --- |\n| 42 |\n", "one column, the empty row dropped")
	assert.Contains(t, text, "3 more columns not shown")
	assert.NotContains(t, text, fmt.Sprintf("h%d", maxSheetCols))
}

// columnName is columnIndex's inverse: 0 → A, 26 → AA.
func columnName(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}

func TestOffice_ACorruptFileIsAnError(t *testing.T) {
	_, _, err := officeText("docx", []byte("not a zip"))
	require.Error(t, err)
	_, _, err = officeText("docx", ooxml(t, map[string]string{"other.xml": "<x/>"}))
	require.Error(t, err, "a zip that is not a Word document")
}
