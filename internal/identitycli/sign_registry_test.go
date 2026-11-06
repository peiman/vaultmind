package identitycli

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

// testRegistryJSON is a valid unsigned registry (one live binding) whose pubkey
// is filled in per-test (the agent key is generated fresh so we can verify the
// emitted distribution envelope).
func testRegistryJSON(pubB64 string) string {
	return `{"agents":[{"authorized_origin_daemons":["daemon-eu-1"],"display_name":"Mira",` +
		`"key_epoch":1,"pubkey":"` + pubB64 + `","slug":"mira",` +
		`"valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":5,"valid_from":1770000000,"valid_until":1780000000}`
}

// TestSignRegistryProducesVerifyingDistributionKeylessly proves SignRegistry
// signs the JCS-canonical registry through the SignerClient seam (no key file)
// and emits a distribution envelope that ParseDistribution+VerifyAndLoad accept.
func TestSignRegistryProducesVerifyingDistributionKeylessly(t *testing.T) {
	rootPub, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey root: %v", err)
	}
	agentPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey agent: %v", err)
	}
	fake := &fakeSignerClient{priv: rootPriv}

	var out bytes.Buffer
	in := testRegistryJSON(base64.StdEncoding.EncodeToString(agentPub))
	if err := SignRegistry(&out, fake, []byte(in)); err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	env, err := registry.ParseDistribution(out.Bytes())
	if err != nil {
		t.Fatalf("ParseDistribution(output): %v\noutput=%q", err, out.String())
	}
	now := time.Unix(1_770_000_500, 0)
	loaded, newHighest, err := registry.VerifyAndLoad(rootPub, env, 4, now, time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad: %v", err)
	}
	if newHighest != 5 {
		t.Fatalf("newHighest = %d, want 5", newHighest)
	}
	if len(loaded.Agents) != 1 || loaded.Agents[0].Slug != "mira" {
		t.Fatalf("loaded registry mismatch: %+v", loaded.Agents)
	}
}

// TestSignRegistryByteIdenticalToDomain proves the CLI path yields the SAME
// distribution as registry.SignRegistryWithSigner over the same registry — the
// CLI wire-decode introduces no drift.
func TestSignRegistryByteIdenticalToDomain(t *testing.T) {
	_, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	agentPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey agent: %v", err)
	}
	fake := &fakeSignerClient{priv: rootPriv}

	var out bytes.Buffer
	in := testRegistryJSON(base64.StdEncoding.EncodeToString(agentPub))
	if err := SignRegistry(&out, fake, []byte(in)); err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}
	cliEnv, err := registry.ParseDistribution(out.Bytes())
	if err != nil {
		t.Fatalf("ParseDistribution: %v", err)
	}

	pk, err := registry.NewPublicKey(agentPub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	reg := registry.Registry{
		Epoch: 5, ValidFrom: 1770000000, ValidUntil: 1780000000,
		Agents: []registry.AgentBinding{{
			Slug: "mira", DisplayName: "Mira", PubKey: pk, KeyEpoch: 1,
			ValidFrom: 1770000000, ValidUntil: 1780000000,
			AuthorizedOriginDaemons: []string{"daemon-eu-1"},
		}},
	}
	domEnv, err := registry.SignRegistryWithSigner(&keyBackedSigner{priv: rootPriv}, reg)
	if err != nil {
		t.Fatalf("SignRegistryWithSigner: %v", err)
	}
	if !bytes.Equal(cliEnv.Registry, domEnv.Registry) {
		t.Fatalf("canonical bytes differ:\n cli=%q\n dom=%q", cliEnv.Registry, domEnv.Registry)
	}
	if !bytes.Equal(cliEnv.RootSig, domEnv.RootSig) {
		t.Fatal("root sigs differ")
	}
}

// keyBackedSigner signs with a held root key (mirrors the registry-package test
// helper) so the CLI-vs-domain parity test can use a deterministic signer.
type keyBackedSigner struct{ priv ed25519.PrivateKey }

func (k *keyBackedSigner) Sign(b []byte) ([]byte, error) { return ed25519.Sign(k.priv, b), nil }

