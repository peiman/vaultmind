package identitycli

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity/enrollment"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

// ── test helpers ──────────────────────────────────────────────────────────

// fixedAgentKey derives a deterministic ed25519 keypair from a one-byte seed
// fill so test pubkeys are LOW-ENTROPY (the gitleaks entropy scanner stays
// quiet). NewKeyFromSeed yields a validatable (non-small-order) key.
func fixedAgentKey(t *testing.T, fill byte) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	seed := bytes.Repeat([]byte{fill}, ed25519.SeedSize)
	priv := ed25519.NewKeyFromSeed(seed)
	return priv.Public().(ed25519.PublicKey), priv
}

// lowEntropyTransport is a deterministic 32-byte (length-only-checked) WireGuard
// transport pubkey, base64-std. It is NOT validated as a key, so a repeated byte
// keeps entropy low.
func lowEntropyTransport() string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
}

// signedRequestFor builds a signed enrollment-request wire JSON for the agent
// (slug, networkID) at created, signing the canonical bytes with agentPriv so
// proof-of-possession verifies. It is the output a member's `identity enroll`
// would produce.
func signedRequestFor(t *testing.T, agentPub ed25519.PublicKey, agentPriv ed25519.PrivateKey, slug, networkID string, created int64) []byte {
	t.Helper()
	f := enrollment.Fields{
		AlgVersion:      enrollment.AlgVersion,
		Created:         created,
		DisplayName:     "Mira ⭐",
		KeyEpoch:        1,
		NetworkID:       networkID,
		Nonce:           "YWJjZGVmZ2hpamtsbW5vcA==",
		PubKey:          base64.StdEncoding.EncodeToString(agentPub),
		Slug:            slug,
		TransportPubKey: lowEntropyTransport(),
	}
	canon, err := enrollment.CanonicalizeEnrollment(f)
	if err != nil {
		t.Fatalf("CanonicalizeEnrollment: %v", err)
	}
	sig := ed25519.Sign(agentPriv, canon.Bytes())
	raw, err := enrollment.MarshalWire(f, base64.StdEncoding.EncodeToString(sig))
	if err != nil {
		t.Fatalf("MarshalWire: %v", err)
	}
	return raw
}

// unsignedRegistryJSON renders an unsigned wireRegistry JSON with one binding.
func unsignedRegistryJSON(slug, pubB64 string, keyEpoch, epoch int64) string {
	return `{"agents":[{"authorized_origin_daemons":["daemon-eu-1"],"display_name":"Existing",` +
		`"key_epoch":` + strconv.FormatInt(keyEpoch, 10) + `,"pubkey":"` + pubB64 + `","slug":"` + slug + `",` +
		`"valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":` + strconv.FormatInt(epoch, 10) + `,"valid_from":1770000000,"valid_until":1780000000}`
}

// signedEnvelopeFrom root-signs an unsigned wireRegistry JSON into a distribution
// envelope (the deployed artifact shape) using rootPriv.
func signedEnvelopeFrom(t *testing.T, rootPriv ed25519.PrivateKey, unsignedJSON string) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := SignRegistry(&out, &keyBackedSigner{priv: rootPriv}, []byte(unsignedJSON)); err != nil {
		t.Fatalf("SignRegistry(envelope): %v", err)
	}
	return out.Bytes()
}

// networkOf is the admin network id derived from a root pubkey.
func networkOf(rootPub ed25519.PublicKey) string {
	return registry.NetworkID(rootPub)
}

// decodeEmitted parses the emitted UNSIGNED wireRegistry JSON from EnrollAdd's
// stdout into a generic map so a test can assert structure.
func decodeEmitted(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode emitted registry: %v\nemitted=%q", err, string(b))
	}
	return m
}

func fixedNow(unix int64) func() time.Time {
	return func() time.Time { return time.Unix(unix, 0) }
}

const testValiditySeconds = "1000000"

// ── happy path ────────────────────────────────────────────────────────────

