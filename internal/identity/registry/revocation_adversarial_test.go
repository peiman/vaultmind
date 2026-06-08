package registry

import (
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
)

// =====================================================================
// ADVERSARIAL REVOCATION + EXPIRY BYPASS TESTS
// =====================================================================

// Attack 1: RevokedAt=0 (zero value via pointer) — does Resolve still reject?
// If the code checks `*b.RevokedAt == 0` it might treat zero as "not revoked".
func TestAdversarial_RevokedAtZero_Resolve(t *testing.T) {
	agentPub, _ := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	zero := int(0)
	b.RevokedAt = &zero
	reg := freshRegistry(5, b)
	if _, err := Resolve(reg, "mira", fixedNow); err == nil {
		t.Fatal("SECURITY: Resolve accepted a binding with RevokedAt=0 — revocation bypass!")
	}
}

// Attack 2: RevokedAt negative (e.g. -1) — could trip up a "if *revokedAt > 0" check.
func TestAdversarial_RevokedAtNegative_Resolve(t *testing.T) {
	agentPub, _ := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	neg := int(-1)
	b.RevokedAt = &neg
	reg := freshRegistry(5, b)
	if _, err := Resolve(reg, "mira", fixedNow); err == nil {
		t.Fatal("SECURITY: Resolve accepted a binding with RevokedAt=-1 — revocation bypass!")
	}
}

// Attack 3: RevokedAt in the far future — "not yet revoked" bypass attempt.
func TestAdversarial_RevokedAtFuture_Resolve(t *testing.T) {
	agentPub, _ := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	future := int(9_999_999_999)
	b.RevokedAt = &future
	reg := freshRegistry(5, b)
	if _, err := Resolve(reg, "mira", fixedNow); err == nil {
		t.Fatal("SECURITY: Resolve accepted a binding with future RevokedAt — revocation bypass!")
	}
}

// Attack 4: RevokedAt=0 + VerifyMessage — can we get a valid verify through the revoked binding?
func TestAdversarial_RevokedAtZero_VerifyMessage(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	zero := int(0)
	b.RevokedAt = &zero
	reg := freshRegistry(5, b)
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	if _, err := VerifyMessage(reg, "mira", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("SECURITY: VerifyMessage accepted a revoked binding (RevokedAt=0) — revocation bypass!")
	}
}

// Attack 5: RevokedAt negative + VerifyMessage
func TestAdversarial_RevokedAtNegative_VerifyMessage(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	neg := int(-1)
	b.RevokedAt = &neg
	reg := freshRegistry(5, b)
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	if _, err := VerifyMessage(reg, "mira", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("SECURITY: VerifyMessage accepted a revoked binding (RevokedAt=-1) — revocation bypass!")
	}
}

// Attack 6: Key rotation — old {pubkey, epoch=1} is revoked, can the OLD private key
// still verify messages under epoch=1?
func TestAdversarial_RotatedKey_OldEpochStillVerifies(t *testing.T) {
	oldPub, oldPriv := genKey(t)
	newPub, _ := genKey(t)
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
	reg := freshRegistry(6, old, fresh)

	canonical, _ := identity.Canonicalize([]byte(`{"msg":"hello from old key"}`))
	sig, _ := identity.SignCanonical(oldPriv, canonical)

	// The old key with epoch 1 must NOT verify
	ok, err := VerifyMessage(reg, "mira", 1, canonical, sig, fixedNow)
	if err == nil && ok {
		t.Fatal("SECURITY: old revoked key still verifies messages — rotation bypass!")
	}
	if err == nil {
		t.Fatal("SECURITY: VerifyMessage returned no error for revoked old tuple (returned false,nil instead of error)")
	}
}

