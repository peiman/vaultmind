// internal/docs/yaml_test.go

package docs

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateYAMLDocs tests the YAML document generation
func TestGenerateYAMLDocs(t *testing.T) {
	// SETUP PHASE
	// Create output buffer
	var buf bytes.Buffer

	// Create a mock registry function for testing
	mockRegistry := func() []config.ConfigOption {
		return []config.ConfigOption{
			{
				Key:          "app.log_level",
				Type:         "string",
				DefaultValue: "info",
				Description:  "Application log level",
			},
			{
				Key:          "app.ping.enabled",
				Type:         "bool",
				DefaultValue: true,
				Description:  "Enable ping endpoint",
			},
		}
	}

	// Create generator with mock registry
	cfg := Config{
		Writer:       &buf,
		OutputFormat: FormatYAML,
		OutputFile:   "",
		Registry:     mockRegistry,
	}
	generator := NewGenerator(cfg)

	// EXECUTION PHASE
	err := generator.GenerateYAMLDocs(&buf)

	// ASSERTION PHASE
	require.NoError(t, err, "GenerateYAMLDocs failed")

	output := buf.String()

	// Check for basic YAML structure
	expectedLines := []string{
		"app:",         // Top-level section
		"  log_level:", // Option
		"  ping:",      // Nested section
	}

	for _, line := range expectedLines {
		assert.True(t, strings.Contains(output, line), "Missing expected YAML line: %s", line)
	}

	// Check that options have descriptions
	assert.True(t, strings.Contains(output, "  # "), "Missing option description comments")

	// Check that we have proper indentation for nested options
	assert.True(t, strings.Contains(output, "    "), // 4-space indentation for nested options
		"Missing proper indentation for nested options")
}

// TestGenerateYAMLContent tests the YAML content generator
func TestGenerateYAMLContent(t *testing.T) {
	// SETUP PHASE
	// Create a simple mock config registry for testing
	mockOptions := []struct {
		key         string
		description string
	}{
		{"app.simple", "A simple option"},
		{"app.nested.option", "A nested option"},
		{"standalone", "A standalone option"},
	}

	// Build mock ConfigOptions from the simple data
	mockConfigOptions := make([]config.ConfigOption, 0, len(mockOptions))
	for _, opt := range mockOptions {
		mockConfigOptions = append(mockConfigOptions, config.ConfigOption{
			Key:          opt.key,
			Description:  opt.description,
			DefaultValue: "test-value",
			Type:         "string",
		})
	}

	// Create buffer for output
	var buf bytes.Buffer

	// EXECUTION PHASE
	err := generateYAMLContent(&buf, mockConfigOptions)

	// ASSERTION PHASE
	require.NoError(t, err, "generateYAMLContent failed")

	output := buf.String()

	// Check basic structure
	expectedStructure := []string{
		"app:",
		"  # A simple option",
		"  simple: test-value",
		"  nested:",
		"    # A nested option",
		"    option: test-value",
		"# A standalone option",
		"standalone: test-value",
	}

	for _, line := range expectedStructure {
		assert.True(t, strings.Contains(output, line), "Missing expected YAML content: %s", line)
	}

	// Verify proper indentation logic
	assert.False(t, strings.Contains(output, "app.simple"),
		"Improper key formatting - did not properly convert dots to nesting")
}

// TestGenerateYAMLDocs_EmptyRegistry tests handling of an empty registry
func TestGenerateYAMLDocs_EmptyRegistry(t *testing.T) {
	// SETUP PHASE
	// Create output buffer
	var buf bytes.Buffer

	// Create generator with empty registry
	cfg := Config{
		Writer:       &buf,
		OutputFormat: FormatYAML,
		OutputFile:   "",
		Registry: func() []config.ConfigOption {
			return []config.ConfigOption{}
		},
	}
	generator := NewGenerator(cfg)

	// EXECUTION PHASE
	err := generator.GenerateYAMLDocs(&buf)

	// ASSERTION PHASE
	require.NoError(t, err, "GenerateYAMLDocs failed with empty registry")

	// For an empty registry, we expect an empty output (or just whitespace)
	output := buf.String()
	trimmed := strings.TrimSpace(output)
	assert.Empty(t, trimmed, "Expected empty output for empty registry, got: %q", output)
}

// TestGenerateYAMLContent_Deterministic pins the ordering of generated output.
//
// generateYAMLContent grouped options into maps and then ranged over those maps
// directly, so Go's randomized map iteration reordered whole sections on every
// run. Two consecutive generations from one binary differed by 850 lines —
// identical content, shuffled. That made the checked-in docs/config-template.yaml
// and docs/configuration.md impossible to keep in sync: regenerating always
// produced an enormous meaningless diff, so the honest act looked like vandalism
// and the files drifted until they documented the upstream scaffold instead of
// this tool.
//
// A drift gate cannot exist until this holds, which is why the assertion is on
// byte-equality across runs rather than on any particular key order.
func TestGenerateYAMLContent_Deterministic(t *testing.T) {
	registry := []config.ConfigOption{
		{Key: "zeta.b.two", Type: "string", DefaultValue: "2", Description: "z b two"},
		{Key: "alpha.one", Type: "string", DefaultValue: "1", Description: "a one"},
		{Key: "mid.nested.deep", Type: "bool", DefaultValue: false, Description: "m deep"},
		{Key: "zeta.a.one", Type: "int", DefaultValue: 1, Description: "z a one"},
		{Key: "alpha.two", Type: "string", DefaultValue: "2", Description: "a two"},
		{Key: "mid.other.deep", Type: "bool", DefaultValue: true, Description: "m other"},
		{Key: "bare", Type: "string", DefaultValue: "x", Description: "no group"},
	}

	var first bytes.Buffer
	require.NoError(t, generateYAMLContent(&first, registry))

	// Many runs, because map iteration order is random per-range: a single
	// repeat can coincidentally match even when ordering is unstable.
	for i := 0; i < 50; i++ {
		var next bytes.Buffer
		require.NoError(t, generateYAMLContent(&next, registry))
		require.Equal(t, first.String(), next.String(),
			"generation %d differed from the first — output is not deterministic", i)
	}
}
