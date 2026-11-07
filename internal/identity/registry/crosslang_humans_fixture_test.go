package registry

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// humansFixturePath is the cross-language acceptance fixture for people in the
// trust registry. Regenerate with:
//
//	go test ./internal/identity/registry/ -run TestGenerate_CrossLanguageHumansFixture -update
const humansFixturePath = "testdata/cross_language_humans_vectors.json"

// humansFixture is one valid root-signed registry (two agents, one of them a
// bridge, plus peiman with god and isolated custody and siavoush with no roles),
// the schema rejects, and the human-signed chat samples. Every reject is signed
// by the same root, so the signature holds and the schema does not.
type humansFixture struct {
	RootPubKey      string          `json:"root_pubkey"`
	Now             int64           `json:"now"`
	MaxStalenessSec int64           `json:"max_staleness_secs"`
	Valid           humansValidCase `json:"valid"`
	Reject          []humansReject  `json:"reject"`
	Envelopes       humansEnvelopes `json:"envelopes"`
}

type humansValidCase struct {
	Envelope          json.RawMessage         `json:"envelope"`
	PersistedEpoch    int                     `json:"persisted_highest_epoch"`
	CanonicalRegistry string                  `json:"canonical_registry"`
	CanonicalSHA256   string                  `json:"canonical_sha256"`
	Bindings          []humansExpectedBinding `json:"bindings"`
}

type humansExpectedBinding struct {
	Slug    string         `json:"slug"`
	Class   string         `json:"class"`
	Custody string         `json:"custody,omitempty"`
	Roles   []expectedRole `json:"roles"`
}

type humansReject struct {
	Name     string          `json:"name"`
	Reason   string          `json:"reason"`
	Expect   string          `json:"expect"`
	Envelope json.RawMessage `json:"envelope"`
}

type humansEnvelopes struct {
	Valid  humansChat   `json:"valid"`
	Reject []humansChat `json:"reject"`
}

type humansChat struct {
	Name       string `json:"name"`
	Reason     string `json:"reason,omitempty"`
	Expect     string `json:"expect"`
	AlgVersion int64  `json:"alg_version"`
	Body       string `json:"body"`
	FromAgent  string `json:"from_agent"`
	FromClass  string `json:"from_class"`
	KeyEpoch   int64  `json:"key_epoch"`
	Kind       string `json:"kind"`
	Nonce      string `json:"nonce"`
	Room       string `json:"room"`
	Seq        int64  `json:"seq"`
	TS         int64  `json:"ts"`
	Canonical  string `json:"canonical"`
	Signature  string `json:"signature"`
}

func buildHumansFixture(t *testing.T) humansFixture {
	t.Helper()
	rootPub, rootPriv := fixedEd25519(t, 0xD1)
	_, miraPriv := fixedEd25519(t, 0xD2)
	_, peimanPriv := fixedEd25519(t, 0xD4)
	const (
		now       int64 = 2_000_000
		maxStale  int64 = 86_400
		epoch           = 10
		persisted       = 9
	)
	reg := humansFixtureRegistry(t)
	env, err := SignRegistry(rootPriv, reg)
	require.NoError(t, err)
	dist, err := MarshalDistribution(env)
	require.NoError(t, err)
	sum := sha256.Sum256(env.Registry)

	return humansFixture{
		RootPubKey:      b64(rootPub),
		Now:             now,
		MaxStalenessSec: maxStale,
		Valid: humansValidCase{
			Envelope:          dist,
			PersistedEpoch:    persisted,
			CanonicalRegistry: string(env.Registry),
			CanonicalSHA256:   hex.EncodeToString(sum[:]),
			Bindings: []humansExpectedBinding{
				{Slug: "mira", Class: ClassAgent, Roles: []expectedRole{{Kind: "bridge", VouchesFor: []string{"human:peiman"}}}},
				{Slug: "ada", Class: ClassAgent, Roles: []expectedRole{}},
				{Slug: "peiman", Class: ClassHuman, Custody: CustodyIsolated, Roles: []expectedRole{{Kind: "god"}}},
				{Slug: "siavoush", Class: ClassHuman, Roles: []expectedRole{}},
			},
		},
		Reject:    humansFixtureRejects(t, rootPriv, reg),
		Envelopes: humansFixtureChats(t, peimanPriv, miraPriv),
	}
}

