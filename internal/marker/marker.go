package marker

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
)

// Marker represents a detected generated region in a file.
type Marker struct {
	SectionKey   string
	StartOffset  int
	EndOffset    int
	ContentStart int
	ContentEnd   int
	Checksum     string
	Content      string
}

var (
	startRe    = regexp.MustCompile(`(?m)^<!-- VAULTMIND:GENERATED:([a-z0-9_]+):START -->\r?\n`)
	endRe      = regexp.MustCompile(`(?m)^<!-- VAULTMIND:GENERATED:([a-z0-9_]+):END -->\r?\n?`)
	checksumRe = regexp.MustCompile(`^<!-- checksum:([a-f0-9]+) -->\r?\n`)
)

func FindMarkers(raw []byte) ([]Marker, error) {
	text := string(raw)
	starts := startRe.FindAllStringSubmatchIndex(text, -1)
	ends := endRe.FindAllStringSubmatchIndex(text, -1)

	type endInfo struct {
		matchStart int
		matchEnd   int
		key        string
	}
	endMap := make(map[string][]endInfo)
	for _, loc := range ends {
		key := text[loc[2]:loc[3]]
		endMap[key] = append(endMap[key], endInfo{matchStart: loc[0], matchEnd: loc[1], key: key})
	}

	startKeys := make(map[string]bool)
	for _, loc := range starts {
		startKeys[text[loc[2]:loc[3]]] = true
	}
	for key := range endMap {
		if !startKeys[key] {
			return nil, fmt.Errorf("END marker for section %q has no matching START", key)
		}
	}

	var markers []Marker
	for _, loc := range starts {
		key := text[loc[2]:loc[3]]
		endList := endMap[key]
		if len(endList) == 0 {
			return nil, fmt.Errorf("START marker for section %q has no matching END", key)
		}
		var matchedEnd *endInfo
		for i := range endList {
			if endList[i].matchStart > loc[1] {
				matchedEnd = &endList[i]
				break
			}
		}
		if matchedEnd == nil {
			return nil, fmt.Errorf("START marker for section %q has no matching END after it", key)
		}

		afterStart := loc[1]
		checksum := ""
		contentStart := afterStart
		remaining := text[afterStart:]
		if csMatch := checksumRe.FindStringSubmatchIndex(remaining); csMatch != nil && csMatch[0] == 0 {
			checksum = remaining[csMatch[2]:csMatch[3]]
			contentStart = afterStart + csMatch[1]
		}

		markers = append(markers, Marker{
			SectionKey:   key,
			StartOffset:  loc[0],
			EndOffset:    matchedEnd.matchEnd,
			ContentStart: contentStart,
			ContentEnd:   matchedEnd.matchStart,
			Checksum:     checksum,
			Content:      text[contentStart:matchedEnd.matchStart],
		})
	}

	sort.Slice(markers, func(i, j int) bool {
		return markers[i].StartOffset < markers[j].StartOffset
	})

	return markers, nil
}

func ContentChecksum(content []byte) string {
	h := sha256.Sum256(content)
	return fmt.Sprintf("%x", h[:])
}
