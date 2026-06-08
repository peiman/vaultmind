package registry

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
)

// genKey returns a fresh ed25519 keypair for tests.
func genKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return pub, priv
}

// fixedNow is a stable reference instant used across the freshness/expiry tests.
var fixedNow = time.Unix(1_770_000_500, 0)

// liveBinding builds a binding that is live at fixedNow with the given pubkey.
func liveBinding(t *testing.T, slug string, pub ed25519.PublicKey, epoch int) AgentBinding {
	t.Helper()
	pk, err := NewPublicKey(pub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	return AgentBinding{
		Slug:                    slug,
		DisplayName:             "Display " + slug,
		PubKey:                  pk,
		KeyEpoch:                epoch,
		ValidFrom:               1_770_000_000,
		ValidUntil:              1_780_000_000,
		AuthorizedOriginDaemons: []string{"daemon-eu-1"},
		RevokedAt:               nil,
	}
}

// freshRegistry builds a registry whose freshness window contains fixedNow.
func freshRegistry(epoch int, agents ...AgentBinding) Registry {
	return Registry{
		Epoch:      epoch,
		ValidFrom:  1_770_000_000,
		ValidUntil: 1_780_000_000,
		Agents:     agents,
	}
}

func TestSignAndVerifyAndLoad_Valid(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, agentPriv := genKey(t)

	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))

	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	loaded, newHighest, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad: %v", err)
	}
	if newHighest != 5 {
		t.Fatalf("newHighestEpoch = %d, want 5", newHighest)
	}
	if len(loaded.Agents) != 1 {
		t.Fatalf("loaded agents = %d, want 1", len(loaded.Agents))
	}

	b, err := Resolve(loaded, "mira", fixedNow)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if b.Slug != "mira" {
		t.Fatalf("resolved slug = %q, want mira", b.Slug)
	}

	// A real signature by the agent over canonical bytes verifies through the registry.
	canonical, err := identity.Canonicalize([]byte(`{"msg":"hello"}`))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	sig, err := identity.SignCanonical(agentPriv, canonical)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	ok, err := VerifyMessage(loaded, "mira", 1, canonical, sig, fixedNow)
	if err != nil {
		t.Fatalf("VerifyMessage: %v", err)
	}
	if !ok {
		t.Fatal("VerifyMessage = false, want true")
	}
}

func TestVerifyAndLoad_AntiRollback(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// epoch == persistedHighest must be rejected.
	if _, _, err := VerifyAndLoad(rootPub, env, 5, fixedNow, time.Hour*24*365); err == nil {
		t.Fatal("expected rollback reject for epoch == persistedHighest")
	}
	// epoch < persistedHighest must be rejected.
	if _, _, err := VerifyAndLoad(rootPub, env, 6, fixedNow, time.Hour*24*365); err == nil {
		t.Fatal("expected rollback reject for epoch < persistedHighest")
	}
}

func TestVerifyAndLoad_FreshnessFailClosed_PastValidUntil(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// now past valid_until => fail closed.
	past := time.Unix(reg.ValidUntil+1, 0)
	if _, _, err := VerifyAndLoad(rootPub, env, 4, past, time.Hour*24*365); err == nil {
		t.Fatal("expected freshness reject for now past valid_until")
	}
}

func TestVerifyAndLoad_FreshnessFailClosed_StaleBeyondMax(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// now - valid_from exceeds maxStaleness => fail closed, must NOT load.
	staleNow := time.Unix(reg.ValidFrom, 0).Add(time.Hour * 48)
	if _, _, err := VerifyAndLoad(rootPub, env, 4, staleNow, time.Hour); err == nil {
		t.Fatal("expected freshness reject for staleness beyond maxStaleness")
	}
}

func TestVerifyAndLoad_RootForgery_DifferentKey(t *testing.T) {
	rootPub, _ := genKey(t)
	_, otherPriv := genKey(t)
	agentPub, _ := genKey(t)

	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	// Sign with a DIFFERENT private key than the pinned root pubkey.
	env, err := SignRegistry(otherPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}
	if _, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365); err == nil {
		t.Fatal("expected root-forgery reject for mismatched signing key")
	}
}