func humansFixtureRegistry(t *testing.T) Registry {
	t.Helper()
	return Registry{
		Epoch: 10, ValidFrom: 1_990_000, ValidUntil: 3_000_000,
		Agents: []AgentBinding{
			humansFixtureBinding(t, 0xD2, "mira", "Mira", []Role{NewBridgeRole([]string{"human:peiman"})}, ""),
			humansFixtureBinding(t, 0xD3, "ada", "Ada", nil, ""),
		},
		Humans: []AgentBinding{
			humansFixtureBinding(t, 0xD4, "peiman", "Peiman", []Role{NewGodRole()}, CustodyIsolated),
			humansFixtureBinding(t, 0xD5, "siavoush", "Siavoush", nil, ""),
		},
	}
}

func humansFixtureBinding(t *testing.T, fill byte, slug, name string, roles []Role, custody string) AgentBinding {
	t.Helper()
	pub, _ := fixedEd25519(t, fill)
	pk, err := NewPublicKey(pub)
	require.NoError(t, err)
	return AgentBinding{
		Slug: slug, DisplayName: name, PubKey: pk, KeyEpoch: 1,
		ValidFrom: 1_990_000, ValidUntil: 3_000_000,
		AuthorizedOriginDaemons: []string{"daemon-1"},
		Roles:                   roles,
		Custody:                 custody,
	}
}

func humansFixtureRejects(t *testing.T, rootPriv ed25519.PrivateKey, reg Registry) []humansReject {
	t.Helper()
	type spec struct {
		name, reason string
		patch        func(obj map[string]any)
	}
	specs := []spec{
		{name: "god_on_agent", reason: "god role is allowed only on a human", patch: func(obj map[string]any) {
			agents := obj["agents"].([]any)
			agents[0].(map[string]any)["roles"] = []any{map[string]any{"kind": "god"}}
		}},
		{name: "bridge_on_human", reason: "bridge role is allowed only on an agent", patch: func(obj map[string]any) {
			humansFixtureHuman(obj, 1)["roles"] = []any{map[string]any{"kind": "bridge", "vouches_for": []any{"human:ada"}}}
		}},
		{name: "code_signing_on_human", reason: "code_signing role is allowed only on an agent", patch: func(obj map[string]any) {
			humansFixtureHuman(obj, 1)["roles"] = []any{map[string]any{"kind": "code_signing", "platforms": []any{"darwin-arm64"}}}
		}},
		{name: "two_gods", reason: "at most one binding may hold god", patch: func(obj map[string]any) {
			humansFixtureHuman(obj, 1)["roles"] = []any{map[string]any{"kind": "god"}}
		}},
		{name: "slug_across_classes", reason: "slug is bound as both an agent and a human", patch: func(obj map[string]any) {
			humansFixtureHuman(obj, 1)["slug"] = "mira"
		}},
		{name: "bad_custody", reason: "custody must be shared or isolated", patch: func(obj map[string]any) {
			humansFixtureHuman(obj, 1)["custody"] = "private"
		}},
		{name: "god_extra_field", reason: "god role carries a field other than kind", patch: func(obj map[string]any) {
			humansFixtureHuman(obj, 0)["roles"] = []any{map[string]any{"kind": "god", "extra": true}}
		}},
		{name: "human_missing_pubkey", reason: "human binding has no pubkey", patch: func(obj map[string]any) {
			delete(humansFixtureHuman(obj, 0), "pubkey")
		}},
	}
	out := make([]humansReject, 0, len(specs))
	for _, spec := range specs {
		out = append(out, humansReject{
			Name: spec.name, Reason: spec.reason, Expect: "reject",
			Envelope: humansRejectEnvelope(t, rootPriv, reg, spec.patch),
		})
	}
	return out
}

func humansFixtureHuman(obj map[string]any, index int) map[string]any {
	humans := obj["humans"].([]any)
	return humans[index].(map[string]any)
}

