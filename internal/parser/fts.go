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
	reFTSHTMLComment      = regexp.MustCompile(`<!--[\s\S]*?-->`)
	// reVaultMindMarker is the one pattern for VaultMind's own bookkeeping
	// comments. Search text keeps reFTSHTMLComment, which already drops every
	// comment, including these. Delivery calls StripVaultMindMarkers and does
	// not grow a second copy of this pattern.
	reVaultMindMarker = regexp.MustCompile(`(?s)<!--[ \t\r\n]*vaultmind:.*?-->`)
	reFTSHTML         = regexp.MustCompile(`<[^>]+/?>`)
	reFTSBlockIDLine  = regexp.MustCompile(`(?m)\s*\^[a-zA-Z0-9][a-zA-Z0-9\-_]*$`)
	reFTSHRule        = regexp.MustCompile(`(?m)^(\-{3,}|\*{3,}|_{3,})\s*$`)
	reFTSBlockquote   = regexp.MustCompile(`(?m)^>\s?`)
	reFTSMultiSpace   = regexp.MustCompile(`[^\S\n]+`)
	reFTSMultiNewline = regexp.MustCompile(`\n{3,}`)
)

// Shared by ordinary indexing and offset-preserving passage normalization.
var ftsRewrites = []struct {
	re          *regexp.Regexp
	replacement string
}{
	{reFTSHTMLComment, ""},
	{reFTSEmbed, ""}, {reFTSWikilinkAliased, "$1"}, {reFTSWikilink, "$1"}, {reFTSMarkdownLink, "$1"},
	{reFTSHeading, ""}, {reFTSHRule, ""}, {reFTSBlockIDLine, ""}, {reFTSHTML, ""},
	{reFTSInlineCode, "$1"}, {reFTSBoldItalic, ""}, {reFTSItalicStar, "$1"}, {reFTSItalicUnderscore, "$1"},
	{reFTSBlockquote, ""}, {reFTSMultiSpace, " "}, {reFTSMultiNewline, "\n\n"},
}

// StripHTMLComments removes every HTML comment. Search text and recall
// excerpts use it: those paths quote the note's words, not its comments.
// A VaultMind marker is removed here because every comment is. Delivery does
// not call this. An author comment is part of the note, and the only comment
// delivery hides is StripVaultMindMarkers.
func StripHTMLComments(s string) string {
	if !strings.Contains(s, "<!--") {
		return s
	}
	return reFTSHTMLComment.ReplaceAllString(s, "")
}

// StripVaultMindMarkers removes HTML comments whose text starts with
// "vaultmind:". Those are VaultMind's own bookkeeping, written so a retried
// accept does not append a proposal twice. Any other HTML comment is the
// author's and stays. Whitespace after "<!--" is allowed; the comment accept
// writes is "<!-- vaultmind:".
//
// Callers that load a body for an agent: QueryFullNote (ask, memory pack,
// arc recite, --read, summarize, MCP ask), populateRawSnippets, and the FTS
// preview. note get does not call this. It is an explicit raw read.
func StripVaultMindMarkers(s string) string {
	if !strings.Contains(s, "<!--") || !strings.Contains(s, "vaultmind:") {
		return s
	}
	return reVaultMindMarker.ReplaceAllString(s, "")
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