// TestSignRegistryCarriesRevokedAt proves a binding with revoked_at set (a
// revoked rotation tuple) round-trips through the CLI into the signed
// distribution. It covers the revoked_at narrowing branch. The registry pairs a
// live mira@2 with a revoked mira@1 (uniqueness allows one live + one revoked).
func TestSignRegistryCarriesRevokedAt(t *testing.T) {
	rootPub, rootPriv, _ := ed25519.GenerateKey(rand.Reader)
	livePub, _, _ := ed25519.GenerateKey(rand.Reader)
	oldPub, _, _ := ed25519.GenerateKey(rand.Reader)
	fake := &fakeSignerClient{priv: rootPriv}

	in := `{"agents":[` +
		`{"authorized_origin_daemons":["d"],"display_name":"Mira","key_epoch":2,` +
		`"pubkey":"` + base64.StdEncoding.EncodeToString(livePub) + `","slug":"mira",` +
		`"valid_from":1770000000,"valid_until":1780000000},` +
		`{"authorized_origin_daemons":["d"],"display_name":"Mira","key_epoch":1,` +
		`"pubkey":"` + base64.StdEncoding.EncodeToString(oldPub) + `","revoked_at":1770000100,` +
		`"slug":"mira","valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":5,"valid_from":1770000000,"valid_until":1780000000}`

	var out bytes.Buffer
	if err := SignRegistry(&out, fake, []byte(in)); err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}
	env, err := registry.ParseDistribution(out.Bytes())
	if err != nil {
		t.Fatalf("ParseDistribution: %v", err)
	}
	now := time.Unix(1_770_000_500, 0)
	loaded, _, err := registry.VerifyAndLoad(rootPub, env, 4, now, time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad: %v", err)
	}
	var sawRevoked bool
	for _, b := range loaded.Agents {
		if b.KeyEpoch == 1 {
			if b.RevokedAt == nil || *b.RevokedAt != 1770000100 {
				t.Fatalf("revoked_at did not round-trip: %+v", b.RevokedAt)
			}
			sawRevoked = true
		}
	}
	if !sawRevoked {
		t.Fatal("revoked binding (key_epoch 1) missing from loaded registry")
	}
}

func TestSignRegistryRejectsBadBase64PubKey(t *testing.T) {
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	in := testRegistryJSON("not-base64!!!")
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected bad-base64 pubkey rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a bad-base64 pubkey")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must print nothing, got %q", out.String())
	}
}

func TestSignRegistryRejectsSmallOrderPubKey(t *testing.T) {
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	// 32 zero bytes is a small-order point -> NewPublicKey rejects it.
	zero := base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
	in := testRegistryJSON(zero)
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected small-order pubkey rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a small-order pubkey")
	}
}

func TestSignRegistryRejectsUnknownField(t *testing.T) {
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	bad := `{"agents":[],"epoch":5,"valid_from":1,"valid_until":2,"evil":true}`
	if err := SignRegistry(&out, fake, []byte(bad)); err == nil {
		t.Fatal("expected unknown-field rejection, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("parse reject must print nothing, got %q", out.String())
	}
}

func TestSignRegistryRejectsTrailingData(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	in := testRegistryJSON(base64.StdEncoding.EncodeToString(pub)) + ` {"trailing":true}`
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected trailing-data rejection, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("parse reject must print nothing, got %q", out.String())
	}
}

func TestSignRegistryFailsClosedOnSignerError(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	fake := &fakeSignerClient{failErr: sentinelErr("signer unreachable")}
	var out bytes.Buffer
	in := testRegistryJSON(base64.StdEncoding.EncodeToString(pub))
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected fail-closed error, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("fail-closed must print nothing, got %q", out.String())
	}
}

