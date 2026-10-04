package importdocs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif" // the pixel size of a GIF, which carries no metadata to read
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bep/imagemeta"
	"github.com/peiman/vaultmind/internal/xdg"
)

// imageFormats are the raster images an import reads, by extension, with
// the format the metadata reader takes.
var imageFormats = map[string]imagemeta.ImageFormat{
	"png": imagemeta.PNG, "jpg": imagemeta.JPEG, "jpeg": imagemeta.JPEG,
	"webp": imagemeta.WebP, "heic": imagemeta.HEIF, "heif": imagemeta.HEIF,
	"gif": imagemeta.ImageFormatAuto, // no metadata reader: the size comes from image.DecodeConfig
}

const (
	// maxImageBytes and maxOCRPixels bound what is read and what is OCRed:
	// a 200-megapixel scan is not worth a minute of tesseract.
	maxImageBytes = 50 << 20
	maxOCRPixels  = 50_000_000
	// minOCRSide is the smallest side worth OCR: below it the image is an
	// icon, and a repository holds hundreds of them.
	minOCRSide = 100
	// minOCRConfidence drops tesseract's guesses: a word under it is
	// usually noise read from a texture.
	minOCRConfidence = 65
	// ocrTimeout bounds one image.
	ocrTimeout = 60 * time.Second
	// ocrCacheVersion keys the cache: bump it when the OCR call or its
	// parsing changes, and every image is read again.
	ocrCacheVersion = "tess-psm3-c65-v1"
)

// tesseractReads are the formats tesseract opens. HEIC is read for its
// metadata only: tesseract has no decoder for it.
var tesseractReads = map[string]bool{"png": true, "jpg": true, "jpeg": true, "webp": true, "gif": true}

// imageReading is one import's view of OCR: whether tesseract is there, and
// how many images went without their text because it was not.
type imageReading struct {
	tesseract string // its path, "" when it is not on PATH
	unread    int
}

func newImageReading() *imageReading {
	p, _ := exec.LookPath("tesseract")
	return &imageReading{tesseract: p}
}

// imageField is one line of an image's metadata.
type imageField struct {
	label, value string
	// text marks a field that earns the image a note: anything but its
	// pixel size. A photo of only date, camera and GPS is kept ("do not drop
	// metadata"); a bare icon is not.
	text bool
}

// readImageDoc reads a raster image as a doc: what its metadata says, the
// text tesseract finds in it, and the alt text the import's markdown gives
// it. ok is false when none of that holds text: such an image is counted,
// not written.
func readImageDoc(src Source, rel, p string, alts []string, r *imageReading) (doc, bool, error) {
	info, err := os.Stat(p)
	if err != nil {
		return doc{}, false, err
	}
	if info.Size() > maxImageBytes {
		return doc{}, false, fmt.Errorf("image is larger than %d MiB", maxImageBytes>>20)
	}
	// nosemgrep: go-path-traversal -- a file found by walking the folder the operator named; symlinks were skipped
	raw, err := os.ReadFile(p) //nolint:gosec // same
	if err != nil {
		return doc{}, false, err
	}
	fields, w, h := imageMetadata(raw, imageFormats[convertedKind(p)])
	text := r.ocr(raw, p, w, h)
	hasText := text != "" || len(alts) > 0
	for _, f := range fields {
		hasText = hasText || f.text
	}
	if !hasText {
		return doc{}, false, nil
	}
	body := imageBody(text, alts, fields)
	docPath := path.Join(src.Prefix, rel)
	return doc{
		Rel: rel, Path: docPath, Repo: src.Repo, Source: src.Repo + ":" + docPath,
		Title: imageTitle(fields, alts, rel), Body: body, Hash: hashOf(body),
	}, true, nil
}

// imageMetadata reads EXIF, IPTC and XMP, and the pixel size. An image whose
// metadata cannot be read still has its pixels: the error is not fatal.
func imageMetadata(raw []byte, format imagemeta.ImageFormat) ([]imageField, int, int) {
	var tags imagemeta.Tags
	var res imagemeta.DecodeResult
	if format == imagemeta.ImageFormatAuto {
		return imageSizeOnly(raw)
	}
	res, _ = imagemeta.Decode(imagemeta.Options{
		R: bytes.NewReader(raw), ImageFormat: format,
		Sources:   imagemeta.EXIF | imagemeta.IPTC | imagemeta.XMP | imagemeta.CONFIG,
		HandleTag: func(ti imagemeta.TagInfo) error { tags.Add(ti); return nil },
		Timeout:   10 * time.Second, LimitNumTags: 5000, LimitTagSize: 1 << 20,
	})
	w, h := res.ImageConfig.Width, res.ImageConfig.Height
	if w == 0 || h == 0 {
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(raw)); err == nil {
			w, h = cfg.Width, cfg.Height
		}
	}
	return metadataFields(&tags, w, h), w, h
}