func TestVerifyAndLoad_SmallOrderRootKey(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}
	_ = rootPub
	// Pin an all-zero (small-order) root pubkey: must be rejected by VerifyCanonical.
	smallOrder := make(ed25519.PublicKey, ed25519.PublicKeySize)
	if _, _, err := VerifyAndLoad(smallOrder, env, 4, fixedNow, time.Hour*24*365); err == nil {
		t.Fatal("expected small-order root pubkey reject")
	}
}

func TestVerifyAndLoad_TamperedBody(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}
	// Flip a byte in the canonical registry body; the root_sig must no longer verify.
	tampered := env
	body := make([]byte, len(env.Registry))
	copy(body, env.Registry)
	body[10] ^= 0xFF
	tampered.Registry = body
	if _, _, err := VerifyAndLoad(rootPub, tampered, 4, fixedNow, time.Hour*24*365); err == nil {
		t.Fatal("expected reject for tampered registry body")
	}
}

func TestResolve_RejectsRevoked(t *testing.T) {
	revokedAt := int(1_770_000_100)
	b := liveBinding(t, "mira", mustKey(t), 1)
	b.RevokedAt = &revokedAt
	reg := freshRegistry(5, b)
	if _, err := Resolve(reg, "mira", fixedNow); err == nil {
		t.Fatal("expected Resolve reject for revoked binding")
	}
}

func TestResolve_RejectsExpiredBinding(t *testing.T) {
	b := liveBinding(t, "mira", mustKey(t), 1)
	reg := freshRegistry(5, b)
	// now past the binding's valid_until.
	expiredNow := time.Unix(b.ValidUntil+1, 0)
	if _, err := Resolve(reg, "mira", expiredNow); err == nil {
		t.Fatal("expected Resolve reject for expired binding")
	}
}

func TestResolve_UnknownSlug(t *testing.T) {
	reg := freshRegistry(5, liveBinding(t, "mira", mustKey(t), 1))
	if _, err := Resolve(reg, "nobody", fixedNow); err == nil {
		t.Fatal("expected Resolve reject for unknown slug")
	}
}

func TestVerifyMessage_RevokedBindingRejected(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	revokedAt := int(1_770_000_100)
	pk, err := NewPublicKey(agentPub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	b := AgentBinding{
		Slug: "mira", DisplayName: "M", PubKey: pk, KeyEpoch: 1,
		ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000,
		AuthorizedOriginDaemons: []string{"d"}, RevokedAt: &revokedAt,
	}
	reg := freshRegistry(5, b)
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	if _, err := VerifyMessage(reg, "mira", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("expected VerifyMessage reject for revoked binding")
	}
}

func TestVerifyMessage_KeyEpochMismatchRejected(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 2))
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	// caller claims epoch 1, binding is epoch 2 => default-deny.
	if _, err := VerifyMessage(reg, "mira", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("expected VerifyMessage reject for key_epoch mismatch")
	}
}

func TestVerifyMessage_NoBindingDefaultDeny(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	if _, err := VerifyMessage(reg, "ghost", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("expected VerifyMessage default-deny for unknown slug")
	}
}

func TestVerifyMessage_WrongSignatureNoMatch(t *testing.T) {
	agentPub, _ := genKey(t)
	_, otherPriv := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	// Sign with a key that is NOT the binding's key.
	sig, _ := identity.SignCanonical(otherPriv, canonical)
	ok, err := VerifyMessage(reg, "mira", 1, canonical, sig, fixedNow)
	if err != nil {
		t.Fatalf("VerifyMessage err: %v", err)
	}
	if ok {
		t.Fatal("expected VerifyMessage = false for non-matching signature")
	}
}

