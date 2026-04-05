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
	required := reg.RequiredFields(note.Type)
	if slices.Contains(required, req.Key) {
		return &MutationError{Code: "missing_required_field", Message: fmt.Sprintf("field %q is required for type %q and cannot be removed", req.Key, note.Type), Field: req.Key}
	}
	return nil
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
