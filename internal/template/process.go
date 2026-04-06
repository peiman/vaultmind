package template

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// knownVars is the set of variable names recognised in templates.
var knownVars = map[string]struct{}{
	"id":         {},
	"type":       {},
	"title":      {},
	"created":    {},
	"updated":    {},
	"vm_updated": {},
	"date":       {},
	"datetime":   {},
	"path":       {},
}

// varPattern matches <%=word%> placeholders.
var varPattern = regexp.MustCompile(`<%=(\w+)%>`)

// ProcessConfig holds all inputs needed to process a note template.
type ProcessConfig struct {
	VaultPath      string            // absolute path to vault root (for metadata)
	Path           string            // vault-relative path of the new note
	Type           string            // note type (e.g. "project", "decision")
	Fields         map[string]string // field overrides (includes optional "title", "id", …)
	Body           string            // explicit body override; empty means use template body
	TemplatePath   string            // absolute path to the template file
	RequiredFields []string          // type-specific required fields (for minimal fallback)
}

// ProcessResult is the output of a successful Process call.
type ProcessResult struct {
	Content          []byte                 // serialized YAML-frontmatter + body
	ID               string                 // final note ID
	Path             string                 // echoes ProcessConfig.Path
	Warnings         []string               // non-fatal issues (e.g. unknown template vars, missing template)
	FinalFrontmatter map[string]interface{} // the final frontmatter map after all substitutions and overrides
}

// SubstituteVars replaces <%=name%> placeholders in content using vars.
// Unknown variable names are left as-is and reported in warnings.
func SubstituteVars(content string, vars map[string]string) (string, []string) {
	var warnings []string
	result := varPattern.ReplaceAllStringFunc(content, func(match string) string {
		sub := varPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		name := sub[1]
		if _, ok := knownVars[name]; !ok {
			warnings = append(warnings, fmt.Sprintf("unrecognized template variable: %q", name))
			return match
		}
		if val, ok := vars[name]; ok {
			return val
		}
		return match
	})
	return result, warnings
}

// Process executes the full note-creation pipeline:
//  1. Load template from disk (fallback to minimal note if missing)
//  2. Build variable map and substitute placeholders
//  3. Parse frontmatter
//  4. Apply field overrides from cfg.Fields
//  5. Ensure core fields (id, type, created, vm_updated) are present
//  6. Override body if cfg.Body is set
//  7. Serialize and return
func Process(cfg ProcessConfig) (*ProcessResult, error) {
	now := time.Now().UTC()
	dateStr := now.Format("2006-01-02")
	datetimeStr := now.Format(time.RFC3339)

	// Determine the effective ID (may be overridden by a Fields["id"] later).
	generatedID := GenerateID(cfg.Path, cfg.Type)

	// Resolve title: prefer explicit override, fall back to filename without extension.
	title := cfg.Fields["title"]
	if title == "" {
		base := filepath.Base(cfg.Path)
		base = strings.TrimSuffix(base, filepath.Ext(base))
		title = base
	}

	// Build variable substitution map.
	vars := map[string]string{
		"id":         generatedID,
		"type":       cfg.Type,
		"title":      title,
		"created":    dateStr,
		"updated":    datetimeStr,
		"vm_updated": datetimeStr,
		"date":       dateStr,
		"datetime":   datetimeStr,
		"path":       cfg.Path,
	}

	var warnings []string

	// Load and substitute template content.
	rawBytes, err := os.ReadFile(cfg.TemplatePath)
	var templateContent string
	if err != nil {
		// Template not found — generate a minimal fallback (includes required fields).
		warnings = append(warnings, fmt.Sprintf("template not found at %q: using minimal fallback", cfg.TemplatePath))
		templateContent = buildMinimalTemplate(cfg.RequiredFields)
	} else {
		templateContent = string(rawBytes)
	}

	substituted, subWarnings := SubstituteVars(templateContent, vars)
	warnings = append(warnings, subWarnings...)

	// Parse frontmatter and body from the substituted content.
	fm, body, err := parseFrontmatterAndBody(substituted)
	if err != nil {
		return nil, fmt.Errorf("parsing template frontmatter: %w", err)
	}
	if fm == nil {
		fm = make(map[string]interface{})
	}

	// Apply field overrides from cfg.Fields (these win over template values).
	for k, v := range cfg.Fields {
		fm[k] = v
	}

	// Determine final ID (may have been overridden via cfg.Fields["id"]).
	finalID := generatedID
	if idVal, ok := fm["id"]; ok {
		if idStr, ok := idVal.(string); ok && idStr != "" {
			finalID = idStr
		}
	}

	// Ensure core fields are always present.
	if _, ok := fm["id"]; !ok {
		fm["id"] = finalID
	}
	if _, ok := fm["type"]; !ok {
		fm["type"] = cfg.Type
	}
	if _, ok := fm["created"]; !ok {
		fm["created"] = dateStr
	}
	if _, ok := fm["vm_updated"]; !ok {
		fm["vm_updated"] = datetimeStr
	}

	// Apply body override if provided.
	if cfg.Body != "" {
		body = cfg.Body
	}

	// Serialize to YAML frontmatter + body.
	content, err := serialize(fm, body)
	if err != nil {
		return nil, fmt.Errorf("serializing note: %w", err)
	}

	return &ProcessResult{
		Content:          content,
		ID:               finalID,
		Path:             cfg.Path,
		Warnings:         warnings,
		FinalFrontmatter: fm,
	}, nil
}

