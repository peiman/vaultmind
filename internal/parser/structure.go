package parser

import (
	"regexp"
	"strings"
	"unicode"
)

// ExtractedHeading is a Markdown heading found in a note body.
type ExtractedHeading struct {
	Level int
	Title string
	Slug  string
	Line  int
}

// ExtractedBlock is a block ID anchor (^block-id) found in a note body.
type ExtractedBlock struct {
	BlockID string
	Heading string
	Line    int
}

var (
	reHeading = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	reBlockID = regexp.MustCompile(`\^([a-zA-Z0-9][a-zA-Z0-9\-_]*)$`)
)

// ExtractHeadings returns all Markdown headings in document order.
// Headings inside fenced code blocks are ignored.
func ExtractHeadings(body string) []ExtractedHeading {
	if body == "" {
		return nil
	}

	lines := strings.Split(body, "\n")
	var headings []ExtractedHeading
	inCodeBlock := false

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1

		if reCodeFence.MatchString(strings.TrimSpace(line)) {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}

		m := reHeading.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		title := strings.TrimSpace(m[2])
		headings = append(headings, ExtractedHeading{
			Level: len(m[1]),
			Title: title,
			Slug:  slugify(title),
			Line:  lineNum,
		})
	}

	return headings
}

// ExtractBlocks returns all block ID anchors (^block-id) in document order.
// Block IDs inside fenced code blocks are ignored.
func ExtractBlocks(body string) []ExtractedBlock {
	if body == "" {
		return nil
	}

	lines := strings.Split(body, "\n")
	var blocks []ExtractedBlock
	inCodeBlock := false
	currentHeading := ""

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1

		if reCodeFence.MatchString(strings.TrimSpace(line)) {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}

		if hm := reHeading.FindStringSubmatch(line); hm != nil {
			currentHeading = strings.TrimSpace(hm[2])
			continue
		}

		if m := reBlockID.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			blocks = append(blocks, ExtractedBlock{
				BlockID: m[1],
				Heading: currentHeading,
				Line:    lineNum,
			})
		}
	}

	return blocks
}

func slugify(title string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			sb.WriteRune(r)
		case r == ' ':
			sb.WriteByte('-')
		case r == '-' || r == '_':
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