func humansRejectEnvelope(t *testing.T, rootPriv ed25519.PrivateKey, reg Registry, patch func(map[string]any)) json.RawMessage {
	t.Helper()
	signed := signPatchedObject(t, rootPriv, reg, func(t *testing.T, obj map[string]any) { patch(obj) })
	out, err := MarshalDistribution(signed)
	require.NoError(t, err)
	return out
}

func humansFixtureChats(t *testing.T, peimanPriv, miraPriv ed25519.PrivateKey) humansEnvelopes {
	t.Helper()
	valid := humansChatSample(t, "human_peiman_chat", "", "accept", "human:peiman", "human", peimanPriv)
	return humansEnvelopes{
		Valid: valid,
		Reject: []humansChat{
			humansChatSample(t, "human_claims_from_class_agent", "human: prefix requires from_class human", "reject", "human:peiman", "agent", peimanPriv),
			humansChatSample(t, "agent_claims_from_class_human", "from_class human requires a human: principal", "reject", "mira", "human", miraPriv),
			humansChatSample(t, "unknown_human", "no live human binding for the slug", "reject", "human:nobody", "human", peimanPriv),
		},
	}
}

func humansChatSample(t *testing.T, name, reason, expect, fromAgent, fromClass string, priv ed25519.PrivateKey) humansChat {
	t.Helper()
	const (
		body  = "hello from peiman"
		nonce = "n-human"
		room  = "mesh"
		seq   = int64(1)
		ts    = int64(2_000_000)
		epoch = int64(1)
	)
	raw, err := json.Marshal(map[string]any{
		"alg_version": int64(1),
		"body":        body,
		"from_agent":  fromAgent,
		"from_class":  fromClass,
		"key_epoch":   epoch,
		"kind":        "chat",
		"nonce":       nonce,
		"room":        room,
		"seq":         seq,
		"ts":          ts,
	})
	require.NoError(t, err)
	canonical, err := identity.Canonicalize(raw)
	require.NoError(t, err)
	sig, err := identity.SignCanonical(priv, canonical)
	require.NoError(t, err)
	return humansChat{
		Name: name, Reason: reason, Expect: expect,
		AlgVersion: 1, Body: body, FromAgent: fromAgent, FromClass: fromClass,
		KeyEpoch: epoch, Kind: "chat", Nonce: nonce, Room: room, Seq: seq, TS: ts,
		Canonical: string(canonical.Bytes()), Signature: b64(sig),
	}
}

func marshalHumansFixture(t *testing.T) []byte {
	t.Helper()
	out, err := json.MarshalIndent(buildHumansFixture(t), "", "  ")
	require.NoError(t, err)
	return append(out, '\n')
}

func TestGenerate_CrossLanguageHumansFixture(t *testing.T) {
	if !*updateFixture {
		t.Skip("run with -update to regenerate the humans cross-language fixture")
	}
	out := marshalHumansFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(humansFixturePath), 0o755))
	require.NoError(t, os.WriteFile(humansFixturePath, out, 0o644))
	t.Logf("wrote %s (%d bytes)", humansFixturePath, len(out))
}

func TestCrossLanguageHumansFixture_MatchesGenerator(t *testing.T) {
	if *updateFixture {
		t.Skip("skipping match check during -update regeneration")
	}
	committed, err := os.ReadFile(humansFixturePath)
	require.NoError(t, err, "committed humans fixture must exist; run with -update to generate it")
	assert.Equal(t, string(committed), string(marshalHumansFixture(t)),
		"committed humans fixture diverges from the generator — regenerate with: "+
			"go test ./internal/identity/registry/ -run TestGenerate_CrossLanguageHumansFixture -update")
}

