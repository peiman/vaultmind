package registry

import (
	"encoding/json"
	"fmt"
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