// TestEnrollAddAppendsAndBumpsEpoch is the happy path: a valid signed request
// for the admin network is appended to a fresh registry; the emitted UNSIGNED
// registry bumps the epoch, refreshes the issuance window, and round-trips
// buildRegistry+canonicalizes (guaranteed signable).
func TestEnrollAddAppendsAndBumpsEpoch(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)
	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)

	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64:   base64.StdEncoding.EncodeToString(rootPub),
		ValiditySeconds: testValiditySeconds,
		OriginDaemons:   "daemon-eu-1,daemon-eu-2",
		Now:             fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err != nil {
		t.Fatalf("EnrollAdd: %v", err)
	}

	m := decodeEmitted(t, out.Bytes())
	if epoch, _ := m["epoch"].(float64); epoch != 1 {
		t.Fatalf("fresh registry first emit epoch = %v, want 1", m["epoch"])
	}
	agents, _ := m["agents"].([]any)
	if len(agents) != 1 {
		t.Fatalf("agents len = %d, want 1", len(agents))
	}
	b0, _ := agents[0].(map[string]any)
	if b0["slug"] != "mira" {
		t.Fatalf("appended slug = %v, want mira", b0["slug"])
	}
	if b0["pubkey"] != base64.StdEncoding.EncodeToString(agentPub) {
		t.Fatalf("appended pubkey mismatch")
	}
	// Window refresh: valid_from == now, valid_until == now+validity.
	if vf, _ := b0["valid_from"].(float64); int64(vf) != 1_770_000_500 {
		t.Fatalf("binding valid_from = %v, want now", b0["valid_from"])
	}
	if vu, _ := b0["valid_until"].(float64); int64(vu) != 1_770_000_500+1_000_000 {
		t.Fatalf("binding valid_until = %v, want now+validity", b0["valid_until"])
	}
	// AuthorizedOriginDaemons parsed from the comma list.
	daemons, _ := b0["authorized_origin_daemons"].([]any)
	if len(daemons) != 2 || daemons[0] != "daemon-eu-1" || daemons[1] != "daemon-eu-2" {
		t.Fatalf("authorized_origin_daemons = %v, want [daemon-eu-1 daemon-eu-2]", daemons)
	}

	// The emitted registry round-trips buildRegistry + canonicalizes (signable).
	assertSignable(t, out.Bytes())
	// Guidance mentions the next step (sign-registry).
	if !strings.Contains(errOut.String(), "sign-registry") {
		t.Fatalf("guidance missing sign-registry next step: %q", errOut.String())
	}
}

// assertSignable proves the emitted UNSIGNED registry JSON is a valid input to
// the sign path: it decodes + builds + canonicalizes without error.
func assertSignable(t *testing.T, emitted []byte) {
	t.Helper()
	w, err := decodeWireRegistry(emitted)
	if err != nil {
		t.Fatalf("emitted registry does not decode: %v", err)
	}
	reg, err := buildRegistry(w)
	if err != nil {
		t.Fatalf("emitted registry does not build: %v", err)
	}
	if _, err := registry.SignRegistryWithSigner(&keyBackedSigner{priv: signKeyForCanon(t)}, reg); err != nil {
		t.Fatalf("emitted registry does not canonicalize/sign: %v", err)
	}
}

func signKeyForCanon(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv := fixedAgentKey(t, 0x09)
	return priv
}