func TestCrossLanguageHumansFixture_ValidRoundTrip(t *testing.T) {
	fx := loadHumansFixture(t)
	rootPub := mustB64Pub(t, fx.RootPubKey)
	parsed, err := ParseDistribution(fx.Valid.Envelope)
	require.NoError(t, err)
	assert.Equal(t, fx.Valid.CanonicalRegistry, string(parsed.Registry))
	sum := sha256.Sum256(parsed.Registry)
	assert.Equal(t, fx.Valid.CanonicalSHA256, hex.EncodeToString(sum[:]))
	assert.NotContains(t, fx.Valid.CanonicalRegistry, `"class"`)
	assert.Contains(t, fx.Valid.CanonicalRegistry, `"custody":"isolated"`)
	assert.Contains(t, fx.Valid.CanonicalRegistry, `"kind":"god"`)

	reg, _, err := VerifyAndLoad(rootPub, parsed, fx.Valid.PersistedEpoch,
		time.Unix(fx.Now, 0), time.Duration(fx.MaxStalenessSec)*time.Second)
	require.NoError(t, err)
	require.Len(t, append(reg.Agents, reg.Humans...), len(fx.Valid.Bindings))
	bySlug := map[string]AgentBinding{}
	for _, b := range reg.Agents {
		bySlug[b.Slug] = b
	}
	for _, b := range reg.Humans {
		bySlug[b.Slug] = b
	}
	for _, want := range fx.Valid.Bindings {
		got, ok := bySlug[want.Slug]
		require.True(t, ok, "missing %s", want.Slug)
		assert.Equal(t, want.Class, got.EffectiveClass())
		assert.Equal(t, want.Custody, got.Custody)
		require.Len(t, got.Roles, len(want.Roles), "slug %s", want.Slug)
		for j, role := range want.Roles {
			assert.Equal(t, RoleKind(role.Kind), got.Roles[j].Kind())
		}
	}
	god, ok := GodBinding(reg)
	require.True(t, ok)
	assert.Equal(t, "peiman", god.Slug)
	assert.Equal(t, ClassHuman, god.Class)

	sig, err := base64.StdEncoding.DecodeString(fx.Envelopes.Valid.Signature)
	require.NoError(t, err)
	verified, err := VerifyHumanMessage(reg, "peiman", 1,
		identity.CanonicalBytesFromTrusted([]byte(fx.Envelopes.Valid.Canonical)), sig, time.Unix(fx.Now, 0))
	require.NoError(t, err)
	assert.True(t, verified)
}

func TestCrossLanguageHumansFixture_RejectsAreSchemaNotSignature(t *testing.T) {
	fx := loadHumansFixture(t)
	rootPub := mustB64Pub(t, fx.RootPubKey)
	want := []struct {
		name, err, slug string
	}{
		{"god_on_agent", ErrGodOnlyOnHuman, "mira"},
		{"bridge_on_human", ErrAgentRoleOnlyOnAgent, "siavoush"},
		{"code_signing_on_human", ErrAgentRoleOnlyOnAgent, "siavoush"},
		{"two_gods", ErrMultipleGods, "peiman"},
		{"slug_across_classes", ErrSlugAcrossClasses, "mira"},
		{"bad_custody", ErrBadCustody, "siavoush"},
		{"god_extra_field", ErrMalformedRole, "peiman"},
		{"human_missing_pubkey", ErrPubKeyBadLen, "peiman"},
	}
	require.Len(t, fx.Reject, len(want))
	for i, rc := range fx.Reject {
		t.Run(rc.Name, func(t *testing.T) {
			assert.Equal(t, want[i].name, rc.Name)
			assert.Equal(t, "reject", rc.Expect)
			assert.NotEmpty(t, rc.Reason)
			parsed, err := ParseDistribution(rc.Envelope)
			require.NoError(t, err)
			ok, err := identity.VerifyCanonical(rootPub, identity.CanonicalBytesFromTrusted(parsed.Registry), parsed.RootSig)
			require.NoError(t, err)
			assert.True(t, ok, "root signature must hold; the reject is the schema")
			_, _, err = VerifyAndLoad(rootPub, parsed, fx.Valid.PersistedEpoch,
				time.Unix(fx.Now, 0), time.Duration(fx.MaxStalenessSec)*time.Second)
			require.ErrorContains(t, err, want[i].err)
			require.ErrorContains(t, err, want[i].slug)
			if rc.Name == "two_gods" {
				assert.ErrorContains(t, err, "siavoush")
			}
		})
	}
}

func loadHumansFixture(t *testing.T) humansFixture {
	t.Helper()
	data, err := os.ReadFile(humansFixturePath)
	require.NoError(t, err, "committed humans fixture must exist; run with -update to generate it")
	var fx humansFixture
	require.NoError(t, json.Unmarshal(data, &fx))
	return fx
}
