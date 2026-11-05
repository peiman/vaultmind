package passage

import "strings"

// Section is a heading and everything under it, up to the next heading at its
// level or above: its subsections and its code included. Offsets are bytes
// into the body; Heading is the heading as written (a setext heading with its
// underline).
type Section struct {
	Start, End, Level int
	Heading           string
}

// Sections lists a body's heading sections in document order, read with the
// same heading and fence rules as Units. A heading inside a fence is code, and
// an H1 that repeats the note title is the note itself, not a section of it.
func Sections(body, title string) []Section {
	var out []Section
	lines := strings.SplitAfter(body, "\n")
	at, fenced, skip := 0, false, false
	for i, line := range lines {
		if skip {
			skip = false
			at += len(line)
			continue
		}
		fence, level, label, setext := classifyLine(lines, i, fenced)
		skip = setext
		if fence {
			fenced = !fenced
		} else if !fenced && level > 0 && (level > 1 || !h1RepeatsTitle(label, title)) {
			out = append(out, Section{Start: at, End: len(body), Level: level, Heading: label})
		}
		at += len(line)
	}
	for i := range out {
		for _, next := range out[i+1:] {
			if next.Level <= out[i].Level {
				out[i].End = next.Start
				break
			}
		}
	}
	return out
}
