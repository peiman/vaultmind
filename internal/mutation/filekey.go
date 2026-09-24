package mutation

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SetFileKey sets one frontmatter key on the file at path, leaving the body
// and every other key as they were. It exists for files the index deliberately
// leaves out — episodes — which Mutator.Run cannot reach because it resolves
// its target through the index. The write is the same conflict-checked atomic
// write Run uses.
func SetFileKey(path, key string, value interface{}) error {
	abs := filepath.Clean(path)
	raw, err := os.ReadFile(abs) // #nosec G304 -- caller-named vault file // nosemgrep: go-path-traversal
	if err != nil {
		return &MutationError{Code: "read_error", Message: fmt.Sprintf("reading %s: %v", abs, err)}
	}
	doc, bodyOffset, err := ParseFrontmatterNode(raw)
	if err != nil {
		return &MutationError{Code: "parse_error", Message: fmt.Sprintf("parsing frontmatter of %s: %v", abs, err)}
	}
	mapping, err := frontmatterMapping(doc)
	if err != nil {
		return err
	}
	if err := SetKey(mapping, key, value); err != nil {
		return &MutationError{Code: "set_error", Message: fmt.Sprintf("setting key %q: %v", key, err)}
	}
	newFM, err := SerializeFrontmatter(doc, DetectLineEnding(raw))
	if err != nil {
		return &MutationError{Code: "serialize_error", Message: fmt.Sprintf("serializing frontmatter: %v", err)}
	}
	return atomicWrite(abs, abs, SpliceFile(raw, newFM, bodyOffset), fileHash(raw))
}

// frontmatterMapping returns the top-level mapping of a parsed frontmatter
// document, refusing an empty or non-mapping one (R1).
func frontmatterMapping(doc *yaml.Node) (*yaml.Node, error) {
	if doc == nil || doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, &MutationError{Code: "parse_error", Message: "frontmatter produced empty document node"}
	}
	mapping := doc.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil, &MutationError{Code: "parse_error", Message: "frontmatter is not a YAML mapping"}
	}
	return mapping, nil
}