// imageSizeOnly is the metadata of a format with none to read: its size.
func imageSizeOnly(raw []byte) ([]imageField, int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, 0
	}
	return metadataFields(&imagemeta.Tags{}, cfg.Width, cfg.Height), cfg.Width, cfg.Height
}

// metadataFields picks the fields worth a line, each from the first source
// that has it.
func metadataFields(tags *imagemeta.Tags, w, h int) []imageField {
	x, i, e := tags.XMP(), tags.IPTC(), tags.EXIF()
	pick := func(sources ...struct {
		m map[string]imagemeta.TagInfo
		k string
	}) string {
		for _, s := range sources {
			if v, ok := s.m[s.k]; ok {
				if str := tagText(v.Value); str != "" {
					return str
				}
			}
		}
		return ""
	}
	type src = struct {
		m map[string]imagemeta.TagInfo
		k string
	}
	var out []imageField
	add := func(label, value string, text bool) {
		if value != "" {
			out = append(out, imageField{label: label, value: value, text: text})
		}
	}
	add("Title", pick(src{x, "Title"}, src{i, "ObjectName"}), true)
	add("Headline", pick(src{x, "Headline"}, src{i, "Headline"}), true)
	add("Description", pick(src{x, "Description"}, src{i, "Caption-Abstract"}, src{e, "ImageDescription"}), true)
	add("Alt text", pick(src{x, "AltTextAccessibility"}), true)
	add("Keywords", pick(src{x, "Subject"}, src{i, "Keywords"}), true)
	add("People", pick(src{x, "PersonInImage"}), true)
	add("Place", place(x, i), true)
	add("Comment", pick(src{e, "UserComment"}), true)
	add("Creator", pick(src{x, "Creator"}, src{i, "By-line"}, src{e, "Artist"}), true)
	add("Copyright", pick(src{x, "Rights"}, src{i, "CopyrightNotice"}, src{e, "Copyright"}), true)
	add("Taken", pick(src{e, "DateTimeOriginal"}, src{x, "DateCreated"}, src{x, "CreateDate"}, src{i, "DateCreated"}), true)
	add("Camera", strings.TrimSpace(pick(src{e, "Make"})+" "+pick(src{e, "Model"})), true)
	if lat, long, err := tags.GetLatLong(); err == nil && (lat != 0 || long != 0) {
		add("GPS", fmt.Sprintf("%.5f, %.5f", lat, long), true)
	}
	if w > 0 && h > 0 {
		add("Size", fmt.Sprintf("%d × %d pixels", w, h), false)
	}
	return out
}

// place joins the location fields IPTC and XMP name, most specific first.
func place(x, i map[string]imagemeta.TagInfo) string {
	var parts []string
	seen := map[string]bool{}
	for _, f := range []struct {
		m map[string]imagemeta.TagInfo
		k string
	}{{i, "Sub-location"}, {x, "Location"}, {i, "City"}, {x, "City"}, {i, "Province-State"}, {x, "State"},
		{i, "Country-PrimaryLocationName"}, {x, "Country"}} {
		if v, ok := f.m[f.k]; ok {
			if s := tagText(v.Value); s != "" && !seen[s] {
				seen[s] = true
				parts = append(parts, s)
			}
		}
	}
	return strings.Join(parts, ", ")
}