func TestSignRegistryRejectsOutOfRangeEpoch(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	// epoch 0 is below the [1, 2^53] floor -> rejected by the registry gate.
	in := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"M",` +
		`"key_epoch":1,"pubkey":"` + base64.StdEncoding.EncodeToString(pub) + `","slug":"mira",` +
		`"valid_from":1,"valid_until":2}],"epoch":0,"valid_from":1,"valid_until":2}`
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected out-of-range epoch rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for an out-of-range epoch")
	}
}

// TestSignRegistryRejectsRegistryEpochAbove2Pow53 covers the registry-epoch gate
// at the JCS-safe ceiling. The gate runs on the int64 wire value BEFORE narrowing
// (epochInRangeI64), so the wire boundary is the range authority — a >2^53 value
// cannot reach the signer.
func TestSignRegistryRejectsRegistryEpochAbove2Pow53(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	// 2^53 + 1 = 9007199254740993, one past MaxSafeEpoch.
	in := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"M",` +
		`"key_epoch":1,"pubkey":"` + base64.StdEncoding.EncodeToString(pub) + `","slug":"mira",` +
		`"valid_from":1,"valid_until":2}],"epoch":9007199254740993,"valid_from":1,"valid_until":2}`
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected >2^53 registry epoch rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a >2^53 registry epoch")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must print nothing, got %q", out.String())
	}
}

// TestSignRegistryRejectsBindingKeyEpochAbove2Pow53 covers the BINDING key_epoch
// gate (distinct from the registry epoch) — the path the original out-of-range
// test did not exercise.
func TestSignRegistryRejectsBindingKeyEpochAbove2Pow53(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	in := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"M",` +
		`"key_epoch":9007199254740993,"pubkey":"` + base64.StdEncoding.EncodeToString(pub) + `","slug":"mira",` +
		`"valid_from":1,"valid_until":2}],"epoch":1,"valid_from":1,"valid_until":2}`
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected >2^53 binding key_epoch rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a >2^53 binding key_epoch")
	}
}

// TestSignRegistryRejectsWrongLengthPubKey covers a well-formed base64 string
// that decodes to the wrong number of bytes (not 32) — a distinct NewPublicKey
// reject path from bad-base64 and small-order, exercised at the CLI boundary.
func TestSignRegistryRejectsWrongLengthPubKey(t *testing.T) {
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	short := base64.StdEncoding.EncodeToString(make([]byte, 16)) // 16 != 32
	in := testRegistryJSON(short)
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected wrong-length pubkey rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a wrong-length pubkey")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must print nothing, got %q", out.String())
	}
}

// TestSignRegistryRejectsNegativeRevokedAt covers the revoked_at sanity gate: a
// negative (pre-epoch) revocation timestamp fails closed.
func TestSignRegistryRejectsNegativeRevokedAt(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	fake := &fakeSignerClient{}
	var out bytes.Buffer
	in := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"M",` +
		`"key_epoch":1,"pubkey":"` + base64.StdEncoding.EncodeToString(pub) + `","revoked_at":-1,` +
		`"slug":"mira","valid_from":1,"valid_until":2}],"epoch":1,"valid_from":1,"valid_until":2}`
	if err := SignRegistry(&out, fake, []byte(in)); err == nil {
		t.Fatal("expected negative revoked_at rejection, got nil")
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a negative revoked_at")
	}
}