// TestEnrollAddEmittedRegistrySignsAndVerifies pipes the emitted unsigned
// registry into the SIGN path (a fake signer) and proves the resulting
// distribution envelope VerifyAndLoads under the root key.
func TestEnrollAddEmittedRegistrySignsAndVerifies(t *testing.T) {
	rootPub, rootPriv := fixedAgentKey(t, 0x01)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)
	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)

	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64:   base64.StdEncoding.EncodeToString(rootPub),
		ValiditySeconds: testValiditySeconds,
		Now:             fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err != nil {
		t.Fatalf("EnrollAdd: %v", err)
	}

	var signed bytes.Buffer
	if err := SignRegistry(&signed, &fakeSignerClient{priv: rootPriv}, out.Bytes()); err != nil {
		t.Fatalf("SignRegistry(emitted): %v", err)
	}
	env, err := registry.ParseDistribution(signed.Bytes())
	if err != nil {
		t.Fatalf("ParseDistribution: %v", err)
	}
	now := time.Unix(1_770_000_600, 0)
	loaded, _, err := registry.VerifyAndLoad(rootPub, env, 0, now, time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad emitted+signed: %v", err)
	}
	if len(loaded.Agents) != 1 || loaded.Agents[0].Slug != "mira" {
		t.Fatalf("loaded mismatch: %+v", loaded.Agents)
	}
}

// ── proof-of-possession + gate rejects ────────────────────────────────────

// TestEnrollAddRejectsPoPFailure proves a request whose sig was made by a
// DIFFERENT key than its pubkey field (proof-of-possession fails) is refused and
// nothing is emitted.
func TestEnrollAddRejectsPoPFailure(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	agentPub, _ := fixedAgentKey(t, 0x02)
	_, wrongPriv := fixedAgentKey(t, 0x03)
	net := networkOf(rootPub)

	// Sign with wrongPriv but claim agentPub -> PoP must fail.
	f := enrollment.Fields{
		AlgVersion: enrollment.AlgVersion, Created: 1_700_000_000, DisplayName: "Mira",
		KeyEpoch: 1, NetworkID: net, Nonce: "YWJj", PubKey: base64.StdEncoding.EncodeToString(agentPub),
		Slug: "mira", TransportPubKey: lowEntropyTransport(),
	}
	canon, _ := enrollment.CanonicalizeEnrollment(f)
	badSig := ed25519.Sign(wrongPriv, canon.Bytes())
	req, _ := enrollment.MarshalWire(f, base64.StdEncoding.EncodeToString(badSig))

	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub), Now: fixedNow(1_770_000_500)}
	err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg)
	if err == nil {
		t.Fatal("expected proof-of-possession rejection, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must emit nothing, got %q", out.String())
	}
}

// TestEnrollAddRejectsGateFailure proves a request that fails a pre-sign gate
// (e.g. alg_version != 1) is refused (VerifyEnrollment returns a gate error).
func TestEnrollAddRejectsGateFailure(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)
	// Hand-build a wire request with alg_version 2 (anti-downgrade gate trips).
	req := []byte(`{"alg_version":2,"created":1700000000,"display_name":"Mira",` +
		`"key_epoch":1,"network_id":"` + net + `","nonce":"YWJj","pubkey":"` +
		base64.StdEncoding.EncodeToString(agentPub) + `","slug":"mira","transport_pubkey":"` +
		lowEntropyTransport() + `","sig":"` + base64.StdEncoding.EncodeToString(ed25519.Sign(agentPriv, []byte("x"))) + `"}`)

	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub), Now: fixedNow(1_770_000_500)}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err == nil {
		t.Fatal("expected gate rejection, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must emit nothing, got %q", out.String())
	}
}

// ── network binding matrix ─────────────────────────────────────────────────

// TestEnrollAddNetworkSpecifierMatrix exercises the admin-network resolution:
// root-pubkey only, network-id only, both-agree, both-disagree (reject), and
// neither (reject).
func TestEnrollAddNetworkSpecifierMatrix(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)
	rootB64 := base64.StdEncoding.EncodeToString(rootPub)
	otherNet := "vmnet1:00000000000000000000000000000000"

	cases := []struct {
		name      string
		rootPub   string
		networkID string
		wantErr   bool
	}{
		{"root-only", rootB64, "", false},
		{"id-only", "", net, false},
		{"both-agree", rootB64, net, false},
		{"both-disagree", rootB64, otherNet, true},
		{"neither", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)
			var out, errOut bytes.Buffer
			cfg := EnrollAddConfig{
				RootPubKeyB64:   tc.rootPub,
				NetworkID:       tc.networkID,
				ValiditySeconds: testValiditySeconds,
				Now:             fixedNow(1_770_000_500),
			}
			err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg)
			if tc.wantErr && err == nil {
				t.Fatalf("%s: expected error, got nil", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}
			if tc.wantErr && out.Len() != 0 {
				t.Fatalf("%s: reject must emit nothing, got %q", tc.name, out.String())
			}
		})
	}
}

