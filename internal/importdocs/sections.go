package importdocs

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/peiman/vaultmind/internal/vault"
)

// A long doc is past what one note can carry: BGE-M3 embeds the first 8,192
// tokens, and delivery shows a note's opening. Split at import into one note
// per section, each section is retrieved, ranked and delivered like any note
// (the 2026-10-03 probe: the exact section reached the pack for 7 of 12 deep
// questions, against the wrong section of the right page before).
const (
	longDocChars    = 32768 // ~8,192 tokens
	minSectionChars = 1500
	maxSectionChars = 6000
)

// section is one part of a long doc: its heading path (empty before the
// first heading) and its text, without its own heading line.
type section struct {
	path []string
	text string
}

var headingLine = regexp.MustCompile(`^(#{1,6})\s+(.*\S)\s*$`)

// splitSections splits a markdown body at headings outside code fences,
// joins a block under minSectionChars to its neighbour, and splits one over
// maxSectionChars at paragraphs.
func splitSections(body string) []section {
	var out []section
	var cur *section
	for _, b := range headingBlocks(body) {
		chunks := []string{b.text}
		if len(b.text) > maxSectionChars {
			chunks = paragraphChunks(b.text)
		}
		for _, chunk := range chunks {
			fits := cur != nil && len(cur.text)+len(chunk) <= maxSectionChars
			switch {
			case cur == nil:
				cur = &section{path: b.path, text: chunk}
			case fits && (len(chunk) < minSectionChars || len(cur.text) < minSectionChars):
				// A small block joins the section before it (a short trailing
				// subsection stays with its parent), and a small section takes
				// the block after it; the section keeps its first heading.
				if len(b.path) > 0 && !samePath(b.path, cur.path) {
					chunk = strings.Repeat("#", min(len(b.path)+1, 6)) + " " + b.path[len(b.path)-1] + "\n\n" + chunk
				}
				cur.text += "\n\n" + chunk
			default:
				out = append(out, *cur)
				cur = &section{path: b.path, text: chunk}
			}
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

// headingBlocks cuts a body at every heading outside a code fence. A
// heading's path is the headings above it: a stack popped to the first
// heading of a higher level, so "## B" after "### A" is a sibling of A's
// parent, not its child.
func headingBlocks(body string) []section {
	type heading struct {
		level int
		text  string
	}
	var out []section
	var stack []heading
	var lines []string
	fence := ""
	flush := func() {
		if text := strings.TrimSpace(strings.Join(lines, "\n")); text != "" {
			path := make([]string, len(stack))
			for i, h := range stack {
				path[i] = h.text
			}
			out = append(out, section{path: path, text: text})
		}
		lines = nil
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence == "" && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
			fence = trimmed[:3]
		} else if fence != "" && strings.HasPrefix(trimmed, fence) {
			fence = ""
		} else if m := headingLine.FindStringSubmatch(line); fence == "" && m != nil {
			flush()
			level := len(m[1])
			for len(stack) > 0 && stack[len(stack)-1].level >= level {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, heading{level: level, text: plainHeading(m[2])})
			continue
		}
		lines = append(lines, line)
	}
	flush()
	return out
}

var headingLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// plainHeading reads a heading's links as their text and drops a permalink
// ("¶", as Sphinx docs end every heading), which would otherwise name a
// section's title and file. A heading that is nothing but a link stays as
// it is, so it still names something.
func plainHeading(h string) string {
	plain := headingLink.ReplaceAllStringFunc(h, func(link string) string {
		text := headingLink.FindStringSubmatch(link)[1]
		if strings.Trim(text, "¶§#🔗 ") == "" {
			return ""
		}
		return text
	})
	if plain = strings.TrimSpace(plain); plain == "" {
		return h
	}
	return plain
}

// paragraphChunks groups paragraphs into chunks of at most maxSectionChars
// (a single longer paragraph stays whole).
func paragraphChunks(text string) []string {
	var out []string
	cur := ""
	for _, p := range regexp.MustCompile(`\n\s*\n`).Split(text, -1) {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if cur != "" && len(cur)+len(p)+2 > maxSectionChars {
			out = append(out, cur)
			cur = p
			continue
		}
		if cur != "" {
			cur += "\n\n"
		}
		cur += p
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func samePath(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// expandLong turns a long doc into its index note and its section notes; any
// other doc comes back as it is. The index keeps the doc's note path, id and
// source, so links to it and its place in the vault do not change. base is
// the vault folder the notes are written under: index and sections link each
// other by note path, which the import fixes, not by id, which it may suffix.
// A section's source is the doc's plus #NN-slug, and it has no `paths:`:
// opening the doc brings its index, not every section.
func expandLong(d doc, base string) []doc {
	if len(d.Body) <= longDocChars || isTable(d.Rel) {
		return []doc{d}
	}
	secs := splitSections(d.Body)
	if len(secs) < 2 {
		return []doc{d}
	}
	dir := strings.TrimSuffix(noteName(d.Rel), vault.NoteExtension)
	indexLink := linkTarget(path.Join(base, dir), noteID(d.Repo, d.Path))
	docBase := strings.TrimSuffix(d.Path, path.Ext(d.Path))
	out := []doc{{}}
	var lines []string
	// The doc's own title heading (an H1 matching the title) is not repeated
	// in every section's name.
	for i := range secs {
		if len(secs[i].path) > 1 && secs[i].path[0] == d.Title {
			secs[i].path = secs[i].path[1:]
		}
	}
	// A heading whose text was split into parts names each part "(k/n)", so
	// no two section notes share a title.
	total, seen := map[string]int{}, map[string]int{}
	for i, s := range secs {
		total[sectionHeading(s, i)]++
	}
	for i, s := range secs {
		heading := sectionHeading(s, i)
		if total[heading] > 1 {
			seen[heading]++
			heading = fmt.Sprintf("%s (%d/%d)", heading, seen[heading], total[heading])
		}
		part := fmt.Sprintf("%02d-%s", i+1, sectionSlug(s, sectionHeading(s, i)))
		sd := doc{
			Rel: dir + "/" + part + vault.NoteExtension, Path: docBase + "/" + part,
			Repo: d.Repo, Source: d.Source + "#" + part, URL: d.URL, NoPaths: true,
			Title: d.Title + " › " + heading,
			Body:  s.text + "\n\nPart of [[" + indexLink + "]].\n",
		}
		sd.Hash = hashOf(sd.Body)
		lines = append(lines, fmt.Sprintf("- [[%s|%s]] — %s", linkTarget(path.Join(base, dir, part), noteID(sd.Repo, sd.Path)), heading, sectionLead(s.text)))
		out = append(out, sd)
	}
	idx := d
	idx.Body = fmt.Sprintf("%s: %d sections.\n\n%s\n", d.Title, len(secs), strings.Join(lines, "\n"))
	idx.Hash = hashOf(idx.Body)
	out[0] = idx
	return out
}

// isTable reports a doc made from a table (CSV, TSV, a spreadsheet): a
// table is one thing, never cut into sections.
func isTable(rel string) bool {
	switch convertedKind(rel) {
	case "csv", "tsv", "xlsx":
		return true
	}
	return false
}

// expandAll applies expandLong to every doc.
func expandAll(docs []doc, base string) []doc {
	var out []doc
	for _, d := range docs {
		out = append(out, expandLong(d, base)...)
	}
	return out
}

// linkTarget is the wikilink target for an imported note: its vault path,
// which the indexer resolves and the import fixes, unless the path holds a
// character the wikilink parser would cut at; then the note's id stands in.
func linkTarget(notePath, id string) string {
	if strings.ContainsAny(notePath, "#|^[]") {
		return id
	}
	return notePath
}

func sectionHeading(s section, i int) string {
	switch {
	case len(s.path) > 0:
		return strings.Join(s.path, " › ")
	case i == 0:
		return "Introduction"
	}
	return fmt.Sprintf("Part %d", i+1)
}

// sectionSlug names a section's file from its last heading (else from its
// display heading, "Introduction" or "Part N"), kept short.
func sectionSlug(s section, heading string) string {
	name := slug(heading)
	if len(s.path) > 0 {
		if sl := slug(s.path[len(s.path)-1]); sl != "" {
			name = sl
		}
	}
	if name == "" {
		name = "part"
	}
	if len(name) > 50 {
		name = strings.TrimRight(name[:50], "-")
	}
	return name
}

// sectionLead is a section's first line of text, cut for the index.
func sectionLead(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if r := []rune(line); len(r) > 120 {
				line = string(r[:120]) + "…"
			}
			return line
		}
	}
	return ""
}