// TestSignRegistryRolesRoundTrip proves sign-registry accepts bridge and
// code_signing roles, that a binding with no roles stays roles-free, and that
// the canonical bytes match the domain signer.
func TestSignRegistryRolesRoundTrip(t *testing.T) {
	rootPub, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey root: %v", err)
	}
	bridgePub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey bridge: %v", err)
	}
	signerPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey signer: %v", err)
	}
	plainPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey plain: %v", err)
	}
	bridgeB64 := base64.StdEncoding.EncodeToString(bridgePub)
	signerB64 := base64.StdEncoding.EncodeToString(signerPub)
	plainB64 := base64.StdEncoding.EncodeToString(plainPub)

	in := `{"agents":[` +
		`{"authorized_origin_daemons":["d"],"display_name":"Bridge","key_epoch":1,` +
		`"pubkey":"` + bridgeB64 + `","roles":[{"kind":"bridge","vouches_for":["human:peiman"]},` +
		`{"kind":"code_signing","platforms":[]}],"slug":"bridge",` +
		`"valid_from":1770000000,"valid_until":1780000000},` +
		`{"authorized_origin_daemons":["d"],"display_name":"Signer","key_epoch":1,` +
		`"pubkey":"` + signerB64 + `","roles":[{"kind":"code_signing","platforms":["darwin-arm64","linux-amd64"]}],` +
		`"slug":"signer","valid_from":1770000000,"valid_until":1780000000},` +
		`{"authorized_origin_daemons":["d"],"display_name":"Plain","key_epoch":1,` +
		`"pubkey":"` + plainB64 + `","slug":"plain",` +
		`"valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":5,"valid_from":1770000000,"valid_until":1780000000}`

	fake := &fakeSignerClient{priv: rootPriv}
	var out bytes.Buffer
	if err := SignRegistry(&out, fake, []byte(in)); err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}
	env, err := registry.ParseDistribution(out.Bytes())
	if err != nil {
		t.Fatalf("ParseDistribution: %v", err)
	}
	now := time.Unix(1_770_000_500, 0)
	loaded, _, err := registry.VerifyAndLoad(rootPub, env, 4, now, time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad: %v", err)
	}
	want := map[string][]registry.Role{
		"bridge": {
			registry.NewBridgeRole([]string{"human:peiman"}),
			registry.NewCodeSigningRole(nil),
		},
		"signer": {registry.NewCodeSigningRole([]string{"darwin-arm64", "linux-amd64"})},
		"plain":  nil,
	}
	if len(loaded.Agents) != len(want) {
		t.Fatalf("loaded %d agents, want %d", len(loaded.Agents), len(want))
	}
	for _, b := range loaded.Agents {
		if !rolesEqual(b.Roles, want[b.Slug]) {
			t.Fatalf("slug %s roles = %#v", b.Slug, roleView(b.Roles))
		}
	}
	if strings.Contains(agentObject(env.Registry, "plain"), "roles") {
		t.Fatalf("roles-free binding emitted roles: %s", env.Registry)
	}

	bridgePK, err := registry.NewPublicKey(bridgePub)
	if err != nil {
		t.Fatalf("NewPublicKey bridge: %v", err)
	}
	signerPK, err := registry.NewPublicKey(signerPub)
	if err != nil {
		t.Fatalf("NewPublicKey signer: %v", err)
	}
	plainPK, err := registry.NewPublicKey(plainPub)
	if err != nil {
		t.Fatalf("NewPublicKey plain: %v", err)
	}
	dom, err := registry.SignRegistryWithSigner(&keyBackedSigner{priv: rootPriv}, registry.Registry{
		Epoch: 5, ValidFrom: 1770000000, ValidUntil: 1780000000,
		Agents: []registry.AgentBinding{
			{Slug: "bridge", DisplayName: "Bridge", PubKey: bridgePK, KeyEpoch: 1,
				ValidFrom: 1770000000, ValidUntil: 1780000000, AuthorizedOriginDaemons: []string{"d"},
				Roles: want["bridge"]},
			{Slug: "signer", DisplayName: "Signer", PubKey: signerPK, KeyEpoch: 1,
				ValidFrom: 1770000000, ValidUntil: 1780000000, AuthorizedOriginDaemons: []string{"d"},
				Roles: want["signer"]},
			{Slug: "plain", DisplayName: "Plain", PubKey: plainPK, KeyEpoch: 1,
				ValidFrom: 1770000000, ValidUntil: 1780000000, AuthorizedOriginDaemons: []string{"d"}},
		},
	})
	if err != nil {
		t.Fatalf("SignRegistryWithSigner: %v", err)
	}
	if !bytes.Equal(env.Registry, dom.Registry) {
		t.Fatalf("CLI canonical bytes differ from domain:\n cli=%s\n dom=%s", env.Registry, dom.Registry)
	}
}

