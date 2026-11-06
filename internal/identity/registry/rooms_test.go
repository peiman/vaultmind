package registry

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// p0aFixtureSHA256 pins the two fixtures this change must not rewrite.
// cross_language_vectors.json is the roles-free contract. cross_language_roles_vectors.json
// is P0a's roles contract. Room policy is a third file.
const (
	p0aVectorsSHA256 = "089548af1bd479931995a5de0f3ba44080d73449c58ec3c168c0c8bde3e7fa0d"
	p0aRolesSHA256   = "d98bd5925ac0c7fef89130999cd63a20b2d42d1d7dbcffb00110b7f0b3e581f8"
)

func TestP0aFixtures_StayByteIdentical(t *testing.T) {
	assertFileSHA256(t, "testdata/cross_language_vectors.json", p0aVectorsSHA256)
	assertFileSHA256(t, "testdata/cross_language_roles_vectors.json", p0aRolesSHA256)
}

func assertFileSHA256(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	assert.Equal(t, want, hex.EncodeToString(sum[:]), path)
}

func TestRoomConstructors_RejectAndDoNotAlias(t *testing.T) {
	member, err := NewRoomMember("agent:workhorse", []string{string(AccessPost), string(AccessRead)})
	require.NoError(t, err)
	room, err := NewRoom("product-control", "human:peiman", []RoomMember{member}, []string{"publish", "merge"})
	require.NoError(t, err)
	assert.Equal(t, "product-control", room.Name())
	assert.Equal(t, "human:peiman", room.Controller())
	assert.Equal(t, []string{"publish", "merge"}, room.ApprovalRequired())
	gotMembers := room.Members()
	require.Len(t, gotMembers, 1)
	assert.Equal(t, "agent:workhorse", gotMembers[0].Principal())
	assert.Equal(t, []Access{AccessPost, AccessRead}, gotMembers[0].Access())

	gotMembers[0] = RoomMember{}
	assert.Equal(t, "agent:workhorse", room.Members()[0].Principal())
	approval := room.ApprovalRequired()
	approval[0] = "nope"
	assert.Equal(t, []string{"publish", "merge"}, room.ApprovalRequired())

	_, err = NewRoom("room.control", "human:peiman", nil, nil)
	require.ErrorContains(t, err, ErrMalformedRoom)
	_, err = NewRoomMember("human:peiman", nil)
	require.ErrorContains(t, err, ErrMalformedRoom)
	_, err = NewDMPolicy("sometimes")
	require.ErrorContains(t, err, ErrMalformedDMPolicy)
	allow, err := NewDMPolicy(DMPolicyAllow)
	require.NoError(t, err)
	assert.False(t, allow.Absent())
	assert.Equal(t, DMPolicyAllow, allow.Effective())
	assert.True(t, DMPolicy{}.Absent())
	assert.Equal(t, DMPolicyAllow, DMPolicy{}.Effective())
}

func TestCanonicalBytes_EmitsRoomPolicyInGivenOrderAndOmitsAbsent(t *testing.T) {
	member, err := NewRoomMember("agent:workhorse", []string{"post", "read"})
	require.NoError(t, err)
	withMembers, err := NewRoom("product-control", "human:peiman", []RoomMember{member}, []string{"publish", "merge"})
	require.NoError(t, err)
	controllerOnly, err := NewRoom("ops", "agent:mira", nil, nil)
	require.NoError(t, err)
	off, err := NewDMPolicy(DMPolicyAgentsOff)
	require.NoError(t, err)

	reg := rolesFreeRegistry(t, []Role{NewAdminRole()})
	reg.Rooms = []Room{withMembers, controllerOnly}
	reg.DMs = off
	cb, err := canonicalBytes(reg)
	require.NoError(t, err)
	body := string(cb.Bytes())
	assert.Contains(t, body, `"dms":"agents_off"`)
	assert.Contains(t, body, `"roles":[{"kind":"admin"}]`)
	assert.Contains(t, body, `"access":["post","read"]`)
	assert.Contains(t, body, `"approval_required":["publish","merge"]`)
	assert.Contains(t, body, `"name":"product-control"`)
	assert.Contains(t, body, `"name":"ops"`)
	assert.NotContains(t, body, `"vouches_for"`)
	assert.NotContains(t, body, `"platforms"`)
	// The controller-only room has no members and no approval list.
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(cb.Bytes(), &decoded))
	rooms, ok := decoded["rooms"].([]any)
	require.True(t, ok)
	require.Len(t, rooms, 2)
	ops, ok := rooms[1].(map[string]any)
	require.True(t, ok)
	_, hasMembers := ops["members"]
	_, hasApproval := ops["approval_required"]
	assert.False(t, hasMembers)
	assert.False(t, hasApproval)

	reversed := reg
	reversed.Rooms = []Room{controllerOnly, withMembers}
	rev, err := canonicalBytes(reversed)
	require.NoError(t, err)
	assert.NotEqual(t, cb.Bytes(), rev.Bytes(), "room array order is signed as given")
}

