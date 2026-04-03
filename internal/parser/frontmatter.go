// Package parser extracts frontmatter, links, headings, and blocks from Markdown files.
package parser

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const frontmatterDelimiter = "---"

// ExtractFrontmatter splits a raw .md file into its YAML frontmatter map
// and the remaining body text.
//
// If the file has no frontmatter (does not start with "---\n"), fm is nil
// and body is the entire file content.
func ExtractFrontmatter(content []byte) (fm map[string]interface{}, body string, err error) {
	s := string(content)

	// Frontmatter must start at byte 0 with "---\n" or "---\r\n"
	if !strings.HasPrefix(s, frontmatterDelimiter+"\n") &&
		!strings.HasPrefix(s, frontmatterDelimiter+"\r\n") {
		return nil, s, nil
	}

	// Find the closing delimiter
	rest := s[len(frontmatterDelimiter):]
	closeIdx := strings.Index(rest, "\n"+frontmatterDelimiter)
	if closeIdx < 0 {
		return nil, s, nil
	}

	yamlBlock := rest[1 : closeIdx+1]
	afterClose := rest[closeIdx+1+len(frontmatterDelimiter):]

	body = strings.TrimPrefix(afterClose, "\r\n")
	body = strings.TrimPrefix(body, "\n")

	if strings.TrimSpace(yamlBlock) == "" {
		return nil, body, nil
	}

	var parsed map[string]interface{}
	if yamlErr := yaml.Unmarshal([]byte(yamlBlock), &parsed); yamlErr != nil {
		return nil, "", fmt.Errorf("parsing frontmatter YAML: %w", yamlErr)
	}

	return parsed, body, nil
}

// ClassifyNote determines whether a note is a domain note (has both id and type
// as non-empty strings) or an unstructured note.
func ClassifyNote(fm map[string]interface{}) (isDomain bool, id string, noteType string) {
	if fm == nil {
		return false, "", ""
	}

	rawID, hasID := fm["id"]
	rawType, hasType := fm["type"]
	if !hasID || !hasType {
		return false, "", ""
	}

	idStr, idOK := rawID.(string)
	typeStr, typeOK := rawType.(string)
	if !idOK || !typeOK || idStr == "" || typeStr == "" {
		return false, "", ""
	}

	return true, idStr, typeStr
}
