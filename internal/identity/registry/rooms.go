package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Room-policy and DM-policy rejects. One constant per schema, the same grain
// as ErrMalformedRole: every rule below fails the whole registry with that
// constant, on the sign input and on VerifyAndLoad.
const (
	// ErrMalformedRoom is returned when rooms is not a valid room-policy array.
	// The name, controller, member, access, approval_required, bound, and
	// unknown-field rules all use this constant.
	ErrMalformedRoom = "registry: malformed room"
	// ErrMalformedDMPolicy is returned when dms is present and is not exactly
	// "allow" or "agents_off".
	ErrMalformedDMPolicy = "registry: malformed DM policy"

	// DMPolicyAllow is the explicit DM policy that permits DMs. The zero
	// DMPolicy means the same thing and is omitted from the signed bytes.
	DMPolicyAllow = "allow"
	// DMPolicyAgentsOff is the DM policy that turns agent DMs off.
	DMPolicyAgentsOff = "agents_off"
)

const (
	maxRooms            = 256
	maxRoomMembers      = 256
	maxApprovalRequired = 32
	// maxApprovalNameLen is the action-name bound ^[a-z][a-z0-9_-]{0,31}$.
	maxApprovalNameLen      = 32
	maxRoutingNameLen       = 128
	maxIdentityLen          = 128
	maxIdentityKindLen      = 32
	roomNameForbiddenSuffix = ".control"
)

// Access is one room-member capability. Only read and post exist.
type Access string

const (
	// AccessRead lets the member read the room.
	AccessRead Access = "read"
	// AccessPost lets the member post to the room.
	AccessPost Access = "post"
)

// DMPolicy is the registry's DM policy.
//
// The zero value is absent. Absent means allow, and MarshalDMPolicy omits the
// key so a registry that never set a policy stays byte-identical to one signed
// before the field existed. NewDMPolicy is the only constructor for a present
// policy, so a value other than allow or agents_off cannot be represented.
type DMPolicy struct {
	set   bool
	value string
}

// NewDMPolicy returns a present DM policy. value must be allow or agents_off.
func NewDMPolicy(value string) (DMPolicy, error) {
	switch value {
	case DMPolicyAllow, DMPolicyAgentsOff:
		return DMPolicy{set: true, value: value}, nil
	default:
		return DMPolicy{}, fmt.Errorf("%s: dms %q", ErrMalformedDMPolicy, value)
	}
}

// Absent reports whether the policy was omitted. Absent means allow.
func (p DMPolicy) Absent() bool { return !p.set }

// Effective returns the policy a consumer applies. Absent returns allow.
func (p DMPolicy) Effective() string {
	if !p.set {
		return DMPolicyAllow
	}
	return p.value
}

// RoomMember is one principal's access to a room. NewRoomMember is the only
// constructor, so an empty access list, an unknown capability, or a repeated
// capability cannot be represented. Access order is the order that was signed.
type RoomMember struct {
	principal string
	access    []Access
}

// NewRoomMember returns a member. principal follows validate_identity and must
// be kind agent or human. access is non-empty, has no repeats, and each entry
// is read or post. Order is preserved.
func NewRoomMember(principal string, access []string) (RoomMember, error) {
	if err := validPrincipal(principal); err != nil {
		return RoomMember{}, err
	}
	parsed, err := parseAccessValues(access)
	if err != nil {
		return RoomMember{}, err
	}
	return RoomMember{principal: principal, access: parsed}, nil
}

// Principal returns the member's identity.
func (m RoomMember) Principal() string { return m.principal }

// Access returns a copy of the member's capabilities, in signed order.
func (m RoomMember) Access() []Access {
	if len(m.access) == 0 {
		return nil
	}
	out := make([]Access, len(m.access))
	copy(out, m.access)
	return out
}

func (m RoomMember) clone() RoomMember {
	return RoomMember{principal: m.principal, access: m.Access()}
}

// Room is one root-signed room. NewRoom is the only constructor. Name, controller,
// members, and approval_required are unexported so a caller cannot mutate a
// room into a shape the verifier must reject.
type Room struct {
	name       string
	controller string
	members    []RoomMember
	approval   []string
}

// NewRoom returns a room. name follows validate_routing_name and must not end
// with ".control". controller follows the member principal rule. members is
// optional, at most 256, unique by principal, and no member equals the
// controller. approval is optional, at most 32, unique, each matching
// ^[a-z][a-z0-9_-]{0,31}$. List order is preserved. Uniqueness of room names
// across a registry is ParseRooms' job.
func NewRoom(name, controller string, members []RoomMember, approval []string) (Room, error) {
	if err := validateOneRoom(name, controller, members, approval); err != nil {
		return Room{}, err
	}
	return Room{
		name:       name,
		controller: controller,
		members:    cloneMembers(members),
		approval:   cloneOptionalStrings(approval),
	}, nil
}

