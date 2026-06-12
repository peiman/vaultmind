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

		// path is produced by filepath.WalkDir rooted at vaultPath, not user input;
		// the symlink-TOCTOU that gosec G122 warns about is out of scope for a
		// single-user CLI reading its own local vault.
		content, readErr := os.ReadFile(path) // #nosec G304 G122
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
			Rule:    RuleUnknownType,
			Message: fmt.Sprintf("Type %q not in registry", noteType),
			Value:   noteType,
		})
		return true
	}

	hasIssue := false
	td, _ := reg.GetTypeDef(noteType)
	for _, req := range td.Required {
		if !reg.IsFieldPresent(fm, req) {
			result.Issues = append(result.Issues, ValidateIssue{
				Path: path, ID: id, Severity: "error",
				Rule:    RuleMissingRequired,
				Message: fmt.Sprintf("Type %q requires field %q", noteType, req),
				Field:   req,
			})
			hasIssue = true
		}
	}

	// Status aliasing intentionally not supported here — same deferral as
	// validate.go's fieldValue. The dedicated `status` column is populated
	// by the indexer from the canonical field name; live validation reads
	// the canonical key directly. Aliasing `status` is rare in practice;
	// defer until a real use case surfaces.
	if status, ok := fm["status"].(string); ok && status != "" {
		if len(td.Statuses) > 0 && !reg.ValidStatus(noteType, status) {
			result.Issues = append(result.Issues, ValidateIssue{
				Path: path, ID: id, Severity: "warning",
				Rule:    RuleInvalidStatus,
				Message: fmt.Sprintf("Status %q not valid for type %q", status, noteType),
				Field:   "status", Value: status,
			})
			hasIssue = true
		}
	}

	return hasIssue
}
