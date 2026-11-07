package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ErrMalformedRole is returned when a role cannot be signed or parsed. An
// unknown kind, a missing variant field, an extra field, a roles value that is
// not an array, or a parameter list that is not an array of strings all fail
// closed with this error. An empty vouches_for or platforms list is valid and
// grants nothing; it is not this error.
const ErrMalformedRole = "registry: malformed agent role"

// RoleKind is the wire discriminant of a root-granted agent role.
type RoleKind string

const (
	// RoleKindBridge is a bridge role: it may vouch for the principals in vouches_for.
	RoleKindBridge RoleKind = "bridge"
	// RoleKindCodeSigning is a code-signing role: it may sign for the platforms list.
	RoleKindCodeSigning RoleKind = "code_signing"
)

// Role is one root-granted agent capability.
//
// The kind and its parameter list are unexported. NewBridgeRole and
// NewCodeSigningRole are the only constructors, so another package cannot build
// a third kind or a role that carries the other variant's parameters. The zero
// Role is not a role; signing it returns ErrMalformedRole. Wire-level rejects
// (unknown kind, missing field, extra field) are ParseRoles' job, because those
// states exist only in JSON.
//
// An empty parameter list is a real role that grants nothing. Constructors store
// nil as an empty slice so the wire form is [] and never null.
type Role struct {
	kind       RoleKind
	vouchesFor []string
	platforms  []string
}

// NewBridgeRole returns a bridge role. An empty or nil vouchesFor grants nothing.
func NewBridgeRole(vouchesFor []string) Role {
	return Role{kind: RoleKindBridge, vouchesFor: cloneStrings(vouchesFor)}
}

// NewCodeSigningRole returns a code-signing role. An empty or nil platforms grants nothing.
func NewCodeSigningRole(platforms []string) Role {
	return Role{kind: RoleKindCodeSigning, platforms: cloneStrings(platforms)}
}

// Kind returns the role discriminant. It is empty for the zero Role.
func (r Role) Kind() RoleKind { return r.kind }

// IsZero reports whether r is the zero Role, which is not a grantable capability.
func (r Role) IsZero() bool { return r.kind == "" }

// VouchesFor returns a copy of the bridge allowlist. It returns nil when r is
// not a bridge role. An empty non-nil slice means a bridge role that grants nothing.
func (r Role) VouchesFor() []string {
	if r.kind != RoleKindBridge {
		return nil
	}
	return cloneStrings(r.vouchesFor)
}

// Platforms returns a copy of the code-signing platform list. It returns nil
// when r is not a code-signing role. An empty non-nil slice grants nothing.
func (r Role) Platforms() []string {
	if r.kind != RoleKindCodeSigning {
		return nil
	}
	return cloneStrings(r.platforms)
}

// wellFormed reports whether r is one of the two variants and carries only that
// variant's parameter list. A same-package composite literal can still break
// this; signing refuses it rather than emitting a role the verifier must reject.
func (r Role) wellFormed() bool {
	switch r.kind {
	case RoleKindBridge:
		return len(r.platforms) == 0
	case RoleKindCodeSigning:
		return len(r.vouchesFor) == 0
	default:
		return false
	}
}