func TestRotation_NewTupleWorksOldRevoked(t *testing.T) {
	oldPub, _ := genKey(t)
	newPub, newPriv := genKey(t)
	revokedAt := int(1_770_000_100)

	oldPk, _ := NewPublicKey(oldPub)
	newPk, _ := NewPublicKey(newPub)

	old := AgentBinding{
		Slug: "mira", DisplayName: "M", PubKey: oldPk, KeyEpoch: 1,
		ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000,
		AuthorizedOriginDaemons: []string{"d"}, RevokedAt: &revokedAt,
	}
	fresh := AgentBinding{
		Slug: "mira", DisplayName: "M", PubKey: newPk, KeyEpoch: 2,
		ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000,
		AuthorizedOriginDaemons: []string{"d"}, RevokedAt: nil,
	}
	// New tuple resolves; Resolve returns the live (epoch 2) binding.
	reg := freshRegistry(6, fresh, old)
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(newPriv, canonical)
	ok, err := VerifyMessage(reg, "mira", 2, canonical, sig, fixedNow)
	if err != nil {
		t.Fatalf("VerifyMessage(new): %v", err)
	}
	if !ok {
		t.Fatal("expected new rotated tuple to verify")
	}
	// Old tuple (epoch 1) is revoked => rejected.
	if _, err := VerifyMessage(reg, "mira", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("expected old revoked tuple to be rejected")
	}
}

func TestNewPublicKey_RejectsWrongLength(t *testing.T) {
	if _, err := NewPublicKey([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected NewPublicKey reject for wrong length")
	}
}

func TestNewPublicKey_RejectsSmallOrder(t *testing.T) {
	smallOrder := make([]byte, ed25519.PublicKeySize) // all zero is small-order
	if _, err := NewPublicKey(smallOrder); err == nil {
		t.Fatal("expected NewPublicKey reject for small-order key")
	}
}

func TestNewPublicKey_Bytes(t *testing.T) {
	pub, _ := genKey(t)
	pk, err := NewPublicKey(pub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	if string(pk.Bytes()) != string(pub) {
		t.Fatal("Bytes() did not round-trip the public key")
	}
}

func TestNewPublicKey_BytesReturnsDefensiveCopy(t *testing.T) {
	pub, _ := genKey(t)
	pk, err := NewPublicKey(pub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	// Mutating the returned slice must NOT corrupt the PublicKey's validated
	// internal storage (Bytes returns a copy, not an alias).
	got := pk.Bytes()
	got[0] ^= 0xFF
	if string(pk.Bytes()) != string(pub) {
		t.Fatal("Bytes() returned an alias to internal storage — a caller mutated the validated key")
	}
}

func TestVerifyAndLoad_RejectsDuplicateSlugEpoch(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	pub1, _ := genKey(t)
	pub2, _ := genKey(t)
	// Two bindings with the SAME {slug,key_epoch} tuple must be rejected.
	b1 := liveBinding(t, "mira", pub1, 1)
	b2 := liveBinding(t, "mira", pub2, 1)
	reg := freshRegistry(5, b1, b2)
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}
	if _, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365); err == nil {
		t.Fatal("expected reject for duplicate {slug,key_epoch} tuple")
	}
}

func TestVerifyAndLoad_RejectsMultipleLiveBindingsPerSlug(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	pub1, _ := genKey(t)
	pub2, _ := genKey(t)
	// Two DISTINCT-epoch but both-LIVE bindings for one slug must be rejected
	// (rotation requires the old tuple be revoked, so only one is ever live).
	b1 := liveBinding(t, "mira", pub1, 1)
	b2 := liveBinding(t, "mira", pub2, 2)
	reg := freshRegistry(5, b1, b2)
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}
	if _, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365); err == nil {
		t.Fatal("expected reject for more than one live binding per slug")
	}
}