func TestCanonicalBytes_ExplicitAllowIsPresent(t *testing.T) {
	allow, err := NewDMPolicy(DMPolicyAllow)
	require.NoError(t, err)
	reg := rolesFreeRegistry(t, nil)
	reg.DMs = allow
	cb, err := canonicalBytes(reg)
	require.NoError(t, err)
	assert.Contains(t, string(cb.Bytes()), `"dms":"allow"`)
	assert.NotContains(t, string(cb.Bytes()), `"rooms"`)
}

func TestVerifyAndLoad_RoomPolicyRoundTrip(t *testing.T) {
	rootPub, rootPriv := fixedEd25519(t, 0x11)
	member, err := NewRoomMember("agent:workhorse", []string{"post", "read"})
	require.NoError(t, err)
	first, err := NewRoom("product-control", "human:peiman", []RoomMember{member}, []string{"publish", "merge"})
	require.NoError(t, err)
	second, err := NewRoom("ops", "agent:mira", nil, nil)
	require.NoError(t, err)
	off, err := NewDMPolicy(DMPolicyAgentsOff)
	require.NoError(t, err)

	reg := rolesFreeRegistry(t, []Role{NewAdminRole()})
	reg.Rooms = []Room{first, second}
	reg.DMs = off
	env, err := SignRegistry(rootPriv, reg)
	require.NoError(t, err)
	loaded, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	require.NoError(t, err)
	require.Len(t, loaded.Rooms, 2)
	assert.Equal(t, first.Name(), loaded.Rooms[0].Name())
	assert.Equal(t, []Access{AccessPost, AccessRead}, loaded.Rooms[0].Members()[0].Access())
	assert.Equal(t, []string{"publish", "merge"}, loaded.Rooms[0].ApprovalRequired())
	assert.Empty(t, loaded.Rooms[1].Members())
	assert.Empty(t, loaded.Rooms[1].ApprovalRequired())
	assert.False(t, loaded.DMs.Absent())
	assert.Equal(t, DMPolicyAgentsOff, loaded.DMs.Effective())
	require.Len(t, loaded.Agents[0].Roles, 1)
	assert.Equal(t, RoleKindAdmin, loaded.Agents[0].Roles[0].Kind())
	assert.Nil(t, loaded.Agents[0].Roles[0].VouchesFor())
	assert.Nil(t, loaded.Agents[0].Roles[0].Platforms())
}

