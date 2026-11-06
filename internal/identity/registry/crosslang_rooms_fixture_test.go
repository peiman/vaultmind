package registry

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roomsFixturePath is the cross-language acceptance fixture for room policy,
// DM policy, and the admin role. It is a separate file from the P0a fixtures:
// a verifier that predates these fields rejects a registry that carries them.
// Regenerate with:
//
//	go test ./internal/identity/registry/ -run TestGenerate_CrossLanguageRoomsFixture -update
const roomsFixturePath = "testdata/cross_language_rooms_vectors.json"

// roomsFixture is one valid root-signed envelope plus one root-signed reject
// per room, DM, and admin rule. Every reject is signed by the same root, so
// the failure is the schema.
type roomsFixture struct {
	RootPubKey      string            `json:"root_pubkey"`
	Now             int64             `json:"now"`
	MaxStalenessSec int64             `json:"max_staleness_secs"`
	Valid           roomsValidCase    `json:"valid"`
	Reject          []roomsRejectCase `json:"reject"`
}

type roomsValidCase struct {
	Envelope       json.RawMessage    `json:"envelope"`
	PersistedEpoch int                `json:"persisted_highest_epoch"`
	DMs            string             `json:"dms"`
	Rooms          []expectedRoom     `json:"rooms"`
	Bindings       []expectedRoomBind `json:"bindings"`
}

type expectedRoom struct {
	Name             string           `json:"name"`
	Controller       string           `json:"controller"`
	Members          []expectedMember `json:"members,omitempty"`
	ApprovalRequired []string         `json:"approval_required,omitempty"`
}

type expectedMember struct {
	Principal string   `json:"principal"`
	Access    []string `json:"access"`
}

type expectedRoomBind struct {
	Slug  string         `json:"slug"`
	Roles []expectedRole `json:"roles"`
}

type roomsRejectCase struct {
	Name     string          `json:"name"`
	Reason   string          `json:"reason"`
	Expect   string          `json:"expect"`
	Envelope json.RawMessage `json:"envelope"`
}