// TestEnrollAddRejectsCrossNetworkRequest_RootPath proves a request whose
// network_id != the admin network (derived from --root-pubkey) is refused.
func TestEnrollAddRejectsCrossNetworkRequest_RootPath(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	// Request is for a DIFFERENT network than the admin root.
	req := signedRequestFor(t, agentPub, agentPriv, "mira", "vmnet1:deadbeefdeadbeefdeadbeefdeadbeef", 1_700_000_000)

	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub), Now: fixedNow(1_770_000_500)}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err == nil {
		t.Fatal("expected cross-network rejection (root path), got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must emit nothing, got %q", out.String())
	}
}

// TestEnrollAddRejectsCrossNetworkRequest_IDPath proves the same refusal when the
// admin network is specified via --network-id only.
func TestEnrollAddRejectsCrossNetworkRequest_IDPath(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	adminNet := networkOf(rootPub)
	req := signedRequestFor(t, agentPub, agentPriv, "mira", "vmnet1:deadbeefdeadbeefdeadbeefdeadbeef", 1_700_000_000)

	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{NetworkID: adminNet, Now: fixedNow(1_770_000_500)}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err == nil {
		t.Fatal("expected cross-network rejection (id path), got nil")
	}
}

// ── signed-envelope --registry input ──────────────────────────────────────

// TestEnrollAddAcceptsSignedEnvelopeInput proves a signed distribution envelope
// (the deployed artifact) with a VALID root_sig is integrity-verified, its inner
// registry extracted, and the new binding appended.
func TestEnrollAddAcceptsSignedEnvelopeInput(t *testing.T) {
	rootPub, rootPriv := fixedAgentKey(t, 0x01)
	existingPub, _ := fixedAgentKey(t, 0x05)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)

	// Current registry already has "alpha" at epoch 7.
	unsigned := unsignedRegistryJSON("alpha", base64.StdEncoding.EncodeToString(existingPub), 1, 7)
	envelope := signedEnvelopeFrom(t, rootPriv, unsigned)

	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)
	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64:   base64.StdEncoding.EncodeToString(rootPub),
		RegistryInput:   envelope,
		ValiditySeconds: testValiditySeconds,
		Now:             fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err != nil {
		t.Fatalf("EnrollAdd(signed envelope): %v", err)
	}
	m := decodeEmitted(t, out.Bytes())
	if epoch, _ := m["epoch"].(float64); epoch != 8 {
		t.Fatalf("epoch = %v, want 8 (7+1)", m["epoch"])
	}
	agents, _ := m["agents"].([]any)
	if len(agents) != 2 {
		t.Fatalf("agents len = %d, want 2 (alpha + mira)", len(agents))
	}
	assertSignable(t, out.Bytes())
}

// TestEnrollAddRejectsTamperedSignedEnvelope proves a signed envelope whose
// root_sig does NOT cover the registry bytes (tampered) is refused — never
// mutate untrusted state.
func TestEnrollAddRejectsTamperedSignedEnvelope(t *testing.T) {
	rootPub, rootPriv := fixedAgentKey(t, 0x01)
	_, wrongRootPriv := fixedAgentKey(t, 0x04)
	existingPub, _ := fixedAgentKey(t, 0x05)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)

	unsigned := unsignedRegistryJSON("alpha", base64.StdEncoding.EncodeToString(existingPub), 1, 7)
	// Sign with the WRONG root key -> root_sig will not verify under rootPub.
	envelope := signedEnvelopeFrom(t, wrongRootPriv, unsigned)
	_ = rootPriv

	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)
	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub),
		RegistryInput: envelope,
		Now:           fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err == nil {
		t.Fatal("expected tampered-envelope rejection, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must emit nothing, got %q", out.String())
	}
}

