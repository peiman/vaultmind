package query

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
)

// ValidateLive walks vaultPath, parses each .md file's frontmatter, and runs
// schema rules against the live files on disk. It does NOT require an index.
//
// Rules evaluated: unknown_type, missing_required_field, invalid_status.
// Unparseable frontmatter is reported as invalid_frontmatter, and a *.md
// symlink as skipped_symlink (a warning; it is never read).
// The broken_reference rule is skipped — it requires the full link graph,
// which only the indexer produces.
func ValidateLive(vaultPath string, reg *schema.Registry) (*ValidateResult, error) {
	if _, err := os.Stat(vaultPath); err != nil {
		return nil, fmt.Errorf("vault path: %w", err)
	}
	// The vault the operator named may itself be a link (a "current"
	// pointer). That is their choice of path, so it is resolved; WalkDir does
	// not descend into a root that is a link, and would report an empty vault
	// as clean. The never-follow rule is for links found INSIDE the vault.
	root, err := filepath.EvalSymlinks(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("vault path: %w", err)
	}

	result := &ValidateResult{Issues: []ValidateIssue{}}

	walkErr := filepath.WalkDir(root, func(walked string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		path := underNamedRoot(vaultPath, root, walked)
		if d.IsDir() {
			if walked != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		// Never follow a link: os.ReadFile would read whatever it points at, and
		// a parse error could echo it back (#194). Reported, not dropped.
		if _, skip := vault.SkipSymlink(root, walked, d); skip {
			result.Issues = append(result.Issues, ValidateIssue{
				Path: path, Severity: "warning", Rule: RuleSkippedSymlink,
				Message: "a symlink; not followed",
			})
			return nil
		}

		// walked comes from WalkDir under the vault root and was not a link when
		// listed. A file swapped for a link between that check and this read is
		// out of scope: it needs write access to the vault during the run.
		content, readErr := os.ReadFile(walked) // #nosec G304 G122
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

// underNamedRoot reports a walked path under the vault path the operator
// named, so issue paths read as they typed them, not as the link resolves.
func underNamedRoot(named, root, walked string) string {
	rel, err := filepath.Rel(root, walked)
	if err != nil {
		return walked
	}
	return filepath.Join(named, rel)
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