func TestVerifyAndLoad_AllowsRotationOneLivePlusRevoked(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	oldPub, _ := genKey(t)
	newPub, _ := genKey(t)
	revokedAt := int(1_770_000_100)
	oldPk, _ := NewPublicKey(oldPub)
	newPk, _ := NewPublicKey(newPub)
	// A legitimate rotation (old tuple revoked, new tuple live) MUST still load —
	// the uniqueness invariant does not over-reject valid rotations.
	old := AgentBinding{
		Slug: "mira", DisplayName: "M", PubKey: oldPk, KeyEpoch: 1,
		ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000,
		AuthorizedOriginDaemons: []string{"d"}, RevokedAt: &revokedAt,
	}
	fresh := AgentBinding{
		Slug: "mira", DisplayName: "M", PubKey: newPk, KeyEpoch: 2,
		ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000,
		AuthorizedOriginDaemons: []string{"d"}, RevokedAt: nil,
	}
	reg := freshRegistry(6, old, fresh)
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}
	if _, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365); err != nil {
		t.Fatalf("legitimate rotation must still load: %v", err)
	}
}

func TestNetworkID_DerivedFromPubkey(t *testing.T) {
	pub, _ := genKey(t)
	other, _ := genKey(t)

	id1 := NetworkID(pub)
	id2 := NetworkID(pub)
	id3 := NetworkID(other)

	if id1 != id2 {
		t.Fatal("NetworkID is not deterministic for the same pubkey")
	}
	if id1 == id3 {
		t.Fatal("NetworkID collided for distinct pubkeys")
	}
	if !strings.HasPrefix(id1, GlobalPrefix) {
		t.Fatalf("NetworkID %q missing reserved global prefix %q", id1, GlobalPrefix)
	}
}

