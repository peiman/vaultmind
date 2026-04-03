package parser

import (
	"regexp"
	"strings"
)

// LinkType classifies how a link was expressed in the source text.
type LinkType string

const (
	LinkTypeWikilink LinkType = "wikilink"
	LinkTypeEmbed    LinkType = "embed"
	LinkTypeMarkdown LinkType = "markdown"
)

// TargetKind classifies what the link target refers to.
type TargetKind string

const (
	TargetKindNote     TargetKind = "note"
	TargetKindHeading  TargetKind = "heading"
	TargetKindBlock    TargetKind = "block"
	TargetKindPath     TargetKind = "path"
	TargetKindExternal TargetKind = "external"
)

// ExtractedLink is a single outbound link found in a note body.
type ExtractedLink struct {
	Target     string
	Display    string
	LinkType   LinkType
	TargetKind TargetKind
	Heading    string
	BlockID    string
	Line       int
}

var (
	reEmbed     = regexp.MustCompile(`!\[\[([^\[\]]+)\]\]`)
	reWikilink  = regexp.MustCompile(`(?:^|[^!])\[\[([^\[\]]+)\]\]`)
	reMarkdown  = regexp.MustCompile(`\[([^\[\]]+)\]\(([^)]+)\)`)
	reCodeFence = regexp.MustCompile("^```")
)

// ExtractLinks scans body text for all outbound links.
// Code fences and inline code spans are skipped.
func ExtractLinks(body string) []ExtractedLink {
	if body == "" {
		return nil
	}

	lines := strings.Split(body, "\n")
	var links []ExtractedLink
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

		stripped := stripInlineCode(line)

		for _, m := range reEmbed.FindAllStringSubmatch(stripped, -1) {
			links = append(links, parseWikilinkInner(m[1], LinkTypeEmbed, lineNum))
		}

		withoutEmbeds := reEmbed.ReplaceAllString(stripped, "")

		for _, m := range reWikilink.FindAllStringSubmatch(withoutEmbeds, -1) {
			links = append(links, parseWikilinkInner(m[1], LinkTypeWikilink, lineNum))
		}

		for _, m := range reMarkdown.FindAllStringSubmatch(stripped, -1) {
			links = append(links, ExtractedLink{
				Target:     m[2],
				Display:    m[1],
				LinkType:   LinkTypeMarkdown,
				TargetKind: classifyMarkdownTarget(m[2]),
				Line:       lineNum,
			})
		}
	}

	return links
}

func parseWikilinkInner(inner string, lt LinkType, line int) ExtractedLink {
	link := ExtractedLink{LinkType: lt, Line: line}

	display := ""
	if idx := strings.Index(inner, "|"); idx >= 0 {
		display = inner[idx+1:]
		inner = inner[:idx]
	}

	if idx := strings.Index(inner, "#"); idx >= 0 {
		anchor := inner[idx+1:]
		link.Target = strings.TrimSpace(inner[:idx])
		if strings.HasPrefix(anchor, "^") {
			link.BlockID = strings.TrimSpace(anchor[1:])
			link.TargetKind = TargetKindBlock
		} else {
			link.Heading = strings.TrimSpace(anchor)
			link.TargetKind = TargetKindHeading
		}
	} else {
		link.Target = strings.TrimSpace(inner)
		link.TargetKind = TargetKindNote
	}

	if display != "" {
		link.Display = strings.TrimSpace(display)
	} else {
		link.Display = link.Target
		if link.Heading != "" {
			link.Display = link.Target + "#" + link.Heading
		} else if link.BlockID != "" {
			link.Display = link.Target + "#^" + link.BlockID
		}
	}

	return link
}

func classifyMarkdownTarget(target string) TargetKind {
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") ||
		strings.HasPrefix(target, "mailto:") {
		return TargetKindExternal
	}
	return TargetKindPath
}

func stripInlineCode(line string) string {
	var sb strings.Builder
	inCode := false
	for i := 0; i < len(line); i++ {
		if line[i] == '`' {
			inCode = !inCode
			sb.WriteByte('`')
			continue
		}
		if inCode {
			sb.WriteByte(' ')
		} else {
			sb.WriteByte(line[i])
		}
	}
	return sb.String()
}