func TestSignRegistryRejectsMalformedRoles(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString(pub)
	cases := []struct {
		name  string
		roles string
	}{
		{name: "unknown_kind", roles: `[{"kind":"wizard","vouches_for":["human:peiman"]}]`},
		{name: "bridge_extra_field", roles: `[{"kind":"bridge","vouches_for":["human:peiman"],"extra":true}]`},
		{name: "bridge_missing_vouches_for", roles: `[{"kind":"bridge"}]`},
		{name: "code_signing_missing_platforms", roles: `[{"kind":"code_signing"}]`},
		{name: "roles_not_array", roles: `{"kind":"bridge"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeSignerClient{}
			var out bytes.Buffer
			in := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"M",` +
				`"key_epoch":1,"pubkey":"` + b64 + `","roles":` + tc.roles + `,"slug":"mira",` +
				`"valid_from":1770000000,"valid_until":1780000000}],` +
				`"epoch":5,"valid_from":1770000000,"valid_until":1780000000}`
			err := SignRegistry(&out, fake, []byte(in))
			if err == nil || !strings.Contains(err.Error(), ErrRegistryMalformedRole) {
				t.Fatalf("got %v, want substring %q", err, ErrRegistryMalformedRole)
			}
			if fake.gotCanonical != nil {
				t.Fatal("signer was called for a malformed role")
			}
			if out.Len() != 0 {
				t.Fatalf("reject must print nothing, got %q", out.String())
			}
		})
	}
}

// nfdAcute is U+0301 COMBINING ACUTE ACCENT, written as an escape so the
// source cannot be precomposed to U+00E9.
const nfdAcute = "\u0301"

// TestSignRegistryRejectsNonNFCStrings proves the unsigned sign-registry input
// is refused when any string — display name, vouches_for, or platforms — is not
// Unicode NFC. The signer is not called and nothing is printed.
func TestSignRegistryRejectsNonNFCStrings(t *testing.T) {
	_, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey root: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString(pub)
	base := testRegistryJSON(b64)
	cases := []struct {
		name string
		in   string
	}{
		{name: "display_name", in: strings.Replace(base, `"display_name":"Mira"`, `"display_name":"Mira e`+nfdAcute+`"`, 1)},
		{name: "vouches_for", in: strings.Replace(base, `"slug":"mira"`, `"roles":[{"kind":"bridge","vouches_for":["human:peima`+nfdAcute+`n"]}],"slug":"mira"`, 1)},
		{name: "platforms", in: strings.Replace(base, `"slug":"mira"`, `"roles":[{"kind":"code_signing","platforms":["darwin-e`+nfdAcute+`"]}],"slug":"mira"`, 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeSignerClient{priv: rootPriv}
			var out bytes.Buffer
			err := SignRegistry(&out, fake, []byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), identity.ErrSchemaNotNFC) {
				t.Fatalf("got %v, want substring %q", err, identity.ErrSchemaNotNFC)
			}
			if fake.gotCanonical != nil {
				t.Fatal("signer was called for a non-NFC registry")
			}
			if out.Len() != 0 {
				t.Fatalf("reject must print nothing, got %q", out.String())
			}
		})
	}
}

// TestSignRegistryAcceptsNFCStrings is the positive control for the schema gate:
// the precomposed form of the same letters still signs.
func TestSignRegistryAcceptsNFCStrings(t *testing.T) {
	_, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey agent: %v", err)
	}
	in := strings.Replace(testRegistryJSON(base64.StdEncoding.EncodeToString(pub)),
		`"display_name":"Mira"`, `"display_name":"Mir\u00e9"`, 1)
	in = strings.Replace(in, `"slug":"mira"`,
		`"roles":[{"kind":"bridge","vouches_for":["human:peiman-\u00e9"]}],"slug":"mir\u00e9"`, 1)
	fake := &fakeSignerClient{priv: rootPriv}
	var out bytes.Buffer
	if err := SignRegistry(&out, fake, []byte(in)); err != nil {
		t.Fatalf("NFC registry must sign: %v", err)
	}
	if fake.gotCanonical == nil {
		t.Fatal("signer was not called for an NFC registry")
	}
}