// TestEnrollAddRejectsSignedEnvelopeWithoutRootPubKey proves a signed envelope
// input WITHOUT --root-pubkey is refused (cannot integrity-verify without the
// root key).
func TestEnrollAddRejectsSignedEnvelopeWithoutRootPubKey(t *testing.T) {
	rootPub, rootPriv := fixedAgentKey(t, 0x01)
	existingPub, _ := fixedAgentKey(t, 0x05)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)

	unsigned := unsignedRegistryJSON("alpha", base64.StdEncoding.EncodeToString(existingPub), 1, 7)
	envelope := signedEnvelopeFrom(t, rootPriv, unsigned)

	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)
	var out, errOut bytes.Buffer
	// network-id alone resolves the network, but a signed envelope still needs the
	// root pubkey to integrity-verify -> reject.
	cfg := EnrollAddConfig{NetworkID: net, RegistryInput: envelope, Now: fixedNow(1_770_000_500)}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err == nil {
		t.Fatal("expected signed-envelope-without-root-pubkey rejection, got nil")
	}
}

// TestEnrollAddAcceptsUnsignedRegistryInput proves an UNSIGNED wireRegistry input
// (the two-verb intermediate) is used directly (no integrity verify needed).
func TestEnrollAddAcceptsUnsignedRegistryInput(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	existingPub, _ := fixedAgentKey(t, 0x05)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)

	unsigned := unsignedRegistryJSON("alpha", base64.StdEncoding.EncodeToString(existingPub), 1, 7)
	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)
	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64:   base64.StdEncoding.EncodeToString(rootPub),
		RegistryInput:   []byte(unsigned),
		ValiditySeconds: testValiditySeconds,
		Now:             fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err != nil {
		t.Fatalf("EnrollAdd(unsigned input): %v", err)
	}
	m := decodeEmitted(t, out.Bytes())
	if epoch, _ := m["epoch"].(float64); epoch != 8 {
		t.Fatalf("epoch = %v, want 8", m["epoch"])
	}
	assertSignable(t, out.Bytes())
}

// TestEnrollAddRejectsUnrecognizedRegistryShape proves a registry input that is
// neither a signed envelope nor an unsigned wireRegistry is refused.
func TestEnrollAddRejectsUnrecognizedRegistryShape(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)
	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)

	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub),
		RegistryInput: []byte(`{"unexpected":"shape"}`),
		Now:           fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err == nil {
		t.Fatal("expected unrecognized-registry-shape rejection, got nil")
	}
}

// ── uniqueness (fail-closed, pre-append) ──────────────────────────────────

// TestEnrollAddRejectsLiveSlugDuplicate proves adding a slug that already has a
// LIVE binding is refused (≤1 live binding per slug; revoke before re-adding).
func TestEnrollAddRejectsLiveSlugDuplicate(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	existingPub, _ := fixedAgentKey(t, 0x05)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)

	// Current registry already has a LIVE "mira".
	unsigned := unsignedRegistryJSON("mira", base64.StdEncoding.EncodeToString(existingPub), 1, 7)
	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)
	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub),
		RegistryInput: []byte(unsigned),
		Now:           fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err == nil {
		t.Fatal("expected live-slug-duplicate rejection, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must emit nothing, got %q", out.String())
	}
}

