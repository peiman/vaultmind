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

// StripForFTS converts Markdown body text to plain text for full-text search indexing.
func StripForFTS(body string) string {
	if body == "" {
		return ""
	}

	body = removeFencedCodeBlocks(body)
	body = reFTSEmbed.ReplaceAllString(body, "")
	body = reFTSWikilinkAliased.ReplaceAllString(body, "$1")
	body = reFTSWikilink.ReplaceAllString(body, "$1")
	body = reFTSMarkdownLink.ReplaceAllString(body, "$1")
	body = reFTSHeading.ReplaceAllString(body, "")
	body = reFTSHRule.ReplaceAllString(body, "")
	body = reFTSBlockIDLine.ReplaceAllString(body, "")
	body = reFTSHTML.ReplaceAllString(body, "")
	body = reFTSInlineCode.ReplaceAllString(body, "$1")
	body = reFTSBoldItalic.ReplaceAllString(body, "")
	body = reFTSItalicStar.ReplaceAllString(body, "$1")
	body = reFTSItalicUnderscore.ReplaceAllString(body, "$1")
	body = reFTSBlockquote.ReplaceAllString(body, "")
	body = reFTSMultiSpace.ReplaceAllString(body, " ")
	body = reFTSMultiNewline.ReplaceAllString(body, "\n\n")

	return strings.TrimSpace(body)
}

func removeFencedCodeBlocks(body string) string {
	lines := strings.Split(body, "\n")
	var result []string
	inFence := false

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}