func TestSignedEntry_GateRunsAndVerifies(t *testing.T) {
	pub, priv := genKey(t)
	pk, err := NewPublicKey(pub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	raw := []byte(`{"msg":"hi"}`)
	sig, err := identity.SignEntry(priv, raw)
	if err != nil {
		t.Fatalf("SignEntry: %v", err)
	}
	se, err := NewSignedEntry(pk, raw, sig)
	if err != nil {
		t.Fatalf("NewSignedEntry: %v", err)
	}
	if se.Slug() == "" && len(se.Canonical().Bytes()) == 0 {
		t.Fatal("SignedEntry exposes no canonical bytes")
	}
}

func TestSignedEntry_RejectsBadSchema(t *testing.T) {
	pub, priv := genKey(t)
	pk, _ := NewPublicKey(pub)
	// A bare scalar is not a Contract-B object => schema gate must reject.
	raw := []byte(`42`)
	sig, _ := identity.SignCanonical(priv, identity.CanonicalBytesFromTrusted(raw))
	if _, err := NewSignedEntry(pk, raw, sig); err == nil {
		t.Fatal("expected NewSignedEntry reject for non-conformant entry")
	}
}

func TestSignedEntry_RejectsBadSignature(t *testing.T) {
	pub, _ := genKey(t)
	_, otherPriv := genKey(t)
	pk, _ := NewPublicKey(pub)
	raw := []byte(`{"msg":"hi"}`)
	sig, _ := identity.SignEntry(otherPriv, raw)
	if _, err := NewSignedEntry(pk, raw, sig); err == nil {
		t.Fatal("expected NewSignedEntry reject for non-matching signature")
	}
}

func TestSignRegistry_RejectsOutOfRangeKeyEpoch(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	// A binding key_epoch above MaxSafeEpoch is JCS-unsafe and must be refused.
	b := liveBinding(t, "mira", agentPub, MaxSafeEpoch+1)
	reg := freshRegistry(5, b)
	if _, err := SignRegistry(rootPriv, reg); err == nil {
		t.Fatal("expected SignRegistry reject for out-of-range binding key_epoch")
	}
	_ = rootPub
}

func TestResolve_HonorsValidFromBoundaryInclusive(t *testing.T) {
	// Over-rejection guard: a binding is active AT exactly now == valid_from.
	agentPub, _ := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	atStart := time.Unix(b.ValidFrom, 0)
	if _, err := Resolve(freshRegistry(5, b), "mira", atStart); err != nil {
		t.Fatalf("valid_from is inclusive: binding must resolve AT exactly valid_from, got: %v", err)
	}
}

func TestSignRegistry_RejectsBadRootKey(t *testing.T) {
	agentPub, _ := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	if _, err := SignRegistry(nil, reg); err == nil {
		t.Fatal("expected SignRegistry reject for nil root key")
	}
}

// TestCanonicalBindingMatchesDecisionsDoc pins the cross-language canonical form:
// a live binding must serialize to the EXACT JCS bytes ratified in
// docs/mesh/contractb-decisions.md (snake_case keys, base64 pubkey, raw-UTF-8
// display name, no revoked_at for a live tuple). A drift here breaks the Rust
// verifier, so this is a load-bearing regression pin.
func TestCanonicalBindingMatchesDecisionsDoc(t *testing.T) {
	// Use a real (validatable) pubkey; the decisions-doc example string is a
	// synthetic placeholder. The load-bearing assertion is the KEY ORDERING and
	// shape (snake_case, base64 pubkey, raw-UTF-8 display name, no revoked_at for
	// a live tuple), which must match the ratified canonical form.
	pub, _ := genKey(t)
	pk, err := NewPublicKey(pub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString(pub)
	reg := Registry{
		Epoch: 1, ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000,
		Agents: []AgentBinding{{
			Slug: "mira", DisplayName: "Mira ⭐", PubKey: pk, KeyEpoch: 1,
			ValidFrom: 1_770_000_000, ValidUntil: 1_780_000_000,
			AuthorizedOriginDaemons: []string{"daemon-eu-1", "daemon-us-2"},
		}},
	}
	cb, err := canonicalBytes(reg)
	if err != nil {
		t.Fatalf("canonicalBytes: %v", err)
	}
	wantBinding := `{"authorized_origin_daemons":["daemon-eu-1","daemon-us-2"],` +
		`"display_name":"Mira ⭐","key_epoch":1,` +
		`"pubkey":"` + b64 + `",` +
		`"slug":"mira","valid_from":1770000000,"valid_until":1780000000}`
	if !strings.Contains(string(cb.Bytes()), wantBinding) {
		t.Fatalf("canonical binding drift:\n got: %s\nwant substring: %s", cb.Bytes(), wantBinding)
	}
	// A live binding must NOT emit revoked_at.
	if strings.Contains(string(cb.Bytes()), "revoked_at") {
		t.Fatal("live binding leaked a revoked_at field into the canonical form")
	}
}

func TestDecodePubKey_RejectsBadBase64(t *testing.T) {
	if _, err := decodePubKey("not!base64!!"); err == nil {
		t.Fatal("expected decodePubKey reject for invalid base64")
	}
}

func TestVerifyAndLoad_RejectsSmallOrderPubkeyInBody(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	// Hand-build an envelope whose canonical body contains an all-zero (small-order)
	// pubkey: the body verifies under the root, but fromWire must reject the key.
	smallOrderB64 := base64Std(make([]byte, ed25519.PublicKeySize))
	body := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"X",` +
		`"key_epoch":1,"pubkey":"` + smallOrderB64 + `","slug":"mira",` +
		`"valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":5,"valid_from":1770000000,"valid_until":1780000000}`
	canonical, err := identity.Canonicalize([]byte(body))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	sig, err := identity.SignCanonical(rootPriv, canonical)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	env := SignedRegistry{Registry: canonical.Bytes(), RootSig: sig, RootKeyEpoch: 0}
	if _, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365); err == nil {
		t.Fatal("expected reject for small-order pubkey in authenticated body")
	}
}

func TestVerifyAndLoad_RejectsMalformedBody(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	// Authenticated but non-object body: passes root sig, fails JSON unmarshal.
	body := []byte(`"not an object"`)
	canonical := identity.CanonicalBytesFromTrusted(body)
	sig, err := identity.SignCanonical(rootPriv, canonical)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	env := SignedRegistry{Registry: body, RootSig: sig, RootKeyEpoch: 0}
	if _, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365); err == nil {
		t.Fatal("expected reject for malformed authenticated body")
	}
}

// base64Std encodes b with standard padded base64.
func base64Std(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// mustKey returns a fresh public key, failing the test on error.
func mustKey(t *testing.T) ed25519.PublicKey {
	t.Helper()
	pub, _ := genKey(t)
	return pub
}