func buildRoomsFixture(t *testing.T) roomsFixture {
	t.Helper()
	rootPub, rootPriv := fixedEd25519(t, 0xD1)
	const (
		now       int64 = 2_000_000
		maxStale  int64 = 86_400
		regFrom   int64 = 1_990_000
		regUntil  int64 = 3_000_000
		epoch           = 10
		persisted       = 9
	)
	member, err := NewRoomMember("agent:workhorse", []string{"post", "read"})
	require.NoError(t, err)
	withMembers, err := NewRoom("product-control", "human:peiman", []RoomMember{member}, []string{"publish", "merge"})
	require.NoError(t, err)
	controllerOnly, err := NewRoom("ops", "agent:mira", nil, nil)
	require.NoError(t, err)
	off, err := NewDMPolicy(DMPolicyAgentsOff)
	require.NoError(t, err)
	keeper := rolesFixtureBinding(t, 0xD2, "keeper", "Keeper", []Role{NewAdminRole()})
	reg := Registry{
		Epoch: epoch, ValidFrom: regFrom, ValidUntil: regUntil,
		Agents: []AgentBinding{keeper},
		Rooms:  []Room{withMembers, controllerOnly},
		DMs:    off,
	}
	validEnv := genEnvelope(t, rootPriv, reg)

	base := Registry{
		Epoch: epoch, ValidFrom: regFrom, ValidUntil: regUntil,
		Agents: []AgentBinding{rolesFixtureBinding(t, 0xD2, "keeper", "Keeper", nil)},
	}
	longName := "a" + strings.Repeat("b", 128)
	var many strings.Builder
	many.WriteByte('[')
	for i := 0; i < 257; i++ {
		if i > 0 {
			many.WriteByte(',')
		}
		many.WriteString(`{"name":"r`)
		many.WriteString(itoa(i))
		many.WriteString(`","controller":"human:peiman"}`)
	}
	many.WriteByte(']')
	rejects := []struct {
		name, reason, rooms, dms, roles string
	}{
		{"bad_room_name_chars", "room name has a byte outside the routing-name set", `[{"name":"bad name","controller":"human:peiman"}]`, "", ""},
		{"room_name_too_long", "room name is longer than 128 bytes", `[{"name":"` + longName + `","controller":"human:peiman"}]`, "", ""},
		{"room_name_control_suffix", "room name ends with .control", `[{"name":"room.control","controller":"human:peiman"}]`, "", ""},
		{"room_name_control_suffix_case_variant", "room name ends with .control after ASCII case-folding", `[{"name":"Ops.Control","controller":"human:peiman"}]`, "", ""},
		{"duplicate_room_name", "two rooms share a name byte for byte", `[{"name":"product-control","controller":"human:peiman"},{"name":"product-control","controller":"agent:mira"}]`, "", ""},
		{"case_fold_duplicate_room_name", "two room names are equal under ASCII case-folding", `[{"name":"Product-Control","controller":"human:peiman"},{"name":"product-control","controller":"agent:mira"}]`, "", ""},
		{"controller_kind_daemon", "controller kind is not agent or human", `[{"name":"ops","controller":"daemon:eu-1"}]`, "", ""},
		{"controller_missing", "room has no controller", `[{"name":"ops"}]`, "", ""},
		{"member_equals_controller", "a member principal equals the room controller", `[{"name":"ops","controller":"human:peiman","members":[{"principal":"human:peiman","access":["read"]}]}]`, "", ""},
		{"duplicate_member", "a room lists the same principal twice", `[{"name":"ops","controller":"human:peiman","members":[{"principal":"agent:workhorse","access":["read"]},{"principal":"agent:workhorse","access":["post"]}]}]`, "", ""},
		{"empty_access", "a member access list is empty", `[{"name":"ops","controller":"human:peiman","members":[{"principal":"agent:workhorse","access":[]}]}]`, "", ""},
		{"unknown_access", "a member access value is not read or post", `[{"name":"ops","controller":"human:peiman","members":[{"principal":"agent:workhorse","access":["write"]}]}]`, "", ""},
		{"repeated_access", "a member access list repeats a value", `[{"name":"ops","controller":"human:peiman","members":[{"principal":"agent:workhorse","access":["read","read"]}]}]`, "", ""},
		{"bad_approval_required_name", "an approval_required name does not match the action pattern", `[{"name":"ops","controller":"human:peiman","approval_required":["Merge"]}]`, "", ""},
		{"repeated_approval_required", "approval_required repeats a name", `[{"name":"ops","controller":"human:peiman","approval_required":["merge","merge"]}]`, "", ""},
		{"unknown_dms_value", "dms is not allow or agents_off", "", `"deny"`, ""},
		{"unknown_room_field", "a room object carries an unknown field", `[{"name":"ops","controller":"human:peiman","extra":true}]`, "", ""},
		{"admin_extra_field", "an admin role carries a field other than kind", "", "", `[{"kind":"admin","scope":"all"}]`},
		{"too_many_rooms", "rooms has more than 256 entries", many.String(), "", ""},
	}
	cases := make([]roomsRejectCase, 0, len(rejects))
	for _, rc := range rejects {
		cases = append(cases, roomsRejectCase{
			Name: rc.name, Reason: rc.reason, Expect: "reject",
			Envelope: signPatchedPolicy(t, rootPriv, base, rc.rooms, rc.dms, rc.roles),
		})
	}
	return roomsFixture{
		RootPubKey:      b64(rootPub),
		Now:             now,
		MaxStalenessSec: maxStale,
		Valid: roomsValidCase{
			Envelope:       validEnv,
			PersistedEpoch: persisted,
			DMs:            DMPolicyAgentsOff,
			Rooms: []expectedRoom{
				{
					Name: "product-control", Controller: "human:peiman",
					Members:          []expectedMember{{Principal: "agent:workhorse", Access: []string{"post", "read"}}},
					ApprovalRequired: []string{"publish", "merge"},
				},
				{Name: "ops", Controller: "agent:mira"},
			},
			Bindings: []expectedRoomBind{
				{Slug: "keeper", Roles: []expectedRole{{Kind: "admin"}}},
			},
		},
		Reject: cases,
	}
}

