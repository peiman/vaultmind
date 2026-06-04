package marker

import (
	"fmt"
	"strings"
)

// Issue represents a marker validation problem found in a file.
type Issue struct {
	SectionKey string `json:"section_key,omitempty"`
	Rule       string `json:"rule"`
	Message    string `json:"message"`
	Line       int    `json:"line"`
}

// ValidateMarkers inspects raw file bytes for marker problems and returns
// a slice of Issues. An empty slice means the file is clean.
func ValidateMarkers(raw []byte) []Issue {
	var issues []Issue
	text := string(raw)

	_, err := FindMarkers(raw)
	if err != nil {
		starts := startRe.FindAllStringSubmatchIndex(text, -1)
		ends := endRe.FindAllStringSubmatchIndex(text, -1)

		startKeys := make(map[string]int)
		for _, loc := range starts {
			key := text[loc[2]:loc[3]]
			line := strings.Count(text[:loc[0]], "\n") + 1
			startKeys[key] = line
		}
		endKeys := make(map[string]int)
		for _, loc := range ends {
			key := text[loc[2]:loc[3]]
			line := strings.Count(text[:loc[0]], "\n") + 1
			endKeys[key] = line
		}

		for key, line := range startKeys {
			if _, ok := endKeys[key]; !ok {
				issues = append(issues, Issue{
					SectionKey: key,
					Rule:       "malformed_markers",
					Message:    fmt.Sprintf("START marker for section %q has no matching END", key),
					Line:       line,
				})
			}
		}
		for key, line := range endKeys {
			if _, ok := startKeys[key]; !ok {
				issues = append(issues, Issue{
					SectionKey: key,
					Rule:       "malformed_markers",
					Message:    fmt.Sprintf("END marker for section %q has no matching START", key),
					Line:       line,
				})
			}
		}
		if len(issues) == 0 {
			issues = append(issues, Issue{
				Rule:    "malformed_markers",
				Message: err.Error(),
			})
		}
		return issues
	}

	markers, _ := FindMarkers(raw)
	seen := make(map[string]bool)
	for _, m := range markers {
		if seen[m.SectionKey] {
			line := strings.Count(text[:m.StartOffset], "\n") + 1
			issues = append(issues, Issue{
				SectionKey: m.SectionKey,
				Rule:       "duplicate_markers",
				Message:    fmt.Sprintf("section %q appears more than once", m.SectionKey),
				Line:       line,
			})
		}
		seen[m.SectionKey] = true
	}
	return issues
}