// tagText is a tag's value as one line of text.
func tagText(v any) string {
	switch t := v.(type) {
	case string:
		return strings.Join(strings.Fields(t), " ")
	case []string:
		var parts []string
		for _, s := range t {
			// A value given in several languages is often the same text twice.
			if s = strings.Join(strings.Fields(s), " "); s != "" && !containsString(parts, s) {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// imageTitle is the metadata's title or headline, else the first alt text,
// else the file name.
func imageTitle(fields []imageField, alts []string, rel string) string {
	for _, want := range []string{"Title", "Headline"} {
		for _, f := range fields {
			if f.label == want {
				return f.value
			}
		}
	}
	if len(alts) > 0 {
		return alts[0]
	}
	return strings.TrimSuffix(path.Base(rel), path.Ext(rel))
}

// imageBody lays out what is known of an image, each section only when it
// has something.
func imageBody(text string, alts []string, fields []imageField) string {
	var b strings.Builder
	if text != "" {
		fmt.Fprintf(&b, "## Text in the image\n\n%s\n\n", text)
	}
	if len(alts) > 0 {
		b.WriteString("## Alt text\n\n")
		for _, a := range alts {
			fmt.Fprintf(&b, "- %s\n", a)
		}
		b.WriteString("\n")
	}
	if len(fields) > 0 {
		b.WriteString("## Metadata\n\n")
		for _, f := range fields {
			fmt.Fprintf(&b, "- **%s:** %s\n", f.label, f.value)
		}
	}
	return normalizeMarkdown(b.String())
}

// ocr is the text tesseract reads in an image, from the cache when it was
// read before. An icon, an image too large, and an import without
// tesseract get none; the last is counted, to be said once.
func (r *imageReading) ocr(raw []byte, p string, w, h int) string {
	if w < minOCRSide || h < minOCRSide || w*h > maxOCRPixels || !tesseractReads[convertedKind(p)] {
		return ""
	}
	key := ocrCacheKey(raw)
	if text, ok := readOCRCache(key); ok {
		return text
	}
	if r.tesseract == "" {
		r.unread++
		return ""
	}
	text, err := runTesseract(r.tesseract, p)
	if err != nil {
		return ""
	}
	writeOCRCache(key, text)
	return text
}

// runTesseract reads an image with tesseract and keeps its confident words,
// line by line.
func runTesseract(bin, p string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ocrTimeout)
	defer cancel()
	// nosemgrep: go-dangerous-exec -- bin is exec.LookPath("tesseract"), the arguments are fixed, and p is a file found walking the operator's folder
	out, err := exec.CommandContext(ctx, bin, p, "stdout", "--psm", "3", "tsv").Output() //nolint:gosec // same
	if err != nil {
		return "", err
	}
	return tsvText(string(out)), nil
}

// tsvText turns tesseract's TSV into lines of the words it is confident of.
// Columns: level page block par line word left top width height conf text.
func tsvText(tsv string) string {
	var lines []string
	var line []string
	lastKey := ""
	for _, row := range strings.Split(tsv, "\n") {
		cols := strings.Split(row, "\t")
		if len(cols) < 12 || cols[0] != "5" {
			continue
		}
		conf, err := strconv.ParseFloat(cols[10], 64)
		word := strings.TrimSpace(cols[11])
		if err != nil || conf < minOCRConfidence || word == "" {
			continue
		}
		key := strings.Join(cols[1:5], ".")
		if key != lastKey && len(line) > 0 {
			lines = append(lines, strings.Join(line, " "))
			line = nil
		}
		lastKey = key
		line = append(line, word)
	}
	if len(line) > 0 {
		lines = append(lines, strings.Join(line, " "))
	}
	return strings.Join(lines, "\n")
}

func ocrCacheKey(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]) + "-" + ocrCacheVersion
}

// ocrCacheDir is where read text is kept, by image content: a re-import, or
// the same image in another folder, is not read again. A test points it
// elsewhere through XDG_CACHE_HOME.
func ocrCacheDir() (string, error) {
	dir, err := xdg.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ocr"), nil
}

func readOCRCache(key string) (string, bool) {
	dir, err := ocrCacheDir()
	if err != nil {
		return "", false
	}
	// nosemgrep: go-path-traversal -- key is a sha256 hex string and a constant, under our own cache folder
	raw, err := os.ReadFile(filepath.Join(dir, key+".txt")) //nolint:gosec // same
	if err != nil {
		return "", false
	}
	return string(raw), true
}

func writeOCRCache(key, text string) {
	dir, err := ocrCacheDir()
	if err != nil {
		return
	}
	// The text is read from a person's images: the folder is theirs alone.
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	// Written whole, then renamed into place: a concurrent import never reads
	// half a result.
	tmp, err := os.CreateTemp(dir, key+".*.tmp")
	if err != nil {
		return
	}
	_, werr := tmp.WriteString(text)
	if cerr := tmp.Close(); werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name()) //nolint:gosec // the temporary file CreateTemp just made in our cache folder
		return
	}
	// nosemgrep: go-path-traversal -- key is a sha256 hex string and a constant, under our own cache folder
	if os.Rename(tmp.Name(), filepath.Join(dir, key+".txt")) != nil { //nolint:gosec // same
		_ = os.Remove(tmp.Name()) //nolint:gosec // the temporary file CreateTemp just made in our cache folder
	}
}

// markdownImageRef is an image a markdown doc shows: its alt text and its
// target, either <in angle brackets>, which may hold spaces, or bare.
var markdownImageRef = regexp.MustCompile(`!\[([^\]]+)\]\(\s*(?:<([^>]+)>|([^)\s]+))`)

// altTexts maps each image, by its path under the source, to the alt texts
// the import's markdown docs give it. Empty alt texts are passed over.
func altTexts(docs []doc) map[string][]string {
	out := map[string][]string{}
	for _, d := range docs {
		if path.Ext(d.Rel) != ".md" || d.URL != "" {
			continue
		}
		for _, m := range markdownImageRef.FindAllStringSubmatch(d.Body, -1) {
			alt, target := strings.Join(strings.Fields(m[1]), " "), m[2]+m[3]
			if alt == "" {
				continue
			}
			target, _, _ = strings.Cut(target, "#")
			if dec, err := url.PathUnescape(target); err == nil {
				target = dec // my%20flow.png is the file "my flow.png"
			}
			// A root-relative target starts at the source's root, as on the
			// site the docs build. A web image's key names no file here, so it
			// matches nothing.
			key := path.Clean(path.Join(path.Dir(d.Rel), target))
			if strings.HasPrefix(target, "/") {
				key = path.Clean(strings.TrimPrefix(target, "/"))
			}
			if !containsString(out[key], alt) {
				out[key] = append(out[key], alt)
			}
		}
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