func cloneStrings(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func stringListOrEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// bridgeWire and codeSigningWire are separate types so a marshaled role object
// contains only its own parameter field. One struct with both fields would emit
// the other field (an extra field the verifier rejects) or, with omitempty,
// would drop an empty list (a missing field the verifier also rejects).
type bridgeWire struct {
	Kind       string   `json:"kind"`
	VouchesFor []string `json:"vouches_for"`
}

type codeSigningWire struct {
	Kind      string   `json:"kind"`
	Platforms []string `json:"platforms"`
}

func marshalRole(role Role) ([]byte, error) {
	if !role.wellFormed() {
		return nil, fmt.Errorf("%s", ErrMalformedRole)
	}
	switch role.kind {
	case RoleKindBridge:
		return json.Marshal(bridgeWire{Kind: string(RoleKindBridge), VouchesFor: stringListOrEmpty(role.vouchesFor)})
	case RoleKindCodeSigning:
		return json.Marshal(codeSigningWire{Kind: string(RoleKindCodeSigning), Platforms: stringListOrEmpty(role.platforms)})
	default:
		return nil, fmt.Errorf("%s", ErrMalformedRole)
	}
}

// ParseRoles decodes a binding's roles JSON. It is the single parser for
// VerifyAndLoad and for sign-registry (Principle 7 — one schema, both doors).
// Empty or absent input is no roles. A null, a non-array, an unknown kind, a
// missing variant field, an extra field, or a parameter list that is not an
// array of strings returns ErrMalformedRole.
func ParseRoles(raw []byte) ([]Role, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return []Role{}, nil
	}
	elems, err := decodeRoleArray(raw)
	if err != nil {
		return nil, err
	}
	roles := make([]Role, 0, len(elems))
	for _, elem := range elems {
		role, err := parseOneRole(elem)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func roleErr(err error) error {
	return fmt.Errorf("%s: %w", ErrMalformedRole, err)
}

func decodeRoleArray(raw []byte) ([]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, roleErr(err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return nil, fmt.Errorf("%s: roles must be an array", ErrMalformedRole)
	}
	var elems []json.RawMessage
	for dec.More() {
		var elem json.RawMessage
		if err := dec.Decode(&elem); err != nil {
			return nil, roleErr(err)
		}
		elems = append(elems, elem)
	}
	tok, err = dec.Token()
	if err != nil {
		return nil, roleErr(err)
	}
	delim, ok = tok.(json.Delim)
	if !ok || delim != ']' {
		return nil, fmt.Errorf("%s: roles must be an array", ErrMalformedRole)
	}
	if err := rejectTrailing(dec); err != nil {
		return nil, err
	}
	if elems == nil {
		return []json.RawMessage{}, nil
	}
	return elems, nil
}

func rejectTrailing(dec *json.Decoder) error {
	_, err := dec.Token()
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return roleErr(err)
	}
	return fmt.Errorf("%s: trailing data after roles", ErrMalformedRole)
}

// roleObject lists every field a role may carry. DisallowUnknownFields then
// rejects anything else. Which of vouches_for / platforms is legal depends on
// kind and is checked after the object decodes, so a bridge role that also
// carries platforms is an extra field rather than a silent drop.
type roleObject struct {
	Kind       string          `json:"kind"`
	VouchesFor json.RawMessage `json:"vouches_for"`
	Platforms  json.RawMessage `json:"platforms"`
}

func decodeRoleObject(raw json.RawMessage) (roleObject, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var obj roleObject
	if err := dec.Decode(&obj); err != nil {
		return roleObject{}, roleErr(err)
	}
	if rest := bytes.TrimSpace(raw[dec.InputOffset():]); len(rest) != 0 {
		return roleObject{}, fmt.Errorf("%s: trailing data in role", ErrMalformedRole)
	}
	return obj, nil
}

func parseOneRole(raw json.RawMessage) (Role, error) {
	obj, err := decodeRoleObject(raw)
	if err != nil {
		return Role{}, err
	}
	switch RoleKind(obj.Kind) {
	case RoleKindBridge:
		list, err := roleParams(RoleKindBridge, obj.VouchesFor, obj.Platforms)
		if err != nil {
			return Role{}, err
		}
		return NewBridgeRole(list), nil
	case RoleKindCodeSigning:
		list, err := roleParams(RoleKindCodeSigning, obj.Platforms, obj.VouchesFor)
		if err != nil {
			return Role{}, err
		}
		return NewCodeSigningRole(list), nil
	default:
		return Role{}, fmt.Errorf("%s: unknown kind %q", ErrMalformedRole, obj.Kind)
	}
}

// roleParams reads the active variant's string list. inactive must be absent;
// a present value is the other variant's field, which is an extra field.
func roleParams(kind RoleKind, active, inactive json.RawMessage) ([]string, error) {
	if len(inactive) != 0 {
		return nil, fmt.Errorf("%s: extra field on %s role", ErrMalformedRole, kind)
	}
	if len(active) == 0 {
		return nil, fmt.Errorf("%s: missing parameter list on %s role", ErrMalformedRole, kind)
	}
	return decodeStringList(active)
}

func decodeStringList(raw json.RawMessage) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, roleErr(err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return nil, fmt.Errorf("%s: role parameter must be an array of strings", ErrMalformedRole)
	}
	out := []string{}
	for dec.More() {
		var s string
		if err := dec.Decode(&s); err != nil {
			return nil, fmt.Errorf("%s: role parameter must be an array of strings", ErrMalformedRole)
		}
		out = append(out, s)
	}
	tok, err = dec.Token()
	if err != nil {
		return nil, roleErr(err)
	}
	delim, ok = tok.(json.Delim)
	if !ok || delim != ']' {
		return nil, fmt.Errorf("%s: role parameter must be an array of strings", ErrMalformedRole)
	}
	if err := rejectTrailing(dec); err != nil {
		return nil, err
	}
	return out, nil
}

// MarshalRoles encodes roles as a JSON array for embedding in a registry
// binding. An empty list encodes as an empty RawMessage so the binding's
// omitempty tag drops the key. Array order is preserved. Object key order is
// not significant: the registry canonicalizer sorts keys.
func MarshalRoles(roles []Role) (json.RawMessage, error) {
	if len(roles) == 0 {
		return json.RawMessage{}, nil
	}
	out := make([]byte, 0, 32*len(roles))
	out = append(out, '[')
	for i, role := range roles {
		if i > 0 {
			out = append(out, ',')
		}
		encoded, err := marshalRole(role)
		if err != nil {
			return nil, err
		}
		out = append(out, encoded...)
	}
	out = append(out, ']')
	return out, nil
}
