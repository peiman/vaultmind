package plan

import (
	"fmt"
	"sort"

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
			errs = append(errs, shapeErrors(pf, op.Frontmatter, reg)...)
		}
	}
	return errs
}

// shapeErrors checks each frontmatter value against its field's shape, in key
// order so the report is stable. note_create writes the map as given, so this
// is the only check between a wrong-shaped value and the file.
func shapeErrors(pf string, fm map[string]interface{}, reg *schema.Registry) []OpError {
	keys := make([]string, 0, len(fm))
	for k := range fm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var errs []OpError
	for _, k := range keys {
		if msg := reg.ValueShapeError(k, fm[k]); msg != "" {
			errs = append(errs, OpError{Code: "invalid_type", Message: pf + ": " + msg})
		}
	}
	return errs
}