// TestEnrollAddRejectsSlugKeyEpochDuplicate proves adding a binding with the same
// {slug,key_epoch} tuple as an existing (even revoked) one is refused.
func TestEnrollAddRejectsSlugKeyEpochDuplicate(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	existingPub, _ := fixedAgentKey(t, 0x05)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)

	// Existing binding for "mira" at key_epoch 1, REVOKED (so it is not a live
	// dup) — but the new request is also slug=mira key_epoch=1 -> tuple dup.
	unsigned := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"Old","key_epoch":1,` +
		`"pubkey":"` + base64.StdEncoding.EncodeToString(existingPub) + `","revoked_at":1769999999,` +
		`"slug":"mira","valid_from":1760000000,"valid_until":1780000000}],` +
		`"epoch":7,"valid_from":1760000000,"valid_until":1780000000}`

	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)
	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub),
		RegistryInput: []byte(unsigned),
		Now:           fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err == nil {
		t.Fatal("expected {slug,key_epoch} duplicate rejection, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must emit nothing, got %q", out.String())
	}
}

// TestEnrollAddAllowsRevokedSameSlug proves a request for a slug that exists ONLY
// in a REVOKED form (and at a different key_epoch — the request is always
// key_epoch 1, so the revoked one must be a different epoch) is allowed.
func TestEnrollAddAllowsRevokedSameSlug(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	oldPub, _ := fixedAgentKey(t, 0x05)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)

	// "mira" exists ONLY revoked, at key_epoch 9 (different from the request's
	// key_epoch 1) -> neither a live-slug dup nor a tuple dup.
	unsigned := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"Old","key_epoch":9,` +
		`"pubkey":"` + base64.StdEncoding.EncodeToString(oldPub) + `","revoked_at":1769999999,` +
		`"slug":"mira","valid_from":1760000000,"valid_until":1780000000}],` +
		`"epoch":7,"valid_from":1760000000,"valid_until":1780000000}`

	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)
	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64:   base64.StdEncoding.EncodeToString(rootPub),
		RegistryInput:   []byte(unsigned),
		ValiditySeconds: testValiditySeconds,
		Now:             fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err != nil {
		t.Fatalf("EnrollAdd(revoked-same-slug): %v", err)
	}
	m := decodeEmitted(t, out.Bytes())
	agents, _ := m["agents"].([]any)
	if len(agents) != 2 {
		t.Fatalf("agents len = %d, want 2 (revoked mira@9 + live mira@1)", len(agents))
	}
	assertSignable(t, out.Bytes())
}

// ── fresh registry + epoch overflow ───────────────────────────────────────

// TestEnrollAddFreshRegistryEmitsEpoch1 proves that with NO --registry input, a
// fresh empty registry (epoch 0) yields a first emit at epoch 1.
func TestEnrollAddFreshRegistryEmitsEpoch1(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)
	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)

	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64:   base64.StdEncoding.EncodeToString(rootPub),
		ValiditySeconds: testValiditySeconds,
		Now:             fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err != nil {
		t.Fatalf("EnrollAdd(fresh): %v", err)
	}
	m := decodeEmitted(t, out.Bytes())
	if epoch, _ := m["epoch"].(float64); epoch != 1 {
		t.Fatalf("fresh emit epoch = %v, want 1", m["epoch"])
	}
}

// TestEnrollAddRejectsEpochOverflow proves a current registry at MaxSafeEpoch
// cannot bump (newEpoch would exceed the JCS-safe ceiling) and is refused.
func TestEnrollAddRejectsEpochOverflow(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	existingPub, _ := fixedAgentKey(t, 0x05)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)

	// Current epoch == MaxSafeEpoch (2^53) -> +1 overflows.
	unsigned := unsignedRegistryJSON("alpha", base64.StdEncoding.EncodeToString(existingPub), 1, registry.MaxSafeEpoch)
	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)
	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub),
		RegistryInput: []byte(unsigned),
		Now:           fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err == nil {
		t.Fatal("expected epoch-overflow rejection, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must emit nothing, got %q", out.String())
	}
}

// ── malformed-current-registry + bad config ───────────────────────────────

// TestEnrollAddRejectsMalformedCurrentRegistry proves a current registry with a
// bad binding (small-order pubkey) fails the buildRegistry validation.
func TestEnrollAddRejectsMalformedCurrentRegistry(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)
	// 32 zero bytes is a small-order point -> buildRegistry rejects it.
	zero := base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
	unsigned := unsignedRegistryJSON("alpha", zero, 1, 7)

	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)
	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub),
		RegistryInput: []byte(unsigned),
		Now:           fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err == nil {
		t.Fatal("expected malformed-current-registry rejection, got nil")
	}
}