// Name returns the room's routing name.
func (r Room) Name() string { return r.name }

// Controller returns the room's controller principal.
func (r Room) Controller() string { return r.controller }

// Members returns a copy of the room's members, in signed order.
func (r Room) Members() []RoomMember {
	return cloneMembers(r.members)
}

// ApprovalRequired returns a copy of the approval names, in signed order.
func (r Room) ApprovalRequired() []string {
	return cloneOptionalStrings(r.approval)
}

// ParseRooms decodes a registry's rooms JSON. It is the single parser for
// VerifyAndLoad and for sign-registry (Principle 7 — one schema, both doors).
// Empty or absent input is no rooms. A null, a non-array, an unknown field,
// or any room rule below returns ErrMalformedRoom.
func ParseRooms(raw []byte) ([]Room, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	elems, err := decodeRawArray(raw, "rooms")
	if err != nil {
		return nil, err
	}
	if len(elems) > maxRooms {
		return nil, fmt.Errorf("%s: too many rooms", ErrMalformedRoom)
	}
	rooms := make([]Room, 0, len(elems))
	for _, elem := range elems {
		room, err := parseOneRoom(elem)
		if err != nil {
			return nil, err
		}
		rooms = append(rooms, room)
	}
	if err := validateRoomNames(rooms); err != nil {
		return nil, err
	}
	return rooms, nil
}

// ParseDMPolicy decodes a registry's dms JSON. Empty or absent input is the
// absent policy (allow, omitted when signed). Any other value that is not
// exactly "allow" or "agents_off" returns ErrMalformedDMPolicy.
func ParseDMPolicy(raw []byte) (DMPolicy, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return DMPolicy{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var value string
	if err := dec.Decode(&value); err != nil {
		return DMPolicy{}, fmt.Errorf("%s: dms must be a string", ErrMalformedDMPolicy)
	}
	if err := rejectTrailingValue(dec, ErrMalformedDMPolicy, "dms"); err != nil {
		return DMPolicy{}, err
	}
	return NewDMPolicy(value)
}

// MarshalRooms encodes rooms for the registry wire. An empty list encodes as
// an empty RawMessage so the registry's omitempty tag drops the key. Array
// order is preserved. A room that fails its own rules returns ErrMalformedRoom.
func MarshalRooms(rooms []Room) (json.RawMessage, error) {
	if len(rooms) == 0 {
		return json.RawMessage{}, nil
	}
	if err := validateRoomSet(rooms); err != nil {
		return nil, err
	}
	wires := make([]roomWire, len(rooms))
	for i, room := range rooms {
		wires[i] = room.wire()
	}
	raw, err := json.Marshal(wires)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMalformedRoom, err)
	}
	return raw, nil
}

// MarshalDMPolicy encodes a present DM policy. The absent policy encodes as an
// empty RawMessage so the key is omitted. A present value other than the two
// allowed strings returns ErrMalformedDMPolicy.
func MarshalDMPolicy(p DMPolicy) (json.RawMessage, error) {
	if !p.set {
		return json.RawMessage{}, nil
	}
	if p.value != DMPolicyAllow && p.value != DMPolicyAgentsOff {
		return nil, fmt.Errorf("%s: dms %q", ErrMalformedDMPolicy, p.value)
	}
	raw, err := json.Marshal(p.value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMalformedDMPolicy, err)
	}
	return raw, nil
}

type roomWire struct {
	ApprovalRequired []string     `json:"approval_required,omitempty"`
	Controller       string       `json:"controller"`
	Members          []memberWire `json:"members,omitempty"`
	Name             string       `json:"name"`
}

type memberWire struct {
	Access    []string `json:"access"`
	Principal string   `json:"principal"`
}

func (r Room) wire() roomWire {
	var members []memberWire
	if len(r.members) > 0 {
		members = make([]memberWire, len(r.members))
		for i, m := range r.members {
			access := make([]string, len(m.access))
			for j, a := range m.access {
				access[j] = string(a)
			}
			members[i] = memberWire{Access: access, Principal: m.principal}
		}
	}
	return roomWire{
		ApprovalRequired: cloneOptionalStrings(r.approval),
		Controller:       r.controller,
		Members:          members,
		Name:             r.name,
	}
}

type roomObject struct {
	Name             *string         `json:"name"`
	Controller       *string         `json:"controller"`
	Members          json.RawMessage `json:"members"`
	ApprovalRequired json.RawMessage `json:"approval_required"`
}

type memberObject struct {
	Principal *string         `json:"principal"`
	Access    json.RawMessage `json:"access"`
}