// buildMinimalTemplate returns a bare-bones template string with all core fields.
// requiredFields lists any additional type-specific required fields to include
// with empty string defaults.
func buildMinimalTemplate(requiredFields []string) string {
	var sb strings.Builder
	sb.WriteString("---\nid: <%=id%>\ntype: <%=type%>\ntitle: <%=title%>\ncreated: <%=created%>\nvm_updated: <%=vm_updated%>\n")
	for _, f := range requiredFields {
		// Only add fields that are not already core fields in the template.
		switch f {
		case "id", "type", "title", "created", "vm_updated", "updated":
			// already included
		default:
			sb.WriteString(f)
			sb.WriteString(": \n")
		}
	}
	sb.WriteString("---\n")
	return sb.String()
}

// parseFrontmatterAndBody splits content on YAML frontmatter delimiters.
// It returns (nil, fullContent, nil) when no frontmatter is found.
func parseFrontmatterAndBody(content string) (map[string]interface{}, string, error) {
	const delim = "---"
	if !strings.HasPrefix(content, delim+"\n") && !strings.HasPrefix(content, delim+"\r\n") {
		return nil, content, nil
	}

	rest := content[len(delim):]
	closeIdx := findDelimiterClose(rest)
	if closeIdx < 0 {
		return nil, content, nil
	}

	yamlBlock := rest[1 : closeIdx+1] // strip leading newline; stop before closing ---
	afterClose := rest[closeIdx+1+len(delim):]
	body := strings.TrimPrefix(afterClose, "\r\n")
	body = strings.TrimPrefix(body, "\n")

	if strings.TrimSpace(yamlBlock) == "" {
		return nil, body, nil
	}

	var fm map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return nil, "", fmt.Errorf("yaml unmarshal: %w", err)
	}
	return fm, body, nil
}

// findDelimiterClose locates the closing "---" line inside rest (which starts
// just after the opening "---").  Returns the index of the '\n' that precedes
// the closing delimiter, or -1 if not found.
func findDelimiterClose(rest string) int {
	const delim = "---"
	searchFrom := 0
	for {
		idx := strings.Index(rest[searchFrom:], "\n"+delim)
		if idx < 0 {
			return -1
		}
		abs := searchFrom + idx
		after := abs + 1 + len(delim)
		if after >= len(rest) || rest[after] == '\n' || rest[after] == '\r' {
			return abs
		}
		searchFrom = abs + 1
	}
}

// serialize converts a frontmatter map and body into a byte slice of the form:
//
//	---
//	<yaml>
//	---
//	<body>
func serialize(fm map[string]interface{}, body string) ([]byte, error) {
	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return nil, err
	}
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.Write(yamlBytes)
	sb.WriteString("---\n")
	if body != "" {
		if !strings.HasPrefix(body, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString(body)
	}
	return []byte(sb.String()), nil
}
