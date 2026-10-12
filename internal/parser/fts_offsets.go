package parser

import (
	"strings"
)

// StripForFTSOffsets applies the keyword lane's Markdown normalization while
// retaining the raw byte position of each remaining byte for excerpt delivery.
func StripForFTSOffsets(body string) (string, []int) {
	offsets := make([]int, len(body))
	for i := range offsets {
		offsets[i] = i
	}

	// Match the line-based fence stripper, including unclosed fences.
	var out strings.Builder
	var mapped []int
	at, fenced, first := 0, false, true
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
		} else if !fenced {
			if !first {
				out.WriteByte('\n')
				mapped = append(mapped, at-1)
			}
			out.WriteString(line)
			mapped = append(mapped, offsets[at:at+len(line)]...)
			first = false
		}
		at += len(line) + 1
	}
	body, offsets = out.String(), mapped
	for _, rule := range ftsRewrites {
		var out strings.Builder
		mapped := make([]int, 0, len(offsets))
		last := 0
		for _, loc := range rule.re.FindAllStringSubmatchIndex(body, -1) {
			out.WriteString(body[last:loc[0]])
			mapped = append(mapped, offsets[last:loc[0]]...)
			if rule.replacement == "$1" {
				out.WriteString(body[loc[2]:loc[3]])
				mapped = append(mapped, offsets[loc[2]:loc[3]]...)
			} else {
				out.WriteString(rule.replacement)
				for i := range len(rule.replacement) {
					mapped = append(mapped, offsets[loc[0]+i])
				}
			}
			last = loc[1]
		}
		out.WriteString(body[last:])
		mapped = append(mapped, offsets[last:]...)
		body, offsets = out.String(), mapped
	}

	trimmed := strings.TrimSpace(body)
	start := strings.Index(body, trimmed)
	return trimmed, offsets[start : start+len(trimmed)]
}
