package parser

import (
	"regexp"
	"strings"
)

var (
	reFTSEmbed            = regexp.MustCompile(`!\[\[[^\[\]]*\]\]`)
	reFTSWikilinkAliased  = regexp.MustCompile(`\[\[[^\[\]]+\|([^\[\]]+)\]\]`)
	reFTSWikilink         = regexp.MustCompile(`\[\[([^\[\]]*)\]\]`)
	reFTSMarkdownLink     = regexp.MustCompile(`\[([^\[\]]+)\]\([^)]*\)`)
	reFTSHeading          = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	reFTSBoldItalic       = regexp.MustCompile(`\*\*|__|~~`)
	reFTSItalicStar       = regexp.MustCompile(`\*([^*]+)\*`)
	reFTSItalicUnderscore = regexp.MustCompile(`_([^_]+)_`)
	reFTSInlineCode       = regexp.MustCompile("`([^`]*)`")
	reFTSHTML             = regexp.MustCompile(`<[^>]+/?>`)
	reFTSBlockIDLine      = regexp.MustCompile(`(?m)\s*\^[a-zA-Z0-9][a-zA-Z0-9\-_]*$`)
	reFTSHRule            = regexp.MustCompile(`(?m)^(\-{3,}|\*{3,}|_{3,})\s*$`)
	reFTSBlockquote       = regexp.MustCompile(`(?m)^>\s?`)
	reFTSMultiSpace       = regexp.MustCompile(`[^\S\n]+`)
	reFTSMultiNewline     = regexp.MustCompile(`\n{3,}`)
)

// Shared by ordinary indexing and offset-preserving passage normalization.
var ftsRewrites = []struct {
	re          *regexp.Regexp
	replacement string
}{
	{reFTSEmbed, ""}, {reFTSWikilinkAliased, "$1"}, {reFTSWikilink, "$1"}, {reFTSMarkdownLink, "$1"},
	{reFTSHeading, ""}, {reFTSHRule, ""}, {reFTSBlockIDLine, ""}, {reFTSHTML, ""},
	{reFTSInlineCode, "$1"}, {reFTSBoldItalic, ""}, {reFTSItalicStar, "$1"}, {reFTSItalicUnderscore, "$1"},
	{reFTSBlockquote, ""}, {reFTSMultiSpace, " "}, {reFTSMultiNewline, "\n\n"},
}

// StripForFTS converts Markdown body text to plain text for full-text search indexing.
func StripForFTS(body string) string {
	if body == "" {
		return ""
	}

	body = removeFencedCodeBlocks(body)
	for _, rule := range ftsRewrites {
		body = rule.re.ReplaceAllString(body, rule.replacement)
	}

	return strings.TrimSpace(body)
}

func removeFencedCodeBlocks(body string) string {
	lines := strings.Split(body, "\n")
	var result []string
	inFence := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if !inFence {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}
