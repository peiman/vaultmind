package importdocs

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"path"
	"strings"
)

// maxSVGBytes caps a drawing read for its text.
const maxSVGBytes = 5 << 20

// readSVGDoc reads an SVG drawing's words: its <title>, <desc>, the <text>
// and <tspan> it shows, and the aria-labels it gives its parts. Scripts and
// styles are not words. ok is false when the drawing holds none.
func readSVGDoc(src Source, rel, p string) (doc, bool, error) {
	raw, err := readCapped(p, maxSVGBytes, "SVG file is larger than 5 MiB")
	if err != nil {
		return doc{}, false, err
	}
	title, lines := svgText(raw)
	if title == "" && len(lines) == 0 {
		return doc{}, false, nil
	}
	if title == "" {
		title = strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Text in the drawing\n\n")
	for _, l := range lines {
		fmt.Fprintf(&b, "- %s\n", l)
	}
	body := normalizeMarkdown(b.String())
	docPath := path.Join(src.Prefix, rel)
	return doc{
		Rel: rel, Path: docPath, Repo: src.Repo, Source: src.Repo + ":" + docPath,
		Title: title, Body: body, Hash: hashOf(body),
	}, true, nil
}

// svgText is the drawing's title and its other words, in document order,
// each once.
func svgText(raw []byte) (string, []string) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = false
	var title string
	var lines []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.Join(strings.Fields(s), " ")
		if s != "" && !seen[s] {
			seen[s] = true
			lines = append(lines, s)
		}
	}
	var stack []string
	for {
		tok, err := dec.Token()
		if err != nil {
			return title, lines
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			if l := attr(t, "aria-label"); l != "" {
				add(l)
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			switch stack[len(stack)-1] {
			case "title":
				if title == "" {
					title = strings.Join(strings.Fields(string(t)), " ")
					continue
				}
				add(string(t))
			case "desc", "text", "tspan", "textPath":
				add(string(t))
			}
		}
	}
}