// Attack 7: Binding exactly at valid_until boundary (now == valid_until).
// Boundary convention (documented on VerifyAndLoad/Resolve): valid_until is
// INCLUSIVE — a binding is honored AT exactly now == valid_until and rejected
// one second past it. This test pins that intended, consistent behavior.
func TestAdversarial_BindingExactlyAtValidUntil_Resolve(t *testing.T) {
	agentPub, _ := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	// b.ValidUntil is 1_780_000_000; set now to exactly that.
	exactNow := time.Unix(b.ValidUntil, 0)
	resolved, err := Resolve(freshRegistry(5, b), "mira", exactNow)
	if err != nil {
		t.Fatalf("valid_until is inclusive: binding must resolve AT exactly valid_until, got: %v", err)
	}
	if resolved.Slug != "mira" {
		t.Fatalf("resolved slug = %q, want mira", resolved.Slug)
	}
}

// Attack 8: Binding at valid_until+1 must be rejected
func TestAdversarial_BindingOneSecondPastValidUntil_Resolve(t *testing.T) {
	agentPub, _ := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	pastNow := time.Unix(b.ValidUntil+1, 0)
	if _, err := Resolve(freshRegistry(5, b), "mira", pastNow); err == nil {
		t.Fatal("SECURITY: Resolve accepted a binding 1 second past valid_until — expiry bypass!")
	}
}

// Attack 9: Binding at valid_until+1 must be rejected by VerifyMessage too
func TestAdversarial_ExpiredBinding_VerifyMessage(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	reg := freshRegistry(5, b)
	pastNow := time.Unix(b.ValidUntil+1, 0)
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	if _, err := VerifyMessage(reg, "mira", 1, canonical, sig, pastNow); err == nil {
		t.Fatal("SECURITY: VerifyMessage accepted a binding 1 second past valid_until — expiry bypass!")
	}
}

// Attack 10: Key epoch mismatch — claim epoch 2 when binding is epoch 1
func TestAdversarial_KeyEpochMismatch_HigherClaimed(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	// Claim epoch 2, binding is epoch 1
	if _, err := VerifyMessage(reg, "mira", 2, canonical, sig, fixedNow); err == nil {
		t.Fatal("SECURITY: VerifyMessage accepted a key_epoch mismatch (2 vs 1) — epoch bypass!")
	}
}

// Attack 11: Key epoch mismatch — claim epoch 0 when binding is epoch 1
func TestAdversarial_KeyEpochMismatch_ZeroClaimed(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	if _, err := VerifyMessage(reg, "mira", 0, canonical, sig, fixedNow); err == nil {
		t.Fatal("SECURITY: VerifyMessage accepted epoch 0 for binding epoch 1 — epoch bypass!")
	}
}

// Attack 12: Unknown slug must be default-denied
func TestAdversarial_UnknownSlug_DefaultDeny(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	// slug "ghost" has no binding
	if _, err := VerifyMessage(reg, "ghost", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("SECURITY: VerifyMessage accepted an unknown slug — default-deny violated!")
	}
}

// Attack 13: Empty slug
func TestAdversarial_EmptySlug_DefaultDeny(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	if _, err := VerifyMessage(reg, "", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("SECURITY: VerifyMessage accepted an empty slug — default-deny violated!")
	}
}

// Attack 14: Empty registry (no agents at all) — must default-deny
func TestAdversarial_EmptyRegistry_DefaultDeny(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	_ = agentPub
	reg := freshRegistry(5) // no agents
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	if _, err := VerifyMessage(reg, "mira", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("SECURITY: VerifyMessage accepted against an empty registry — default-deny violated!")
	}
}

// Attack 15: Resolve on a slug where ALL bindings are revoked — should get error, not succeed
func TestAdversarial_AllBindingsRevoked_Resolve(t *testing.T) {
	pub1, _ := genKey(t)
	pub2, _ := genKey(t)
	revokedAt := int(1_770_000_100)

	b1 := liveBinding(t, "mira", pub1, 1)
	b1.RevokedAt = &revokedAt
	b2 := liveBinding(t, "mira", pub2, 2)
	b2.RevokedAt = &revokedAt

	reg := freshRegistry(5, b1, b2)
	if _, err := Resolve(reg, "mira", fixedNow); err == nil {
		t.Fatal("SECURITY: Resolve found a live binding when all are revoked — revocation bypass!")
	}
}

