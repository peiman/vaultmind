// internal/docs/yaml.go

package docs

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
)

// sortedGroupKeys returns a map's keys in a stable order, so generated output
// is byte-identical across runs. Generated documentation that reshuffles on
// every regeneration cannot be diffed, and therefore cannot be gated.
func sortedGroupKeys(m map[string][]config.ConfigOption) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Function type for YAML content generation, used for testing
type yamlContentGenerator func(w io.Writer, registry []config.ConfigOption) error

// Package variable that points to the real implementation
// This allows for mocking in tests
var generateYAMLContentFunc yamlContentGenerator = generateYAMLContent

// GenerateYAMLDocs generates a YAML configuration template
func (g *Generator) GenerateYAMLDocs(w io.Writer) error {
	registry := g.cfg.Registry()
	return generateYAMLContentFunc(w, registry)
}

// generateYAMLContent generates YAML content for the given registry
// this function is shared by both YAML and Markdown generators
//
//nolint:errcheck // YAML generation - errors from fmt.Fprintf are acceptable
func generateYAMLContent(w io.Writer, registry []config.ConfigOption) error {
	// Group options by top-level key for a nicer YAML structure
	groups := make(map[string][]config.ConfigOption)
	for _, opt := range registry {
		parts := strings.SplitN(opt.Key, ".", 2)
		if len(parts) > 1 {
			topLevel := parts[0]
			groups[topLevel] = append(groups[topLevel], opt)
		} else {
			groups[""] = append(groups[""], opt)
		}
	}

	// Generate YAML.
	//
	// Iterate group keys in sorted order rather than ranging the map: Go
	// randomizes map iteration, so ranging here reordered whole sections on
	// every run, which is what made the generated docs impossible to keep in
	// sync. See TestGenerateYAMLContent_Deterministic.
	for _, topLevel := range sortedGroupKeys(groups) {
		options := groups[topLevel]
		if topLevel != "" {
			fmt.Fprintf(w, "%s:\n", topLevel)
		}

		// Process options
		nestedGroups := make(map[string][]config.ConfigOption)
		nonNestedOptions := []config.ConfigOption{}

		// First separate nested from non-nested options
		for _, opt := range options {
			parts := strings.SplitN(opt.Key, ".", 2)
			if len(parts) == 1 || len(parts) == 2 && !strings.Contains(parts[1], ".") {
				// This is a top-level or single-level nested option
				nonNestedOptions = append(nonNestedOptions, opt)
			} else if len(parts) == 2 {
				// This has further nesting
				nestedKey := strings.SplitN(parts[1], ".", 2)[0]
				nestedGroups[nestedKey] = append(nestedGroups[nestedKey], opt)
			}
		}

		// Output non-nested options first
		for _, opt := range nonNestedOptions {
			parts := strings.SplitN(opt.Key, ".", 2)
			key := opt.Key
			if len(parts) > 1 {
				key = parts[1]
			}

			fmt.Fprintf(w, "  # %s\n", opt.Description)
			fmt.Fprintf(w, "  %s: %s\n\n", key, opt.ExampleValueString())
		}

		// Process nested groups — sorted, for the same reason as above.
		for _, nestedKey := range sortedGroupKeys(nestedGroups) {
			nestedOpts := nestedGroups[nestedKey]
			fmt.Fprintf(w, "  %s:\n", nestedKey)
			for _, opt := range nestedOpts {
				// Extract the part after the second dot
				parts := strings.SplitN(opt.Key, ".", 3)
				var key string
				if len(parts) >= 3 {
					key = parts[2]
				} else {
					// Should not happen, but just in case
					key = opt.Key
				}

				fmt.Fprintf(w, "    # %s\n", opt.Description)
				fmt.Fprintf(w, "    %s: %s\n\n", key, opt.ExampleValueString())
			}
		}
	}

	return nil
}
