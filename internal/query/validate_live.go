package query

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/schema"
)

// ValidateLive walks vaultPath, parses each .md file's frontmatter, and runs
// schema rules against the live files on disk. It does NOT require an index.
//
// Rules evaluated: unknown_type, missing_required_field, invalid_status.
// Unparseable frontmatter is reported as invalid_frontmatter.
// The broken_reference rule is skipped — it requires the full link graph,
// which only the indexer produces.
func ValidateLive(vaultPath string, reg *schema.Registry) (*ValidateResult, error) {
	if _, err := os.Stat(vaultPath); err != nil {
		return nil, fmt.Errorf("vault path: %w", err)
	}

	result := &ValidateResult{Issues: []ValidateIssue{}}

	walkErr := filepath.WalkDir(vaultPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != vaultPath && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		// path is produced by filepath.WalkDir rooted at vaultPath, not user input.
		content, readErr := os.ReadFile(path) // #nosec G304
		if readErr != nil {
			return fmt.Errorf("reading %s: %w", path, readErr)
		}
		fm, _, parseErr := parser.ExtractFrontmatter(content)
		if parseErr != nil {
			result.FilesChecked++
			result.Issues = append(result.Issues, ValidateIssue{
				Path: path, Severity: "error",
				Rule: "invalid_frontmatter", Message: parseErr.Error(),
			})
			// Record the parse failure as an issue and keep walking —
			// a single unparseable file must not mask issues elsewhere.
			return nil //nolint:nilerr // intentional: issue captured in result
		}

		result.FilesChecked++

		isDomain, id, noteType := parser.ClassifyNote(fm)
		if !isDomain {
			result.Valid++
			return nil
		}

		if !validateDomainNote(path, id, noteType, fm, reg, result) {
			result.Valid++
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return result, nil
}

// validateDomainNote runs the DB-free rules on a parsed domain note and
// appends any issues to result. Returns true if at least one issue was found.
func validateDomainNote(
	path, id, noteType string,
	fm map[string]interface{},
	reg *schema.Registry,
	result *ValidateResult,
) bool {
	if !reg.HasType(noteType) {
		result.Issues = append(result.Issues, ValidateIssue{
			Path: path, ID: id, Severity: "warning",
			Rule:    "unknown_type",
			Message: fmt.Sprintf("Type %q not in registry", noteType),
			Value:   noteType,
		})
		return true
	}

	hasIssue := false
	td, _ := reg.GetTypeDef(noteType)
	for _, req := range td.Required {
		if !fmFieldPresent(fm, req) {
			result.Issues = append(result.Issues, ValidateIssue{
				Path: path, ID: id, Severity: "error",
				Rule:    "missing_required_field",
				Message: fmt.Sprintf("Type %q requires field %q", noteType, req),
				Field:   req,
			})
			hasIssue = true
		}
	}

	if status, ok := fm["status"].(string); ok && status != "" {
		if len(td.Statuses) > 0 && !reg.ValidStatus(noteType, status) {
			result.Issues = append(result.Issues, ValidateIssue{
				Path: path, ID: id, Severity: "warning",
				Rule:    "invalid_status",
				Message: fmt.Sprintf("Status %q not valid for type %q", status, noteType),
				Field:   "status", Value: status,
			})
			hasIssue = true
		}
	}

	return hasIssue
}

// fmFieldPresent reports whether field is present in fm with a non-empty value.
func fmFieldPresent(fm map[string]interface{}, field string) bool {
	raw, ok := fm[field]
	if !ok || raw == nil {
		return false
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case []interface{}:
		return len(v) > 0
	case map[string]interface{}:
		return len(v) > 0
	default:
		return true
	}
}