func parseOneRoom(raw json.RawMessage) (Room, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var obj roomObject
	if err := dec.Decode(&obj); err != nil {
		return Room{}, fmt.Errorf("%s: %w", ErrMalformedRoom, err)
	}
	if rest := bytes.TrimSpace(raw[dec.InputOffset():]); len(rest) != 0 {
		return Room{}, fmt.Errorf("%s: trailing data in room", ErrMalformedRoom)
	}
	if obj.Name == nil || obj.Controller == nil {
		return Room{}, fmt.Errorf("%s: room requires name and controller", ErrMalformedRoom)
	}
	members, err := parseMembers(obj.Members)
	if err != nil {
		return Room{}, err
	}
	approval, err := parseApproval(obj.ApprovalRequired)
	if err != nil {
		return Room{}, err
	}
	return NewRoom(*obj.Name, *obj.Controller, members, approval)
}

func parseMembers(raw json.RawMessage) ([]RoomMember, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	elems, err := decodeRawArray(raw, "members")
	if err != nil {
		return nil, err
	}
	if len(elems) > maxRoomMembers {
		return nil, fmt.Errorf("%s: too many members", ErrMalformedRoom)
	}
	members := make([]RoomMember, 0, len(elems))
	for _, elem := range elems {
		member, err := parseOneMember(elem)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, nil
}

func parseOneMember(raw json.RawMessage) (RoomMember, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var obj memberObject
	if err := dec.Decode(&obj); err != nil {
		return RoomMember{}, fmt.Errorf("%s: %w", ErrMalformedRoom, err)
	}
	if rest := bytes.TrimSpace(raw[dec.InputOffset():]); len(rest) != 0 {
		return RoomMember{}, fmt.Errorf("%s: trailing data in member", ErrMalformedRoom)
	}
	if obj.Principal == nil {
		return RoomMember{}, fmt.Errorf("%s: member requires principal", ErrMalformedRoom)
	}
	access, err := parseStringArray(obj.Access, "access")
	if err != nil {
		return RoomMember{}, err
	}
	return NewRoomMember(*obj.Principal, access)
}

func parseApproval(raw json.RawMessage) ([]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	return parseStringArray(raw, "approval_required")
}

func decodeRawArray(raw []byte, what string) ([]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("%s: %s: %w", ErrMalformedRoom, what, err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return nil, fmt.Errorf("%s: %s must be an array", ErrMalformedRoom, what)
	}
	var elems []json.RawMessage
	for dec.More() {
		var elem json.RawMessage
		if err := dec.Decode(&elem); err != nil {
			return nil, fmt.Errorf("%s: %s: %w", ErrMalformedRoom, what, err)
		}
		elems = append(elems, elem)
	}
	if err := closeArray(dec, what); err != nil {
		return nil, err
	}
	if err := rejectTrailingValue(dec, ErrMalformedRoom, what); err != nil {
		return nil, err
	}
	return elems, nil
}

func closeArray(dec *json.Decoder, what string) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("%s: %s: %w", ErrMalformedRoom, what, err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != ']' {
		return fmt.Errorf("%s: %s must be an array", ErrMalformedRoom, what)
	}
	return nil
}

func rejectTrailingValue(dec *json.Decoder, kind, what string) error {
	var extra any
	err := dec.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %s: %w", kind, what, err)
	}
	return fmt.Errorf("%s: trailing data after %s", kind, what)
}

