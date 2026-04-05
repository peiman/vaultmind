package marker

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/mutation"
)

// ReplaceRegion replaces the content between markers for a given section_key.
// Inserts a checksum comment after START. Returns checksum_mismatch error if hand-edited and force is false.
func ReplaceRegion(raw []byte, sectionKey string, newContent []byte, force bool) ([]byte, error) {
	markers, err := FindMarkers(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing markers: %w", err)
	}

	var target *Marker
	for i := range markers {
		if markers[i].SectionKey == sectionKey {
			target = &markers[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("marker for section %q not found in file", sectionKey)
	}

	// Check for hand-edit
	if target.Checksum != "" && !force {
		currentChecksum := ContentChecksum([]byte(target.Content))
		if currentChecksum != target.Checksum {
			return nil, &mutation.MutationError{
				Code:    "checksum_mismatch",
				Message: fmt.Sprintf("content in section %q was hand-edited since last generation", sectionKey),
				Field:   sectionKey,
			}
		}
	}

	// Build replacement block
	newChecksum := ContentChecksum(newContent)
	startLine := string(raw[target.StartOffset : target.StartOffset+findLineEnd(raw, target.StartOffset)])
	endLine := string(raw[target.ContentEnd:target.EndOffset])

	var replacement []byte
	replacement = append(replacement, []byte(startLine)...)
	replacement = append(replacement, []byte(fmt.Sprintf("<!-- checksum:%s -->\n", newChecksum))...)
	replacement = append(replacement, newContent...)
	replacement = append(replacement, []byte(endLine)...)

	// Splice
	var result []byte
	result = append(result, raw[:target.StartOffset]...)
	result = append(result, replacement...)
	result = append(result, raw[target.EndOffset:]...)

	return result, nil
}

// findLineEnd returns the length from offset to the end of the line (including newline).
func findLineEnd(raw []byte, offset int) int {
	for i := offset; i < len(raw); i++ {
		if raw[i] == '\n' {
			return i - offset + 1
		}
	}
	return len(raw) - offset
}