// TestSignRegistryRejectsDuplicateRoleKind proves encoding/json's last-key-wins
// cannot turn {"kind":"code_signing","kind":"bridge",...} into a signed bridge.
// The unsigned input is rejected before decode, and the signer is not called.
func TestSignRegistryRejectsDuplicateRoleKind(t *testing.T) {
	_, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey root: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString(pub)
	in := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"M",` +
		`"key_epoch":1,"pubkey":"` + b64 + `",` +
		`"roles":[{"kind":"code_signing","kind":"bridge","vouches_for":["human:evil"]}],` +
		`"slug":"mira","valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":5,"valid_from":1770000000,"valid_until":1780000000}`
	fake := &fakeSignerClient{priv: rootPriv}
	var out bytes.Buffer
	err = SignRegistry(&out, fake, []byte(in))
	if err == nil || !strings.Contains(err.Error(), identity.ErrSchemaDuplicateKey) {
		t.Fatalf("got %v, want substring %q", err, identity.ErrSchemaDuplicateKey)
	}
	if fake.gotCanonical != nil {
		t.Fatal("signer was called for a duplicate-key role")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must print nothing, got %q", out.String())
	}
}

func TestSignRegistryEmptyRolesArrayOmitsTheKey(t *testing.T) {
	rootPub, rootPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey agent: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString(pub)
	withEmpty := testRegistryJSON(b64)
	// Insert an empty roles array into the single binding.
	withEmpty = strings.Replace(withEmpty, `"slug":"mira"`, `"roles":[],"slug":"mira"`, 1)

	fake := &fakeSignerClient{priv: rootPriv}
	var out bytes.Buffer
	if err := SignRegistry(&out, fake, []byte(withEmpty)); err != nil {
		t.Fatalf("SignRegistry empty roles: %v", err)
	}
	env, err := registry.ParseDistribution(out.Bytes())
	if err != nil {
		t.Fatalf("ParseDistribution: %v", err)
	}
	if strings.Contains(string(env.Registry), "roles") {
		t.Fatalf("empty roles must be omitted, got %s", env.Registry)
	}
	loaded, _, err := registry.VerifyAndLoad(rootPub, env, 4, time.Unix(1_770_000_500, 0), time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad: %v", err)
	}
	if len(loaded.Agents) != 1 || len(loaded.Agents[0].Roles) != 0 {
		t.Fatalf("empty roles did not load as no roles: %+v", loaded.Agents)
	}
}

func TestWireRegistryRoundTripsRoles(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pk, err := registry.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	reg := registry.Registry{
		Epoch: 5, ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000,
		Agents: []registry.AgentBinding{{
			Slug: "mira", DisplayName: "Mira", PubKey: pk, KeyEpoch: 1,
			ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000,
			AuthorizedOriginDaemons: []string{"daemon-eu-1"},
			Roles: []registry.Role{
				registry.NewBridgeRole([]string{"human:peiman"}),
			},
		}},
	}
	wire, err := toWireRegistry(reg)
	if err != nil {
		t.Fatalf("toWireRegistry: %v", err)
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded, err := decodeWireRegistry(raw)
	if err != nil {
		t.Fatalf("decodeWireRegistry: %v", err)
	}
	built, err := buildRegistry(decoded)
	if err != nil {
		t.Fatalf("buildRegistry: %v", err)
	}
	if !rolesEqual(built.Agents[0].Roles, reg.Agents[0].Roles) {
		t.Fatalf("roles dropped on wire round-trip: %#v", roleView(built.Agents[0].Roles))
	}
}

func rolesEqual(got, want []registry.Role) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i].Kind() != want[i].Kind() {
			return false
		}
		if strings.Join(got[i].VouchesFor(), "\x00") != strings.Join(want[i].VouchesFor(), "\x00") {
			return false
		}
		if strings.Join(got[i].Platforms(), "\x00") != strings.Join(want[i].Platforms(), "\x00") {
			return false
		}
	}
	return true
}

func roleView(roles []registry.Role) []string {
	out := make([]string, len(roles))
	for i, r := range roles {
		out[i] = string(r.Kind()) + " vouches=" + strings.Join(r.VouchesFor(), ",") + " platforms=" + strings.Join(r.Platforms(), ",")
	}
	return out
}

func agentObject(canonical []byte, slug string) string {
	s := string(canonical)
	i := strings.Index(s, `"slug":"`+slug+`"`)
	if i < 0 {
		return s
	}
	start := strings.LastIndex(s[:i], "{")
	if start < 0 {
		return s
	}
	return s[start:]
}