// Attack 16: Resolve on a slug where ALL bindings are expired — should get error
func TestAdversarial_AllBindingsExpired_Resolve(t *testing.T) {
	pub1, _ := genKey(t)
	pub2, _ := genKey(t)

	b1 := liveBinding(t, "mira", pub1, 1)
	b1.ValidUntil = 1_770_000_100 // expired relative to fixedNow (1_770_000_500)
	b2 := liveBinding(t, "mira", pub2, 2)
	b2.ValidUntil = 1_770_000_200 // also expired

	reg := freshRegistry(5, b1, b2)
	if _, err := Resolve(reg, "mira", fixedNow); err == nil {
		t.Fatal("SECURITY: Resolve found a live binding when all are expired — expiry bypass!")
	}
}

// Attack 17: Resolve returns wrong error type when slug exists but is revoked
// This is an information-disclosure concern: callers can't distinguish "revoked"
// from "never existed" via Resolve.
func TestAdversarial_Resolve_RevokedReturnsUnknownSlugError(t *testing.T) {
	agentPub, _ := genKey(t)
	revokedAt := int(1_770_000_100)
	b := liveBinding(t, "mira", agentPub, 1)
	b.RevokedAt = &revokedAt
	reg := freshRegistry(5, b)

	_, err := Resolve(reg, "mira", fixedNow)
	if err == nil {
		t.Fatal("SECURITY: Resolve accepted a revoked binding")
	}
	// Resolve must surface ErrRevoked (not ErrUnknownSlug) so callers can
	// log/alert specifically on revocation attempts.
	if !strings.Contains(err.Error(), ErrRevoked) {
		t.Fatalf("Resolve on a revoked slug must return ErrRevoked, got: %v", err)
	}
}

// Attack 18: VerifyMessage with correct slug+epoch but the binding is BOTH
// revoked AND expired — does the code check both?
func TestAdversarial_RevokedAndExpired_VerifyMessage(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	revokedAt := int(1_770_000_100)
	b := liveBinding(t, "mira", agentPub, 1)
	b.RevokedAt = &revokedAt
	b.ValidUntil = 1_770_000_200 // also expired relative to fixedNow

	reg := freshRegistry(5, b)
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)

	if _, err := VerifyMessage(reg, "mira", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("SECURITY: VerifyMessage accepted a both-revoked-and-expired binding!")
	}
}

// Attack 19: Binding with ValidUntil=0 (epoch zero) — if now.Unix() > 0, should be expired.
func TestAdversarial_BindingValidUntilZero(t *testing.T) {
	agentPub, _ := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	b.ValidUntil = 0 // far in the past
	reg := freshRegistry(5, b)
	if _, err := Resolve(reg, "mira", fixedNow); err == nil {
		t.Fatal("SECURITY: Resolve accepted a binding with ValidUntil=0 — expiry bypass!")
	}
}

// Attack 20: RevokedAt set to math.MaxInt — extreme value
func TestAdversarial_RevokedAtMaxInt_Resolve(t *testing.T) {
	agentPub, _ := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	maxInt := int(1<<63 - 1)
	b.RevokedAt = &maxInt
	reg := freshRegistry(5, b)
	if _, err := Resolve(reg, "mira", fixedNow); err == nil {
		t.Fatal("SECURITY: Resolve accepted a binding with RevokedAt=MaxInt — revocation bypass!")
	}
}