// TestEnrollAddRejectsBadValiditySeconds proves a non-numeric --validity-seconds
// is refused.
func TestEnrollAddRejectsBadValiditySeconds(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)
	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)

	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64:   base64.StdEncoding.EncodeToString(rootPub),
		ValiditySeconds: "not-a-number",
		Now:             fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err == nil {
		t.Fatal("expected bad-validity-seconds rejection, got nil")
	}
}

// TestEnrollAddRejectsBadRootPubKey proves a non-base64 / invalid --root-pubkey
// is refused before any mutation.
func TestEnrollAddRejectsBadRootPubKey(t *testing.T) {
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	req := signedRequestFor(t, agentPub, agentPriv, "mira", "vmnet1:aa", 1_700_000_000)
	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{RootPubKeyB64: "not-base64!!!", Now: fixedNow(1_770_000_500)}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err == nil {
		t.Fatal("expected bad-root-pubkey rejection, got nil")
	}
}

// TestEnrollAddRejectsEmptyRequest proves an empty request input is refused
// (fail closed before any network/registry work).
func TestEnrollAddRejectsEmptyRequest(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub), Now: fixedNow(1_770_000_500)}
	if err := EnrollAdd(&out, &errOut, strings.NewReader("   \n"), cfg); err == nil {
		t.Fatal("expected empty-request rejection, got nil")
	}
}

// TestEnrollAddRejectsMalformedRequestJSON proves a request that is not a valid
// strict wire request is refused.
func TestEnrollAddRejectsMalformedRequestJSON(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub), Now: fixedNow(1_770_000_500)}
	if err := EnrollAdd(&out, &errOut, strings.NewReader(`{"alg_version":1`), cfg); err == nil {
		t.Fatal("expected malformed-request rejection, got nil")
	}
	if out.Len() != 0 {
		t.Fatalf("reject must emit nothing, got %q", out.String())
	}
}

// TestEnrollAddRejectsBadRequestSigBase64 proves a request whose sig field is not
// valid base64-std is refused before the verify step.
func TestEnrollAddRejectsBadRequestSigBase64(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	agentPub, _ := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)
	// A well-formed wire request whose sig is NOT valid base64.
	req := []byte(`{"alg_version":1,"created":1700000000,"display_name":"Mira",` +
		`"key_epoch":1,"network_id":"` + net + `","nonce":"YWJj","pubkey":"` +
		base64.StdEncoding.EncodeToString(agentPub) + `","slug":"mira","transport_pubkey":"` +
		lowEntropyTransport() + `","sig":"not-base64!!!"}`)

	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub), Now: fixedNow(1_770_000_500)}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err == nil {
		t.Fatal("expected bad-sig-base64 rejection, got nil")
	}
}

// failingReader always errors on Read, exercising the request-read fail-closed
// path.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, sentinelErr("read failed") }

// TestEnrollAddRejectsRequestReadError proves a request whose reader errors is
// refused (fail closed on I/O failure).
func TestEnrollAddRejectsRequestReadError(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub), Now: fixedNow(1_770_000_500)}
	if err := EnrollAdd(&out, &errOut, failingReader{}, cfg); err == nil {
		t.Fatal("expected request-read-error rejection, got nil")
	}
}

// TestEnrollAddDefaultValidityAndNow proves the default issuance window (one
// year) and the default clock (Now nil => time.Now) paths: with no
// ValiditySeconds and no Now, the emitted binding's window spans ~one year.
func TestEnrollAddDefaultValidityAndNow(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)
	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)

	var out, errOut bytes.Buffer
	// No ValiditySeconds, no Now -> defaults exercised.
	cfg := EnrollAddConfig{RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub)}
	if err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg); err != nil {
		t.Fatalf("EnrollAdd(defaults): %v", err)
	}
	m := decodeEmitted(t, out.Bytes())
	agents, _ := m["agents"].([]any)
	b0, _ := agents[0].(map[string]any)
	vf, _ := b0["valid_from"].(float64)
	vu, _ := b0["valid_until"].(float64)
	if int64(vu)-int64(vf) != 31_536_000 {
		t.Fatalf("default window = %d seconds, want 31536000 (one year)", int64(vu)-int64(vf))
	}
}

