package importdocs_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/peiman/vaultmind/internal/xdg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pixels is a w×h image of one colour.
func pixels(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 220, B: 240, A: 255})
		}
	}
	return img
}

// jpegWithXMP is a w×h JPEG carrying xmp as its XMP packet.
func jpegWithXMP(t *testing.T, w, h int, xmp string) []byte {
	t.Helper()
	var enc bytes.Buffer
	require.NoError(t, jpeg.Encode(&enc, pixels(w, h), nil))
	raw := enc.Bytes()
	payload := append([]byte("http://ns.adobe.com/xap/1.0/\x00"), []byte(xmp)...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	out := append([]byte{}, raw[:2]...) // SOI
	out = append(out, seg...)
	out = append(out, payload...)
	return append(out, raw[2:]...)
}

func xmpPacket(title, desc, keyword, lat, long string) string {
	return `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` +
		`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:exif="http://ns.adobe.com/exif/1.0/" exif:GPSLatitude="` + lat + `" exif:GPSLongitude="` + long + `">` +
		`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">` + title + `</rdf:li></rdf:Alt></dc:title>` +
		`<dc:description><rdf:Alt><rdf:li xml:lang="x-default">` + desc + `</rdf:li></rdf:Alt></dc:description>` +
		`<dc:subject><rdf:Bag><rdf:li>` + keyword + `</rdf:li></rdf:Bag></dc:subject>` +
		`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, pixels(w, h)))
	return buf.Bytes()
}

// fakeTesseract puts a tesseract on PATH that prints tsv as its result and
// logs each call, and returns the log's path.
func fakeTesseract(t *testing.T, tsv string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\necho \"$1\" >> " + log + "\ncat <<'EOF'\n" + tsv + "EOF\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tesseract"), []byte(script), 0o700)) //nolint:gosec // an executable test double
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	isolateCache(t)
	return log
}

// isolateCache gives the test its own cache, where OCR results are kept.
func isolateCache(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	prev := xdg.GetAppName()
	xdg.SetAppName("vaultmind-test")
	t.Cleanup(func() { xdg.SetAppName(prev) })
}

// tsvWords is tesseract's TSV for words on one line, each with its
// confidence.
func tsvWords(words map[string]int, order []string) string {
	var b strings.Builder
	b.WriteString("level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n")
	b.WriteString("1\t1\t0\t0\t0\t0\t0\t0\t800\t600\t-1\t\n")
	for i, w := range order {
		fmt.Fprintf(&b, "5\t1\t1\t1\t1\t%d\t%d\t10\t40\t12\t%d\t%s\n", i+1, i*50, words[w], w)
	}
	return b.String()
}

func callCount(t *testing.T, log string) int {
	t.Helper()
	raw, err := os.ReadFile(log) //nolint:gosec // test path
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	return strings.Count(string(raw), "\n")
}

func TestImport_AnImagesMetadataBecomesItsNoteGPSIncluded(t *testing.T) {
	fakeTesseract(t, tsvWords(nil, nil))
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "harbour.jpg"),
		string(jpegWithXMP(t, 320, 200, xmpPacket("Harbour at Dawn", "Fishing boats leaving the harbour.", "boats", "59,19.2N", "18,4.2E"))))

	run(t, repo, vault, importdocs.Options{})
	fm, body := noteAt(t, vault, "imported/demo-repo/docs/harbour-jpg.md")
	assert.Equal(t, "Harbour at Dawn", fm["title"])
	assert.Equal(t, []interface{}{"demo-repo:docs/harbour.jpg"}, fm["paths"])
	assert.Contains(t, body, "Fishing boats leaving the harbour.")
	assert.Contains(t, body, "boats")
	assert.Contains(t, body, "59.32", "GPS is kept: Peiman, \"do not drop metadata\"")
	assert.Contains(t, body, "18.07")
	assert.Contains(t, body, "320 × 200")
}

func TestImport_AnImagesTextIsReadAndItsAltTextGathered(t *testing.T) {
	log := fakeTesseract(t, tsvWords(map[string]int{"Ingest": 91, "Pipeline": 88, "~~": 12}, []string{"Ingest", "Pipeline", "~~"}))
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "img", "flow.png"), string(pngBytes(t, 640, 360)))
	write(t, filepath.Join(repo, "docs", "design.md"), "# Design\n\n![The ingest flow, from source to note](img/flow.png)\n")
	write(t, filepath.Join(repo, "docs", "sub", "deep.md"), "# Deep\n\n![Flow, linked from the root](/img/flow.png)\n![Elsewhere](https://example.com/img/flow.png)\n")
	write(t, filepath.Join(repo, "docs", "img", "my flow.png"), string(pngBytes(t, 640, 360)))
	write(t, filepath.Join(repo, "docs", "spaced.md"), "# Spaced\n\n![Bracketed name](<img/my flow.png>)\n![Encoded name](img/my%20flow.png \"a title\")\n")

	run(t, repo, vault, importdocs.Options{})
	fm, body := noteAt(t, vault, "imported/demo-repo/docs/img/flow-png.md")
	assert.Equal(t, "The ingest flow, from source to note", fm["title"], "no metadata title, so the author's alt text")
	assert.Contains(t, body, "Ingest Pipeline")
	assert.NotContains(t, body, "~~", "a word under confidence 65 is noise")
	assert.Contains(t, body, "The ingest flow, from source to note")
	assert.Contains(t, body, "Flow, linked from the root", "a root-relative link starts at the source's root")
	assert.NotContains(t, body, "Elsewhere", "a web image is another image")
	_, spaced := noteAt(t, vault, "imported/demo-repo/docs/img/my flow-png.md")
	assert.Contains(t, spaced, "Bracketed name", "an angle-bracket target may hold spaces")
	assert.Contains(t, spaced, "Encoded name", "a percent-encoded target names the file it decodes to")
	assert.Equal(t, 1, callCount(t, log))

	run(t, repo, vault, importdocs.Options{})
	assert.Equal(t, 1, callCount(t, log), "a re-import reads the text from the cache")
}

func TestImport_AnImageWithNoTextIsSkippedAndCounted(t *testing.T) {
	fakeTesseract(t, tsvWords(nil, nil))
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "blank.png"), string(pngBytes(t, 400, 300)))
	write(t, filepath.Join(repo, "docs", "icon.png"), string(pngBytes(t, 32, 32)))

	res := run(t, repo, vault, importdocs.Options{})
	assert.NoFileExists(t, filepath.Join(vault, "imported/demo-repo/docs/blank-png.md"))
	assert.Contains(t, skipReasons(res), "2 image(s) — hold no text")
}

func TestImport_IconsAreNotOCRed(t *testing.T) {
	log := fakeTesseract(t, tsvWords(map[string]int{"OK": 95}, []string{"OK"}))
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "icon.png"), string(pngBytes(t, 32, 32)))
	run(t, repo, vault, importdocs.Options{})
	assert.Zero(t, callCount(t, log), "an image under 100×100 pixels is an icon")
}

func TestImport_WithoutTesseractImagesAreReadForMetadataOnlyAndSaySo(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	isolateCache(t)
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "harbour.jpg"),
		string(jpegWithXMP(t, 320, 200, xmpPacket("Harbour at Dawn", "Boats.", "boats", "59,19.2N", "18,4.2E"))))
	write(t, filepath.Join(repo, "docs", "flow.png"), string(pngBytes(t, 640, 360)))

	res := run(t, repo, vault, importdocs.Options{})
	assert.FileExists(t, filepath.Join(vault, "imported/demo-repo/docs/harbour-jpg.md"))
	assert.Contains(t, skipReasons(res), "install tesseract to read the text in them")
}

func TestImport_AnSVGsTextBecomesItsNote(t *testing.T) {
	fakeTesseract(t, tsvWords(nil, nil))
	repo, vault := srcRepo(t), t.TempDir()
	write(t, filepath.Join(repo, "docs", "arch.svg"), `<svg xmlns="http://www.w3.org/2000/svg"><title>Architecture</title><desc>How the parts connect</desc>`+
		`<g aria-label="index box"><text x="1" y="2">Indexer</text><text><tspan>Embedder</tspan></text></g><script>alert(1)</script></svg>`)

	run(t, repo, vault, importdocs.Options{})
	fm, body := noteAt(t, vault, "imported/demo-repo/docs/arch-svg.md")
	assert.Equal(t, "Architecture", fm["title"])
	for _, want := range []string{"How the parts connect", "Indexer", "Embedder", "index box"} {
		assert.Contains(t, body, want)
	}
	assert.NotContains(t, body, "alert")
}

// A photo whose metadata is only where it was taken is still a note: "do not drop metadata" (Peiman, 2026-10-03).
func TestImport_APhotoWithOnlyDateAndPlaceIsKept(t *testing.T) {
	fakeTesseract(t, tsvWords(nil, nil))
	repo, vault := srcRepo(t), t.TempDir()
	xmp := `<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"><rdf:Description rdf:about="" xmlns:exif="http://ns.adobe.com/exif/1.0/" exif:GPSLatitude="59,19.2N" exif:GPSLongitude="18,4.2E"/></rdf:RDF></x:xmpmeta>`
	write(t, filepath.Join(repo, "docs", "IMG_0042.jpg"), string(jpegWithXMP(t, 320, 200, xmp)))
	run(t, repo, vault, importdocs.Options{})
	_, body := noteAt(t, vault, "imported/demo-repo/docs/IMG_0042-jpg.md")
	assert.Contains(t, body, "59.32")
}

// A folder of only bare images is reported, not refused as empty.
func TestImport_AFolderOfOnlyBareImagesReportsThem(t *testing.T) {
	fakeTesseract(t, tsvWords(nil, nil))
	dir := filepath.Join(t.TempDir(), "shots")
	write(t, filepath.Join(dir, "blank.png"), string(pngBytes(t, 300, 300)))
	res, err := importdocs.Import(importdocs.Source{Dir: dir, Repo: "shots"}, t.TempDir(), importdocs.Options{})
	require.NoError(t, err)
	assert.Contains(t, skipReasons(res), "1 image(s) — hold no text")
}