// signPatchedPolicy root-signs reg after replacing rooms, dms, and the first
// binding's roles. Empty strings leave the key absent. Numbers stay in their
// original JSON text. The returned value is distribution-envelope JSON.
func signPatchedPolicy(t *testing.T, rootPriv ed25519.PrivateKey, reg Registry, rooms, dms, roles string) json.RawMessage {
	t.Helper()
	wire, err := toWire(reg)
	require.NoError(t, err)
	raw, err := json.Marshal(wire)
	require.NoError(t, err)
	var obj map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &obj))
	if rooms != "" {
		obj["rooms"] = json.RawMessage(rooms)
	}
	if dms != "" {
		obj["dms"] = json.RawMessage(dms)
	}
	if roles != "" {
		var agents []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(obj["agents"], &agents))
		require.NotEmpty(t, agents)
		agents[0]["roles"] = json.RawMessage(roles)
		agentsRaw, err := json.Marshal(agents)
		require.NoError(t, err)
		obj["agents"] = agentsRaw
	}
	patched, err := json.Marshal(obj)
	require.NoError(t, err)
	canonical, err := identity.Canonicalize(patched)
	require.NoError(t, err)
	sig, err := identity.SignCanonical(rootPriv, canonical)
	require.NoError(t, err)
	out, err := MarshalDistribution(SignedRegistry{Registry: canonical.Bytes(), RootSig: sig, RootKeyEpoch: 0})
	require.NoError(t, err)
	return out
}

func marshalRoomsFixture(t *testing.T) []byte {
	t.Helper()
	out, err := json.MarshalIndent(buildRoomsFixture(t), "", "  ")
	require.NoError(t, err)
	return append(out, '\n')
}

func TestGenerate_CrossLanguageRoomsFixture(t *testing.T) {
	if !*updateFixture {
		t.Skip("run with -update to regenerate the rooms cross-language fixture")
	}
	out := marshalRoomsFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(roomsFixturePath), 0o755))
	require.NoError(t, os.WriteFile(roomsFixturePath, out, 0o644))
	t.Logf("wrote %s (%d bytes)", roomsFixturePath, len(out))
}

func TestCrossLanguageRoomsFixture_MatchesGenerator(t *testing.T) {
	if *updateFixture {
		t.Skip("skipping match check during -update regeneration")
	}
	committed, err := os.ReadFile(roomsFixturePath)
	require.NoError(t, err, "committed rooms fixture must exist; run with -update to generate it")
	assert.Equal(t, string(committed), string(marshalRoomsFixture(t)),
		"committed rooms fixture diverges from the generator — regenerate with: "+
			"go test ./internal/identity/registry/ -run TestGenerate_CrossLanguageRoomsFixture -update")
}