// TestEnrollAddReadsRequestFromReader proves the request is read from the
// supplied reader (the stdin seam) when RequestPath is empty.
func TestEnrollAddReadsRequestFromReader(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	agentPub, agentPriv := fixedAgentKey(t, 0x02)
	net := networkOf(rootPub)
	req := signedRequestFor(t, agentPub, agentPriv, "mira", net, 1_700_000_000)

	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64:   base64.StdEncoding.EncodeToString(rootPub),
		ValiditySeconds: testValiditySeconds,
		Now:             fixedNow(1_770_000_500),
	}
	if err := EnrollAdd(&out, &errOut, strings.NewReader(string(req)), cfg); err != nil {
		t.Fatalf("EnrollAdd(reader): %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("expected emitted registry from reader input")
	}
}

// TestEnrollAddRejectsPrePoisonedRegistry proves the pre-emit signability gate
// runs the FULL consumer trust gate (VerifyAndLoad), not just canonicalization:
// a CURRENT registry already internally inconsistent (two LIVE bindings for one
// slug — which checkUniqueness cannot see, as it only checks the NEW binding
// against existing ones) is refused, so enroll-add never emits a document the
// consumer would reject wholesale. Without the VerifyAndLoad gate this passes
// (buildRegistry/SignRegistry skip uniqueness) and silently emits a poisoned
// registry — this test is the regression guard for that.
func TestEnrollAddRejectsPrePoisonedRegistry(t *testing.T) {
	rootPub, _ := fixedAgentKey(t, 0x01)
	ghost1, _ := fixedAgentKey(t, 0x05)
	ghost2, _ := fixedAgentKey(t, 0x06)
	agentPub, agentPriv := fixedAgentKey(t, 0x09)
	net := networkOf(rootPub)

	// Pre-poisoned current registry: TWO LIVE bindings for slug "ghost" (epochs 1
	// and 2) — a >1-live-per-slug violation already present, untouched by the new
	// non-conflicting "newbie" add. checkUniqueness passes (newbie is unique), so
	// only the full consumer gate in serializeSignable catches this.
	poisoned := `{"agents":[` +
		`{"authorized_origin_daemons":["d"],"display_name":"G1","key_epoch":1,` +
		`"pubkey":"` + base64.StdEncoding.EncodeToString(ghost1) + `","slug":"ghost",` +
		`"valid_from":1760000000,"valid_until":1780000000},` +
		`{"authorized_origin_daemons":["d"],"display_name":"G2","key_epoch":2,` +
		`"pubkey":"` + base64.StdEncoding.EncodeToString(ghost2) + `","slug":"ghost",` +
		`"valid_from":1760000000,"valid_until":1780000000}],` +
		`"epoch":7,"valid_from":1760000000,"valid_until":1780000000}`

	req := signedRequestFor(t, agentPub, agentPriv, "newbie", net, 1_700_000_000)
	var out, errOut bytes.Buffer
	cfg := EnrollAddConfig{
		RootPubKeyB64: base64.StdEncoding.EncodeToString(rootPub),
		RegistryInput: []byte(poisoned),
		Now:           fixedNow(1_770_000_500),
	}
	err := EnrollAdd(&out, &errOut, bytes.NewReader(req), cfg)
	if err == nil {
		t.Fatal("expected pre-poisoned-registry rejection via the full consumer gate, got nil")
	}
	if !strings.Contains(err.Error(), errEnrollAddNotSignable) {
		t.Fatalf("expected errEnrollAddNotSignable, got: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("reject must emit nothing, got %q", out.String())
	}
}
