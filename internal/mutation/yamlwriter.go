// Package mutation provides utilities for reading and writing YAML frontmatter
// in Markdown note files while preserving key order, scalar styles, and line
// ending conventions.
package mutation

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// DetectLineEnding returns "\r\n" if CRLF detected, "\n" otherwise.
func DetectLineEnding(raw []byte) string {
	idx := bytes.IndexByte(raw, '\n')
	if idx < 0 {
		return "\n"
	}
	if idx > 0 && raw[idx-1] == '\r' {
		return "\r\n"
	}
	return "\n"
}

// DetectTrailingNewline returns true if raw bytes end with a newline.
func DetectTrailingNewline(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	return raw[len(raw)-1] == '\n'
}

// ParseFrontmatterNode parses raw file bytes into a yaml.Node tree.
// Returns the document node and the body byte offset (after closing ---\n).
func ParseFrontmatterNode(raw []byte) (*yaml.Node, int, error) {
	if !bytes.HasPrefix(raw, []byte("---")) {
		return nil, 0, fmt.Errorf("no frontmatter: file does not start with ---")
	}

	searchStart := bytes.IndexByte(raw, '\n')
	if searchStart < 0 {
		return nil, 0, fmt.Errorf("no frontmatter: no newline after opening ---")
	}
	searchStart++

	closeIdx := -1
	for i := searchStart; i < len(raw); {
		idx := bytes.Index(raw[i:], []byte("---"))
		if idx < 0 {
			break
		}
		absIdx := i + idx
		if absIdx == 0 || raw[absIdx-1] == '\n' {
			afterDashes := absIdx + 3
			if afterDashes >= len(raw) || raw[afterDashes] == '\n' || raw[afterDashes] == '\r' {
				closeIdx = absIdx
				break
			}
		}
		i = absIdx + 3
	}

	if closeIdx < 0 {
		return nil, 0, fmt.Errorf("no frontmatter: closing --- not found")
	}

	yamlBytes := raw[searchStart:closeIdx]

	var doc yaml.Node
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return nil, 0, fmt.Errorf("invalid frontmatter YAML: %w", err)
	}

	bodyOffset := closeIdx + 3
	if bodyOffset < len(raw) && raw[bodyOffset] == '\r' {
		bodyOffset++
	}
	if bodyOffset < len(raw) && raw[bodyOffset] == '\n' {
		bodyOffset++
	}

	return &doc, bodyOffset, nil
}

// SetKey sets or inserts a key in a yaml.Node mapping.
// Preserves position of existing keys. New keys are appended at the end.
func SetKey(mapping *yaml.Node, key string, value interface{}) error {
	if mapping.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping node, got kind %d", mapping.Kind)
	}
	valueNode, err := valueToNode(value)
	if err != nil {
		return fmt.Errorf("converting value for key %q: %w", key, err)
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = valueNode
			return nil
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"}
	mapping.Content = append(mapping.Content, keyNode, valueNode)
	return nil
}

// UnsetKey removes a key from a yaml.Node mapping.
func UnsetKey(mapping *yaml.Node, key string) bool {
	if mapping.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return true
		}
	}
	return false
}

// valueToNode converts a Go value to a yaml.Node.
func valueToNode(value interface{}) (*yaml.Node, error) {
	yamlBytes, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return nil, err
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0], nil
	}
	return nil, fmt.Errorf("unexpected node structure after marshal/unmarshal")
}

// SerializeFrontmatter marshals the yaml.Node back to YAML bytes,
// wrapped in --- delimiters, using the specified line ending convention.
func SerializeFrontmatter(doc *yaml.Node, lineEnding string) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("serializing frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("closing encoder: %w", err)
	}

	yamlStr := buf.String()
	yamlStr = strings.TrimSuffix(yamlStr, "...\n")
	yamlStr = strings.TrimSuffix(yamlStr, "\n")

	var out bytes.Buffer
	out.WriteString("---")
	out.WriteString(lineEnding)
	lines := strings.Split(yamlStr, "\n")
	for i, line := range lines {
		out.WriteString(line)
		if i < len(lines)-1 {
			out.WriteString(lineEnding)
		}
	}
	out.WriteString(lineEnding)
	out.WriteString("---")
	out.WriteString(lineEnding)

	return out.Bytes(), nil
}