// Attack 21: Multiple bindings for same slug, one revoked one live — ensure
// the live one is returned, not the revoked one.
func TestAdversarial_MultipleSameSlug_LiveAfterRevoked(t *testing.T) {
	pub1, _ := genKey(t)
	pub2, _ := genKey(t)
	revokedAt := int(1_770_000_100)

	b1 := liveBinding(t, "mira", pub1, 1)
	b1.RevokedAt = &revokedAt
	b2 := liveBinding(t, "mira", pub2, 2)

	// Revoked first, live second
	reg := freshRegistry(5, b1, b2)
	resolved, err := Resolve(reg, "mira", fixedNow)
	if err != nil {
		t.Fatalf("Resolve should find the live binding: %v", err)
	}
	if resolved.KeyEpoch != 2 {
		t.Fatalf("Resolve returned epoch %d, want 2 (the live binding)", resolved.KeyEpoch)
	}
}

// Attack 22: Multiple bindings for same slug, live first then revoked — order test
func TestAdversarial_MultipleSameSlug_LiveBeforeRevoked(t *testing.T) {
	pub1, _ := genKey(t)
	pub2, _ := genKey(t)
	revokedAt := int(1_770_000_100)

	b1 := liveBinding(t, "mira", pub1, 1) // live
	b2 := liveBinding(t, "mira", pub2, 2)
	b2.RevokedAt = &revokedAt // revoked

	// Live first, revoked second
	reg := freshRegistry(5, b1, b2)
	resolved, err := Resolve(reg, "mira", fixedNow)
	if err != nil {
		t.Fatalf("Resolve should find the live binding: %v", err)
	}
	if resolved.KeyEpoch != 1 {
		t.Fatalf("Resolve returned epoch %d, want 1 (the first live binding)", resolved.KeyEpoch)
	}
}

// Attack 23: resolveTuple — same slug, multiple epochs, the matching epoch is revoked
// but a different epoch is live. The revoked epoch should still be rejected.
func TestAdversarial_ResolveTuple_MatchingEpochRevoked_OtherLive(t *testing.T) {
	pub1, priv1 := genKey(t)
	pub2, _ := genKey(t)
	revokedAt := int(1_770_000_100)

	b1 := liveBinding(t, "mira", pub1, 1)
	b1.RevokedAt = &revokedAt             // epoch 1 revoked
	b2 := liveBinding(t, "mira", pub2, 2) // epoch 2 live

	reg := freshRegistry(5, b1, b2)
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(priv1, canonical)

	// Try to verify with the revoked epoch 1
	ok, err := VerifyMessage(reg, "mira", 1, canonical, sig, fixedNow)
	if err == nil && ok {
		t.Fatal("SECURITY: VerifyMessage accepted a revoked epoch even though another epoch is live — revocation bypass!")
	}
	if err == nil {
		t.Fatal("SECURITY: VerifyMessage returned no error for revoked epoch (false,nil instead of error)")
	}
}

// Attack 24: ValidFrom in the future — binding not yet active.
// NOTE: Resolve does NOT check ValidFrom! Only ValidUntil and RevokedAt.
func TestAdversarial_BindingValidFromInFuture_Resolve(t *testing.T) {
	agentPub, _ := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	b.ValidFrom = fixedNow.Unix() + 86400 // 1 day in the future
	b.ValidUntil = fixedNow.Unix() + 86400*2

	reg := freshRegistry(5, b)
	if _, err := Resolve(reg, "mira", fixedNow); err == nil {
		t.Fatal("SECURITY: Resolve accepted a binding whose ValidFrom is in the future — " +
			"a not-yet-active binding must not resolve (premature identity use)")
	}
}

// Attack 25: ValidFrom in the future + VerifyMessage — same check via verify path
func TestAdversarial_BindingValidFromInFuture_VerifyMessage(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	b.ValidFrom = fixedNow.Unix() + 86400
	b.ValidUntil = fixedNow.Unix() + 86400*2

	reg := freshRegistry(5, b)
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)

	if _, err := VerifyMessage(reg, "mira", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("SECURITY: VerifyMessage accepted a message under a not-yet-active binding " +
			"(future ValidFrom) — premature identity use")
	}
}