func TestVerifyAndLoad_RejectsRoomAndDMRules(t *testing.T) {
	longName := "a" + strings.Repeat("b", 128)
	var many strings.Builder
	many.WriteByte('[')
	for i := 0; i < 257; i++ {
		if i > 0 {
			many.WriteByte(',')
		}
		many.WriteString(`{"name":"room-`)
		many.WriteString(strings.Repeat("x", 0))
		many.WriteString(itoa(i))
		many.WriteString(`","controller":"human:peiman"}`)
	}
	many.WriteByte(']')

	cases := []struct {
		name  string
		rooms string
		dms   string
		roles string
		err   string
	}{
		{name: "bad_room_name_chars", rooms: `[{"name":"bad name","controller":"human:peiman"}]`, err: ErrMalformedRoom},
		{name: "room_name_too_long", rooms: `[{"name":"` + longName + `","controller":"human:peiman"}]`, err: ErrMalformedRoom},
		{name: "room_name_control_suffix", rooms: `[{"name":"room.control","controller":"human:peiman"}]`, err: ErrMalformedRoom},
		{name: "duplicate_room_name", rooms: `[{"name":"product-control","controller":"human:peiman"},{"name":"product-control","controller":"agent:mira"}]`, err: ErrMalformedRoom},
		{name: "case_fold_duplicate_room_name", rooms: `[{"name":"Product-Control","controller":"human:peiman"},{"name":"product-control","controller":"agent:mira"}]`, err: ErrMalformedRoom},
		{name: "controller_kind_daemon", rooms: `[{"name":"ops","controller":"daemon:eu-1"}]`, err: ErrMalformedRoom},
		{name: "controller_missing", rooms: `[{"name":"ops"}]`, err: ErrMalformedRoom},
		{name: "member_equals_controller", rooms: `[{"name":"ops","controller":"human:peiman","members":[{"principal":"human:peiman","access":["read"]}]}]`, err: ErrMalformedRoom},
		{name: "duplicate_member", rooms: `[{"name":"ops","controller":"human:peiman","members":[{"principal":"agent:workhorse","access":["read"]},{"principal":"agent:workhorse","access":["post"]}]}]`, err: ErrMalformedRoom},
		{name: "empty_access", rooms: `[{"name":"ops","controller":"human:peiman","members":[{"principal":"agent:workhorse","access":[]}]}]`, err: ErrMalformedRoom},
		{name: "unknown_access", rooms: `[{"name":"ops","controller":"human:peiman","members":[{"principal":"agent:workhorse","access":["write"]}]}]`, err: ErrMalformedRoom},
		{name: "repeated_access", rooms: `[{"name":"ops","controller":"human:peiman","members":[{"principal":"agent:workhorse","access":["read","read"]}]}]`, err: ErrMalformedRoom},
		{name: "bad_approval_required_name", rooms: `[{"name":"ops","controller":"human:peiman","approval_required":["Merge"]}]`, err: ErrMalformedRoom},
		{name: "repeated_approval_required", rooms: `[{"name":"ops","controller":"human:peiman","approval_required":["merge","merge"]}]`, err: ErrMalformedRoom},
		{name: "unknown_dms_value", dms: `"deny"`, err: ErrMalformedDMPolicy},
		{name: "unknown_room_field", rooms: `[{"name":"ops","controller":"human:peiman","extra":true}]`, err: ErrMalformedRoom},
		{name: "admin_extra_field", roles: `[{"kind":"admin","scope":"all"}]`, err: ErrMalformedRole},
		{name: "too_many_rooms", rooms: many.String(), err: ErrMalformedRoom},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rootPub, env := signedPolicyBody(t, tc.rooms, tc.dms, tc.roles)
			ok, err := identity.VerifyCanonical(rootPub, identity.CanonicalBytesFromTrusted(env.Registry), env.RootSig)
			require.NoError(t, err)
			assert.True(t, ok, "root signature must hold; the reject is the schema")
			_, _, err = VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
			require.ErrorContains(t, err, tc.err)
		})
	}
}

