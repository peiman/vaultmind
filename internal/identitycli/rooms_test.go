package identitycli

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

// TestSignRegistryRoomPolicyRoundTrip proves sign-registry accepts rooms, an
// explicit DM policy, and the admin role, and that the canonical bytes match
// the domain signer. Access and approval_required stay in the order given.
func TestSignRegistryRoomPolicyRoundTrip(t *testing.T) {
	rootPub, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey root: %v", err)
	}
	agentPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey agent: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString(agentPub)
	in := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"Keeper","key_epoch":1,` +
		`"pubkey":"` + b64 + `","roles":[{"kind":"admin"}],"slug":"keeper",` +
		`"valid_from":1770000000,"valid_until":1780000000}],` +
		`"dms":"agents_off","epoch":5,` +
		`"rooms":[` +
		`{"name":"product-control","controller":"human:peiman",` +
		`"members":[{"principal":"agent:workhorse","access":["post","read"]}],` +
		`"approval_required":["publish","merge"]},` +
		`{"name":"ops","controller":"agent:mira"}],` +
		`"valid_from":1770000000,"valid_until":1780000000}`

	fake := &fakeSignerClient{priv: rootPriv}
	var out bytes.Buffer
	if err := SignRegistry(&out, fake, []byte(in)); err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}
	env, err := registry.ParseDistribution(out.Bytes())
	if err != nil {
		t.Fatalf("ParseDistribution: %v", err)
	}
	loaded, _, err := registry.VerifyAndLoad(rootPub, env, 4, time.Unix(1_770_000_500, 0), time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad: %v", err)
	}
	if loaded.DMs.Absent() || loaded.DMs.Effective() != registry.DMPolicyAgentsOff {
		t.Fatalf("dms = absent:%v effective:%q", loaded.DMs.Absent(), loaded.DMs.Effective())
	}
	if len(loaded.Rooms) != 2 || loaded.Rooms[0].Name() != "product-control" || loaded.Rooms[1].Name() != "ops" {
		t.Fatalf("rooms = %#v", loaded.Rooms)
	}
	access := loaded.Rooms[0].Members()[0].Access()
	if len(access) != 2 || access[0] != registry.AccessPost || access[1] != registry.AccessRead {
		t.Fatalf("access = %#v", access)
	}
	if strings.Join(loaded.Rooms[0].ApprovalRequired(), ",") != "publish,merge" {
		t.Fatalf("approval = %#v", loaded.Rooms[0].ApprovalRequired())
	}
	if len(loaded.Rooms[1].Members()) != 0 || len(loaded.Rooms[1].ApprovalRequired()) != 0 {
		t.Fatalf("controller-only room gained lists")
	}
	if len(loaded.Agents) != 1 || loaded.Agents[0].Roles[0].Kind() != registry.RoleKindAdmin {
		t.Fatalf("admin role missing: %#v", loaded.Agents)
	}
	if strings.Contains(string(env.Registry), `"members":[]`) || strings.Contains(string(env.Registry), `"approval_required":[]`) {
		t.Fatalf("empty room lists were materialized: %s", env.Registry)
	}

	pk, err := registry.NewPublicKey(agentPub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	member, err := registry.NewRoomMember("agent:workhorse", []string{"post", "read"})
	if err != nil {
		t.Fatalf("NewRoomMember: %v", err)
	}
	first, err := registry.NewRoom("product-control", "human:peiman", []registry.RoomMember{member}, []string{"publish", "merge"})
	if err != nil {
		t.Fatalf("NewRoom: %v", err)
	}
	second, err := registry.NewRoom("ops", "agent:mira", nil, nil)
	if err != nil {
		t.Fatalf("NewRoom ops: %v", err)
	}
	off, err := registry.NewDMPolicy(registry.DMPolicyAgentsOff)
	if err != nil {
		t.Fatalf("NewDMPolicy: %v", err)
	}
	dom, err := registry.SignRegistryWithSigner(&keyBackedSigner{priv: rootPriv}, registry.Registry{
		Epoch: 5, ValidFrom: 1770000000, ValidUntil: 1780000000,
		Agents: []registry.AgentBinding{{
			Slug: "keeper", DisplayName: "Keeper", PubKey: pk, KeyEpoch: 1,
			ValidFrom: 1770000000, ValidUntil: 1780000000,
			AuthorizedOriginDaemons: []string{"d"},
			Roles:                   []registry.Role{registry.NewAdminRole()},
		}},
		Rooms: []registry.Room{first, second},
		DMs:   off,
	})
	if err != nil {
		t.Fatalf("domain sign: %v", err)
	}
	if !bytes.Equal(env.Registry, dom.Registry) {
		t.Fatalf("CLI canonical bytes differ from domain:\n cli=%s\n dom=%s", env.Registry, dom.Registry)
	}
}

func TestSignRegistryRejectsRoomAndDMRules(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString(pub)
	base := testRegistryJSON(b64)
	longName := "a" + strings.Repeat("b", 128)
	cases := []struct {
		name string
		in   string
		err  string
	}{
		{name: "bad_room_name_chars", in: withPolicy(base, `[{"name":"bad name","controller":"human:peiman"}]`, ""), err: registry.ErrMalformedRoom},
		{name: "room_name_too_long", in: withPolicy(base, `[{"name":"`+longName+`","controller":"human:peiman"}]`, ""), err: registry.ErrMalformedRoom},
		{name: "room_name_control_suffix", in: withPolicy(base, `[{"name":"room.control","controller":"human:peiman"}]`, ""), err: registry.ErrMalformedRoom},
		{name: "duplicate_room_name", in: withPolicy(base, `[{"name":"product-control","controller":"human:peiman"},{"name":"product-control","controller":"agent:mira"}]`, ""), err: registry.ErrMalformedRoom},
		{name: "case_fold_duplicate_room_name", in: withPolicy(base, `[{"name":"Product-Control","controller":"human:peiman"},{"name":"product-control","controller":"agent:mira"}]`, ""), err: registry.ErrMalformedRoom},
		{name: "controller_kind_daemon", in: withPolicy(base, `[{"name":"ops","controller":"daemon:eu-1"}]`, ""), err: registry.ErrMalformedRoom},
		{name: "controller_missing", in: withPolicy(base, `[{"name":"ops"}]`, ""), err: registry.ErrMalformedRoom},
		{name: "member_equals_controller", in: withPolicy(base, `[{"name":"ops","controller":"human:peiman","members":[{"principal":"human:peiman","access":["read"]}]}]`, ""), err: registry.ErrMalformedRoom},
		{name: "duplicate_member", in: withPolicy(base, `[{"name":"ops","controller":"human:peiman","members":[{"principal":"agent:workhorse","access":["read"]},{"principal":"agent:workhorse","access":["post"]}]}]`, ""), err: registry.ErrMalformedRoom},
		{name: "empty_access", in: withPolicy(base, `[{"name":"ops","controller":"human:peiman","members":[{"principal":"agent:workhorse","access":[]}]}]`, ""), err: registry.ErrMalformedRoom},
		{name: "unknown_access", in: withPolicy(base, `[{"name":"ops","controller":"human:peiman","members":[{"principal":"agent:workhorse","access":["write"]}]}]`, ""), err: registry.ErrMalformedRoom},
		{name: "repeated_access", in: withPolicy(base, `[{"name":"ops","controller":"human:peiman","members":[{"principal":"agent:workhorse","access":["read","read"]}]}]`, ""), err: registry.ErrMalformedRoom},
		{name: "bad_approval_required_name", in: withPolicy(base, `[{"name":"ops","controller":"human:peiman","approval_required":["Merge"]}]`, ""), err: registry.ErrMalformedRoom},
		{name: "repeated_approval_required", in: withPolicy(base, `[{"name":"ops","controller":"human:peiman","approval_required":["merge","merge"]}]`, ""), err: registry.ErrMalformedRoom},
		{name: "unknown_dms_value", in: withPolicy(base, "", `"deny"`), err: registry.ErrMalformedDMPolicy},
		{name: "unknown_room_field", in: withPolicy(base, `[{"name":"ops","controller":"human:peiman","extra":true}]`, ""), err: registry.ErrMalformedRoom},
		{name: "admin_extra_field", in: strings.Replace(base, `"slug":"mira"`, `"roles":[{"kind":"admin","scope":"all"}],"slug":"mira"`, 1), err: registry.ErrMalformedRole},
		{name: "rooms_null", in: withPolicy(base, `null`, ""), err: registry.ErrMalformedRoom},
		{name: "dms_null", in: withPolicy(base, "", `null`), err: registry.ErrMalformedDMPolicy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeSignerClient{}
			var out bytes.Buffer
			err := SignRegistry(&out, fake, []byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("got %v, want substring %q", err, tc.err)
			}
			if fake.gotCanonical != nil {
				t.Fatal("signer was called for a rejected registry")
			}
			if out.Len() != 0 {
				t.Fatalf("reject must print nothing, got %q", out.String())
			}
		})
	}
}

func TestSignRegistryEmptyRoomsOmitsTheKey(t *testing.T) {
	_, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey agent: %v", err)
	}
	in := withPolicy(testRegistryJSON(base64.StdEncoding.EncodeToString(pub)), `[]`, "")
	fake := &fakeSignerClient{priv: rootPriv}
	var out bytes.Buffer
	if err := SignRegistry(&out, fake, []byte(in)); err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}
	env, err := registry.ParseDistribution(out.Bytes())
	if err != nil {
		t.Fatalf("ParseDistribution: %v", err)
	}
	if strings.Contains(string(env.Registry), `"rooms"`) || strings.Contains(string(env.Registry), `"dms"`) {
		t.Fatalf("empty rooms or absent dms were emitted: %s", env.Registry)
	}
}

func TestSignRegistryRejectsDuplicateRoomNameKey(t *testing.T) {
	_, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey agent: %v", err)
	}
	in := withPolicy(testRegistryJSON(base64.StdEncoding.EncodeToString(pub)),
		`[{"name":"ops","name":"evil","controller":"human:peiman"}]`, "")
	fake := &fakeSignerClient{priv: rootPriv}
	var out bytes.Buffer
	err = SignRegistry(&out, fake, []byte(in))
	if err == nil || !strings.Contains(err.Error(), identity.ErrSchemaDuplicateKey) {
		t.Fatalf("got %v, want substring %q", err, identity.ErrSchemaDuplicateKey)
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a duplicate key")
	}
}

func TestEnrollAddCarriesRoomsAndDMs(t *testing.T) {
	rootPub, rootPriv := fixedAgentKey(t, 0x31)
	existingPub, _ := fixedAgentKey(t, 0x32)
	agentPub, agentPriv := fixedAgentKey(t, 0x33)
	net := networkOf(rootPub)
	unsigned := `{"agents":[{"authorized_origin_daemons":["daemon-eu-1"],"display_name":"Keeper","key_epoch":1,` +
		`"pubkey":"` + base64.StdEncoding.EncodeToString(existingPub) + `","roles":[{"kind":"admin"}],"slug":"keeper",` +
		`"valid_from":1770000000,"valid_until":1780000000}],` +
		`"dms":"agents_off","epoch":5,` +
		`"rooms":[{"approval_required":["publish","merge"],"controller":"human:peiman",` +
		`"members":[{"access":["post","read"],"principal":"agent:workhorse"}],"name":"product-control"},` +
		`{"controller":"agent:mira","name":"ops"}],` +
		`"valid_from":1770000000,"valid_until":1780000000}`
	signed := signedEnvelopeFrom(t, rootPriv, unsigned)

	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)
	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64:   base64.StdEncoding.EncodeToString(rootPub),
		RegistryInput:   signed,
		ValiditySeconds: testValiditySeconds,
		Now:             fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err != nil {
		t.Fatalf("EnrollAdd: %v", err)
	}
	emitted := decodeEmitted(t, out.Bytes())
	if emitted["dms"] != "agents_off" {
		t.Fatalf("dms = %v, want agents_off", emitted["dms"])
	}
	rooms, _ := emitted["rooms"].([]any)
	if len(rooms) != 2 {
		t.Fatalf("rooms len = %d, want 2 (%s)", len(rooms), out.String())
	}
	first, _ := rooms[0].(map[string]any)
	if first["name"] != "product-control" {
		t.Fatalf("first room = %v", first)
	}
	members, _ := first["members"].([]any)
	if len(members) != 1 {
		t.Fatalf("members dropped: %v", first)
	}
	agents, _ := emitted["agents"].([]any)
	if len(agents) != 2 {
		t.Fatalf("agents = %d, want 2", len(agents))
	}
	keeper, _ := agents[0].(map[string]any)
	roles, _ := keeper["roles"].([]any)
	if len(roles) != 1 {
		t.Fatalf("admin role stripped: %v", keeper)
	}

	// A registry with neither rooms nor dms must not grow those keys.
	plain := signedEnvelopeFrom(t, rootPriv, unsignedRegistryJSON("keeper", base64.StdEncoding.EncodeToString(existingPub), 1, 5))
	req2 := signedRequestFor(t, agentPub, agentPriv, "ada", net, 1_700_000_000)
	var out2, errOut2 bytes.Buffer
	cfg.RegistryInput = plain
	if err := EnrollAdd(&out2, &errOut2, bytes.NewReader(req2), cfg); err != nil {
		t.Fatalf("EnrollAdd plain: %v", err)
	}
	if strings.Contains(out2.String(), `"rooms"`) || strings.Contains(out2.String(), `"dms"`) {
		t.Fatalf("enroll-add materialized policy: %s", out2.String())
	}
}

// withPolicy inserts rooms and dms before epoch. The base document already has
// a comma in front of "epoch", so the inserted text does not start with one.
func withPolicy(base, rooms, dms string) string {
	var parts []string
	if rooms != "" {
		parts = append(parts, `"rooms":`+rooms)
	}
	if dms != "" {
		parts = append(parts, `"dms":`+dms)
	}
	if len(parts) == 0 {
		return base
	}
	insert := strings.Join(parts, ",") + ","
	return strings.Replace(base, `"epoch":5`, insert+`"epoch":5`, 1)
}