func parseStringArray(raw json.RawMessage, what string) ([]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("%s: %s is required", ErrMalformedRoom, what)
	}
	elems, err := decodeRawArray(raw, what)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(elems))
	for _, elem := range elems {
		var s string
		dec := json.NewDecoder(bytes.NewReader(elem))
		if err := dec.Decode(&s); err != nil {
			return nil, fmt.Errorf("%s: %s must be an array of strings", ErrMalformedRoom, what)
		}
		if err := rejectTrailingValue(dec, ErrMalformedRoom, what); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func validateRoomSet(rooms []Room) error {
	if len(rooms) > maxRooms {
		return fmt.Errorf("%s: too many rooms", ErrMalformedRoom)
	}
	for _, room := range rooms {
		if err := validateOneRoom(room.name, room.controller, room.members, room.approval); err != nil {
			return err
		}
	}
	return validateRoomNames(rooms)
}

func validateRoomNames(rooms []Room) error {
	seen := make(map[string]string, len(rooms))
	for _, room := range rooms {
		fold := asciiFold(room.name)
		prev, ok := seen[fold]
		if !ok {
			seen[fold] = room.name
			continue
		}
		if prev == room.name {
			return fmt.Errorf("%s: duplicate room name", ErrMalformedRoom)
		}
		return fmt.Errorf("%s: room names equal under ASCII case-folding", ErrMalformedRoom)
	}
	return nil
}

func validateOneRoom(name, controller string, members []RoomMember, approval []string) error {
	if err := validRoutingName(name); err != nil {
		return err
	}
	if err := validPrincipal(controller); err != nil {
		return err
	}
	if err := validateMembers(controller, members); err != nil {
		return err
	}
	return validateApproval(approval)
}

func validateMembers(controller string, members []RoomMember) error {
	if len(members) > maxRoomMembers {
		return fmt.Errorf("%s: too many members", ErrMalformedRoom)
	}
	seen := make(map[string]struct{}, len(members))
	for _, m := range members {
		if err := validPrincipal(m.principal); err != nil {
			return err
		}
		if _, err := parseAccessValues(accessStrings(m.access)); err != nil {
			return err
		}
		if m.principal == controller {
			return fmt.Errorf("%s: member equals controller", ErrMalformedRoom)
		}
		if _, ok := seen[m.principal]; ok {
			return fmt.Errorf("%s: duplicate member", ErrMalformedRoom)
		}
		seen[m.principal] = struct{}{}
	}
	return nil
}

func validateApproval(approval []string) error {
	if len(approval) > maxApprovalRequired {
		return fmt.Errorf("%s: too many approval_required names", ErrMalformedRoom)
	}
	seen := make(map[string]struct{}, len(approval))
	for _, name := range approval {
		if !validApprovalName(name) {
			return fmt.Errorf("%s: approval_required %q", ErrMalformedRoom, name)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("%s: repeated approval_required", ErrMalformedRoom)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func validRoutingName(s string) error {
	if len(s) < 1 || len(s) > maxRoutingNameLen {
		return fmt.Errorf("%s: room name length", ErrMalformedRoom)
	}
	if !asciiAlnum(s[0]) || !routingBytes(s) {
		return fmt.Errorf("%s: room name has an invalid character", ErrMalformedRoom)
	}
	if strings.HasSuffix(s, roomNameForbiddenSuffix) {
		return fmt.Errorf("%s: room name ends with .control", ErrMalformedRoom)
	}
	return nil
}

func validPrincipal(s string) error {
	if !identityShape(s) {
		return fmt.Errorf("%s: principal is not an identity", ErrMalformedRoom)
	}
	kind, _, _ := strings.Cut(s, ":")
	if kind != "agent" && kind != "human" {
		return fmt.Errorf("%s: principal kind must be agent or human", ErrMalformedRoom)
	}
	return nil
}

func identityShape(s string) bool {
	if len(s) == 0 || len(s) > maxIdentityLen {
		return false
	}
	kind, name, ok := strings.Cut(s, ":")
	if !ok || !validKind(kind) || !validIdentityName(name) {
		return false
	}
	return true
}

func validKind(kind string) bool {
	if len(kind) < 1 || len(kind) > maxIdentityKindLen || kind[0] < 'a' || kind[0] > 'z' {
		return false
	}
	for i := 0; i < len(kind); i++ {
		c := kind[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

func validIdentityName(name string) bool {
	if len(name) == 0 || !asciiAlnum(name[0]) {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !nameByte(name[i]) {
			return false
		}
	}
	return true
}

func parseAccessValues(access []string) ([]Access, error) {
	if len(access) == 0 {
		return nil, fmt.Errorf("%s: empty access", ErrMalformedRoom)
	}
	seen := make(map[Access]struct{}, len(access))
	out := make([]Access, len(access))
	for i, raw := range access {
		a := Access(raw)
		if a != AccessRead && a != AccessPost {
			return nil, fmt.Errorf("%s: unknown access %q", ErrMalformedRoom, raw)
		}
		if _, ok := seen[a]; ok {
			return nil, fmt.Errorf("%s: repeated access", ErrMalformedRoom)
		}
		seen[a] = struct{}{}
		out[i] = a
	}
	return out, nil
}

func validApprovalName(s string) bool {
	if len(s) < 1 || len(s) > maxApprovalNameLen {
		return false
	}
	if s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

func routingBytes(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !nameByte(c) && c != ':' && c != '/' {
			return false
		}
	}
	return true
}

func nameByte(c byte) bool {
	return asciiAlnum(c) || c == '.' || c == '_' || c == '-'
}

func asciiAlnum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func asciiFold(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

func accessStrings(in []Access) []string {
	out := make([]string, len(in))
	for i, a := range in {
		out[i] = string(a)
	}
	return out
}

func cloneMembers(in []RoomMember) []RoomMember {
	if len(in) == 0 {
		return nil
	}
	out := make([]RoomMember, len(in))
	for i := range in {
		out[i] = in[i].clone()
	}
	return out
}

func cloneOptionalStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	return cloneStrings(in)
}
