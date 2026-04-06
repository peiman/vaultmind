// Package template provides template processing and ID generation for notes.
package template

import (
	"path/filepath"
	"strings"
)

// GenerateID creates a deterministic note ID from the note file path and type.
// The ID is formed as "<type>-<slug>" where slug is the lowercase, hyphenated
// base filename (without the .md extension).
func GenerateID(path, noteType string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".md")
	slug := strings.ToLower(base)
	slug = strings.ReplaceAll(slug, " ", "-")
	return noteType + "-" + slug
}