func TestCrossLanguageRoomsFixture_ValidRoundTrips(t *testing.T) {
	fx := loadRoomsFixture(t)
	rootPub := mustB64Pub(t, fx.RootPubKey)
	parsed, err := ParseDistribution(fx.Valid.Envelope)
	require.NoError(t, err)
	reg, _, err := VerifyAndLoad(rootPub, parsed, fx.Valid.PersistedEpoch,
		time.Unix(fx.Now, 0), time.Duration(fx.MaxStalenessSec)*time.Second)
	require.NoError(t, err)
	assert.False(t, reg.DMs.Absent())
	assert.Equal(t, fx.Valid.DMs, reg.DMs.Effective())
	require.Len(t, reg.Rooms, len(fx.Valid.Rooms))
	for i, want := range fx.Valid.Rooms {
		got := reg.Rooms[i]
		assert.Equal(t, want.Name, got.Name())
		assert.Equal(t, want.Controller, got.Controller())
		assert.Equal(t, want.ApprovalRequired, got.ApprovalRequired())
		require.Len(t, got.Members(), len(want.Members))
		for j, member := range want.Members {
			assert.Equal(t, member.Principal, got.Members()[j].Principal())
			gotAccess := make([]string, len(got.Members()[j].Access()))
			for k, a := range got.Members()[j].Access() {
				gotAccess[k] = string(a)
			}
			assert.Equal(t, member.Access, gotAccess)
		}
	}
	require.Len(t, reg.Agents, len(fx.Valid.Bindings))
	for i, want := range fx.Valid.Bindings {
		got := reg.Agents[i]
		assert.Equal(t, want.Slug, got.Slug)
		require.Len(t, got.Roles, len(want.Roles))
		for j, role := range want.Roles {
			assert.Equal(t, RoleKind(role.Kind), got.Roles[j].Kind())
		}
	}

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(parsed.Registry, &body))
	_, hasDMs := body["dms"]
	assert.True(t, hasDMs)
	var rooms []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body["rooms"], &rooms))
	require.Len(t, rooms, 2)
	_, hasMembers := rooms[1]["members"]
	_, hasApproval := rooms[1]["approval_required"]
	assert.False(t, hasMembers, "a controller-only room omits members")
	assert.False(t, hasApproval, "a controller-only room omits approval_required")
}

func TestCrossLanguageRoomsFixture_RejectsAreSchemaNotSignature(t *testing.T) {
	fx := loadRoomsFixture(t)
	rootPub := mustB64Pub(t, fx.RootPubKey)
	want := []struct {
		name string
		err  string
	}{
		{"bad_room_name_chars", ErrMalformedRoom},
		{"room_name_too_long", ErrMalformedRoom},
		{"room_name_control_suffix", ErrMalformedRoom},
		{"room_name_control_suffix_case_variant", ErrMalformedRoom},
		{"duplicate_room_name", ErrMalformedRoom},
		{"case_fold_duplicate_room_name", ErrMalformedRoom},
		{"controller_kind_daemon", ErrMalformedRoom},
		{"controller_missing", ErrMalformedRoom},
		{"member_equals_controller", ErrMalformedRoom},
		{"duplicate_member", ErrMalformedRoom},
		{"empty_access", ErrMalformedRoom},
		{"unknown_access", ErrMalformedRoom},
		{"repeated_access", ErrMalformedRoom},
		{"bad_approval_required_name", ErrMalformedRoom},
		{"repeated_approval_required", ErrMalformedRoom},
		{"unknown_dms_value", ErrMalformedDMPolicy},
		{"unknown_room_field", ErrMalformedRoom},
		{"admin_extra_field", ErrMalformedRole},
		{"too_many_rooms", ErrMalformedRoom},
	}
	require.Len(t, fx.Reject, len(want))
	for i, rc := range fx.Reject {
		t.Run(rc.Name, func(t *testing.T) {
			assert.Equal(t, want[i].name, rc.Name)
			assert.Equal(t, "reject", rc.Expect)
			parsed, err := ParseDistribution(rc.Envelope)
			require.NoError(t, err, "the envelope itself is well formed")
			ok, err := identity.VerifyCanonical(rootPub, identity.CanonicalBytesFromTrusted(parsed.Registry), parsed.RootSig)
			require.NoError(t, err)
			assert.True(t, ok, "root signature must hold; the reject is the schema")
			_, _, err = VerifyAndLoad(rootPub, parsed, fx.Valid.PersistedEpoch,
				time.Unix(fx.Now, 0), time.Duration(fx.MaxStalenessSec)*time.Second)
			require.ErrorContains(t, err, want[i].err)
		})
	}
}

func loadRoomsFixture(t *testing.T) roomsFixture {
	t.Helper()
	data, err := os.ReadFile(roomsFixturePath)
	require.NoError(t, err, "committed rooms fixture must exist; run with -update to generate it")
	var fx roomsFixture
	require.NoError(t, json.Unmarshal(data, &fx))
	return fx
}