func TestParseRooms_BoundsAndShapes(t *testing.T) {
	_, err := ParseRooms([]byte(`null`))
	require.ErrorContains(t, err, ErrMalformedRoom)
	_, err = ParseRooms([]byte(`{"name":"ops"}`))
	require.ErrorContains(t, err, ErrMalformedRoom)
	rooms, err := ParseRooms(nil)
	require.NoError(t, err)
	assert.Empty(t, rooms)
	rooms, err = ParseRooms([]byte(`[]`))
	require.NoError(t, err)
	assert.Empty(t, rooms)

	_, err = ParseDMPolicy([]byte(`null`))
	require.ErrorContains(t, err, ErrMalformedDMPolicy)
	_, err = ParseDMPolicy([]byte(`1`))
	require.ErrorContains(t, err, ErrMalformedDMPolicy)
	absent, err := ParseDMPolicy(nil)
	require.NoError(t, err)
	assert.True(t, absent.Absent())

	name := "a" + strings.Repeat("b", 127)
	raw := `[{"name":"` + name + `","controller":"human:peiman"}]`
	rooms, err = ParseRooms([]byte(raw))
	require.NoError(t, err)
	require.Len(t, rooms, 1)
	assert.Equal(t, name, rooms[0].Name())

	_, err = ParseRooms([]byte(`[{"name":"Room.Control","controller":"human:peiman"}]`))
	require.NoError(t, err)

	_, err = ParseRooms([]byte(`[{"name":"ops","controller":"human:peiman","members":[{"principal":"human:Peiman","access":["read"]}]}]`))
	require.NoError(t, err)

	var b strings.Builder
	b.WriteString(`[{"name":"ops","controller":"human:peiman","members":[`)
	for i := 0; i < 256; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"principal":"agent:m`)
		b.WriteString(itoa(i))
		b.WriteString(`","access":["read"]}`)
	}
	b.WriteString(`]}]`)
	rooms, err = ParseRooms([]byte(b.String()))
	require.NoError(t, err)
	require.Len(t, rooms[0].Members(), 256)

	b.Reset()
	b.WriteString(`[{"name":"ops","controller":"human:peiman","members":[`)
	for i := 0; i < 257; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"principal":"agent:m`)
		b.WriteString(itoa(i))
		b.WriteString(`","access":["read"]}`)
	}
	b.WriteString(`]}]`)
	_, err = ParseRooms([]byte(b.String()))
	require.ErrorContains(t, err, ErrMalformedRoom)

	var approval strings.Builder
	approval.WriteString(`[{"name":"ops","controller":"human:peiman","approval_required":[`)
	for i := 0; i < 32; i++ {
		if i > 0 {
			approval.WriteByte(',')
		}
		approval.WriteString(`"a`)
		approval.WriteString(itoa(i))
		approval.WriteString(`"`)
	}
	approval.WriteString(`]}]`)
	_, err = ParseRooms([]byte(approval.String()))
	require.NoError(t, err)
	approval.Reset()
	approval.WriteString(`[{"name":"ops","controller":"human:peiman","approval_required":[`)
	for i := 0; i < 33; i++ {
		if i > 0 {
			approval.WriteByte(',')
		}
		approval.WriteString(`"a`)
		approval.WriteString(itoa(i))
		approval.WriteString(`"`)
	}
	approval.WriteString(`]}]`)
	_, err = ParseRooms([]byte(approval.String()))
	require.ErrorContains(t, err, ErrMalformedRoom)

	_, err = ParseRooms([]byte(`[{"name":"ops","controller":"human:peiman","members":[{"principal":"agent:workhorse","access":["read"],"note":1}]}]`))
	require.ErrorContains(t, err, ErrMalformedRoom)
	_, err = NewRoom("dm:a/b.C_1-z", "agent:Mira.v2_1", nil, nil)
	require.NoError(t, err)
}

func TestSignRegistry_RejectsHandBuiltRoom(t *testing.T) {
	_, rootPriv := fixedEd25519(t, 0x11)
	reg := rolesFreeRegistry(t, nil)
	reg.Rooms = []Room{{name: "not-checked"}}
	_, err := SignRegistry(rootPriv, reg)
	require.ErrorContains(t, err, ErrMalformedRoom)
}

func signedPolicyBody(t *testing.T, rooms, dms, roles string) (ed25519.PublicKey, SignedRegistry) {
	t.Helper()
	rootPub, rootPriv := fixedEd25519(t, 0x11)
	pub, _ := fixedEd25519(t, 0x22)
	rolesField := ""
	if roles != "" {
		rolesField = `,"roles":` + roles
	}
	extra := ""
	if rooms != "" {
		extra += `,"rooms":` + rooms
	}
	if dms != "" {
		extra += `,"dms":` + dms
	}
	body := `{"agents":[{"authorized_origin_daemons":["daemon-eu-1"],"display_name":"Mira",` +
		`"key_epoch":1,"pubkey":"` + base64.StdEncoding.EncodeToString(pub) + `"` + rolesField +
		`,"slug":"mira","valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":5` + extra + `,"valid_from":1770000000,"valid_until":1780000000}`
	canonical, err := identity.Canonicalize([]byte(body))
	require.NoError(t, err)
	sig, err := identity.SignCanonical(rootPriv, canonical)
	require.NoError(t, err)
	return rootPub, SignedRegistry{Registry: canonical.Bytes(), RootSig: sig}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [8]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}
