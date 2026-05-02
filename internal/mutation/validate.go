package mutation

import (
	"fmt"
	"slices"

	"github.com/peiman/vaultmind/internal/schema"
)

var immutableFields = []string{"id", "type"}

// ValidateMutation checks a MutationRequest against schema rules before any
// file is modified. It returns a *MutationError with a structured code on
// failure, or nil when the request is safe to apply.
func ValidateMutation(req MutationRequest, note ParsedNoteInfo, reg *schema.Registry) error {
	if req.Op == OpNormalize {
		return nil
	}
	if !note.IsDomain {
		return &MutationError{Code: "not_domain_note", Message: "operation requires a domain note (with id and type)"}
	}
	switch req.Op {
	case OpSet:
		return validateSet(req, note, reg)
	case OpUnset:
		return validateUnset(req, note, reg)
	case OpMerge:
		return validateMerge(req, note, reg)
	}
	return nil
}

func validateSet(req MutationRequest, note ParsedNoteInfo, reg *schema.Registry) error {
	if slices.Contains(immutableFields, req.Key) {
		return &MutationError{Code: "immutable_field", Message: fmt.Sprintf("field %q cannot be modified", req.Key), Field: req.Key}
	}
	if !req.AllowExtra && !reg.IsFieldAllowed(note.Type, req.Key) {
		return &MutationError{Code: "unknown_key", Message: fmt.Sprintf("field %q is not allowed for type %q", req.Key, note.Type), Field: req.Key}
	}
	if req.Key == "status" {
		if s, ok := req.Value.(string); ok {
			if !reg.ValidStatus(note.Type, s) {
				return &MutationError{Code: "invalid_status", Message: fmt.Sprintf("status %q is not valid for type %q", s, note.Type), Field: "status"}
			}
		}
	}
	return nil
}

func validateUnset(req MutationRequest, note ParsedNoteInfo, reg *schema.Registry) error {
	if slices.Contains(immutableFields, req.Key) {
		return &MutationError{Code: "immutable_field", Message: fmt.Sprintf("field %q cannot be modified", req.Key), Field: req.Key}
	}
	// Determine which canonical required field, if any, this unset would
	// affect. If req.Key is itself canonical and required, it's the obvious
	// candidate. If req.Key is a registered alias, the canonical it aliases
	// might be required. Either way, the question is the same: after the
	// unset, is the canonical's role still satisfied by another key (the
	// canonical itself or another registered alias) on this note?
	required := reg.RequiredFields(note.Type)
	canonical := req.Key
	if !slices.Contains(required, canonical) {
		// req.Key is not directly required; check whether it's an alias of
		// some required canonical.
		for _, c := range required {
			if reg.IsAlias(c, req.Key) {
				canonical = c
				break
			}
		}
	}
	if !slices.Contains(required, canonical) {
		// Neither canonical-required nor an alias of a required canonical.
		// Optional/unknown fields can be unset freely.
		return nil
	}
	// canonical is required for this type. After unsetting req.Key, would
	// the canonical's role still be satisfied? Yes iff another key on the
	// note carries the canonical's value — either the canonical itself or
	// any registered alias other than req.Key.
	candidates := reg.FieldNamesForLookup(canonical)
	for _, name := range candidates {
		if name == req.Key {
			continue
		}
		if slices.Contains(note.Keys, name) {
			return nil
		}
	}
	return &MutationError{Code: "missing_required_field", Message: fmt.Sprintf("field %q is required for type %q and cannot be removed", req.Key, note.Type), Field: req.Key}
}

func validateMerge(req MutationRequest, note ParsedNoteInfo, reg *schema.Registry) error {
	for key, value := range req.Fields {
		subReq := MutationRequest{Op: OpSet, Key: key, Value: value, AllowExtra: req.AllowExtra}
		if err := validateSet(subReq, note, reg); err != nil {
			return err
		}
	}
	return nil
}
