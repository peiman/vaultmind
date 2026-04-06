package plan

import (
	"fmt"

	"github.com/peiman/vaultmind/internal/schema"
)

// ValidatePlan checks a Plan for structural validity against the type registry.
// It returns a slice of OpErrors describing every problem found.
// An empty slice means the plan is valid.
func ValidatePlan(p Plan, reg *schema.Registry) []OpError {
	var errs []OpError
	if p.Version != 1 {
		errs = append(errs, OpError{Code: "unsupported_version", Message: fmt.Sprintf("version %d not supported", p.Version)})
		return errs
	}
	if len(p.Operations) == 0 {
		errs = append(errs, OpError{Code: "empty_plan", Message: "plan has no operations"})
		return errs
	}
	for i, op := range p.Operations {
		pf := fmt.Sprintf("operation[%d]", i)
		if !KnownOps[op.Op] {
			errs = append(errs, OpError{Code: "unknown_operation", Message: fmt.Sprintf("%s: unknown op %q", pf, op.Op)})
			continue
		}
		switch op.Op {
		case OpFrontmatterSet:
			if op.Target == "" {
				errs = append(errs, OpError{Code: "missing_field", Message: pf + ": requires target"})
			}
			if op.Key == "" {
				errs = append(errs, OpError{Code: "missing_field", Message: pf + ": requires key"})
			}
			if op.Value == nil {
				errs = append(errs, OpError{Code: "missing_field", Message: pf + ": requires value"})
			}
		case OpFrontmatterUnset:
			if op.Target == "" {
				errs = append(errs, OpError{Code: "missing_field", Message: pf + ": requires target"})
			}
			if op.Key == "" {
				errs = append(errs, OpError{Code: "missing_field", Message: pf + ": requires key"})
			}
		case OpFrontmatterMerge:
			if op.Target == "" {
				errs = append(errs, OpError{Code: "missing_field", Message: pf + ": requires target"})
			}
			if len(op.Fields) == 0 {
				errs = append(errs, OpError{Code: "missing_field", Message: pf + ": requires fields"})
			}
		case OpGeneratedRegion:
			if op.Target == "" {
				errs = append(errs, OpError{Code: "missing_field", Message: pf + ": requires target"})
			}
			if op.SectionKey == "" {
				errs = append(errs, OpError{Code: "missing_field", Message: pf + ": requires section_key"})
			}
			if op.Template == "" {
				errs = append(errs, OpError{Code: "missing_field", Message: pf + ": requires template"})
			}
		case OpNoteCreate:
			if op.Path == "" {
				errs = append(errs, OpError{Code: "missing_field", Message: pf + ": requires path"})
			}
			if op.Type == "" {
				errs = append(errs, OpError{Code: "missing_field", Message: pf + ": requires type"})
			}
			if op.Frontmatter == nil {
				errs = append(errs, OpError{Code: "missing_field", Message: pf + ": requires frontmatter"})
			}
			if op.Type != "" && !reg.HasType(op.Type) {
				errs = append(errs, OpError{Code: "unknown_type", Message: fmt.Sprintf("%s: type %q not in registry", pf, op.Type)})
			}
		}
	}
	return errs
}
