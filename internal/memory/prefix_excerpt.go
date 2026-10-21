package memory

import (
	"strings"

	"github.com/peiman/vaultmind/internal/parser"
)

// PrefixExcerpt returns the opening of a pinned note, including headings,
// lists and code. It keeps whole lines when possible and includes any closing
// code fence in the token cost, so an excerpt cannot swallow the next note.
func PrefixExcerpt(body string, maxTokens int) string {
	if maxTokens <= 0 {
		return ""
	}
	body = strings.TrimSpace(parser.StripHTMLComments(stripFrontmatter(body)))
	if body == "" {
		return ""
	}
	var prefix, kept, fence string
	for i, line := range strings.Split(body, "\n") {
		if i > 0 {
			prefix += "\n"
		}
		prefix += line
		fence = prefixFenceAfter(line, fence)
		candidate := closePrefixFence(prefix, fence)
		if EstimateTokens(candidate) > maxTokens {
			if i == 0 {
				return truncatePrefixLine(line, fence, maxTokens)
			}
			break
		}
		kept = candidate
	}
	return strings.TrimSpace(kept)
}

// prefixFenceAfter tracks backtick fences, including longer Markdown fences.
// A shorter fence or a fence with trailing text cannot close an open block.
func prefixFenceAfter(line, open string) string {
	line = strings.TrimSpace(line)
	n := len(line) - len(strings.TrimLeft(line, "`"))
	if n < 3 {
		return open
	}
	if open == "" {
		if !strings.Contains(line[n:], "`") {
			return line[:n]
		}
	} else if n >= len(open) && strings.TrimSpace(line[n:]) == "" {
		return ""
	}
	return open
}

func closePrefixFence(text, fence string) string {
	if fence != "" {
		return text + "\n" + fence
	}
	return text
}

// Only an oversized first line is cut internally. Rune boundaries preserve
// UTF-8, and the ellipsis and closing fence both count against the cap.
func truncatePrefixLine(line, fence string, maxTokens int) string {
	var kept string
	for end := range line {
		if end == 0 || end < len(fence) {
			continue
		}
		candidate := closePrefixFence(line[:end]+ellipsis, fence)
		if EstimateTokens(candidate) > maxTokens {
			break
		}
		kept = candidate
	}
	return kept
}
