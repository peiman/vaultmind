package registry

import (
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
)

// Attack 26: Registry-level ValidFrom in the future — VerifyAndLoad should reject
// a registry that hasn't started its validity window yet.
func TestAdversarial_RegistryValidFromFuture_VerifyAndLoad(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	// Registry valid from 1 day in the future
	futureStart := fixedNow.Unix() + 86400
	futureEnd := fixedNow.Unix() + 86400*2
	reg := Registry{
		Epoch:      5,
		ValidFrom:  futureStart,
		ValidUntil: futureEnd,
		Agents:     []AgentBinding{liveBinding(t, "mira", agentPub, 1)},
	}

	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// VerifyAndLoad at fixedNow (before the registry's ValidFrom) MUST reject:
	// a not-yet-valid registry fails closed even with a large maxStaleness.
	if _, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365); err == nil {
		t.Fatal("SECURITY: VerifyAndLoad accepted a registry whose ValidFrom is in the future — " +
			"a pre-signed future registry must not load early (perpetual-freshness bypass)")
	}
}

// Attack 27: Binding ValidFrom in the future, verify message through it
// A binding not yet active should not be used for signature verification.
func TestAdversarial_BindingNotYetActive_FullE2E(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, agentPriv := genKey(t)

	b := liveBinding(t, "mira", agentPub, 1)
	b.ValidFrom = fixedNow.Unix() + 86400
	b.ValidUntil = fixedNow.Unix() + 86400*2

	reg := freshRegistry(5, b)
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// The registry itself is in-window here (freshRegistry), so it LOADS; the
	// not-yet-active BINDING must then refuse to resolve/verify a message.
	loaded, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad should load an in-window registry: %v", err)
	}

	canonical, _ := identity.Canonicalize([]byte(`{"msg":"premature"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)

	// A binding whose ValidFrom is in the future must NOT verify a message, even
	// with an authentic signature by that agent's key.
	if _, err := VerifyMessage(loaded, "mira", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("SECURITY: a not-yet-active binding (future ValidFrom) verified a message — " +
			"premature identity use")
	}
}

// Attack 28: Stale maxStaleness with future ValidFrom — the staleness check
// `now.Sub(time.Unix(reg.ValidFrom, 0))` would be NEGATIVE (now < ValidFrom),
// which means the Duration is negative, which is < maxStaleness. So staleness
// check always passes for future registries!
func TestAdversarial_StalenessNegativeDuration(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	// Registry starts 1 hour in the future
	reg := Registry{
		Epoch:      5,
		ValidFrom:  fixedNow.Unix() + 3600,
		ValidUntil: fixedNow.Unix() + 86400,
		Agents:     []AgentBinding{liveBinding(t, "mira", agentPub, 1)},
	}

	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// Very tight maxStaleness (1 minute): the now < valid_from guard must reject
	// BEFORE the staleness check, so a negative duration can never bypass it.
	if _, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Minute); err == nil {
		t.Fatal("SECURITY: VerifyAndLoad accepted a future-ValidFrom registry — " +
			"the negative staleness duration must not bypass the staleness check")
	}
}

// Attack 29: Resolve does not return ErrRevoked — it returns ErrUnknownSlug.
// This means callers cannot distinguish between "slug never existed" and
// "slug was specifically revoked". Can an attacker use this to avoid detection?
func TestAdversarial_Resolve_ErrorMessage_Differentiation(t *testing.T) {
	agentPub, _ := genKey(t)
	revokedAt := int(1_770_000_100)
	b := liveBinding(t, "mira", agentPub, 1)
	b.RevokedAt = &revokedAt
	reg := freshRegistry(5, b)

	_, errRevoked := Resolve(reg, "mira", fixedNow)
	_, errUnknown := Resolve(reg, "nonexistent", fixedNow)

	if errRevoked == nil || errUnknown == nil {
		t.Fatal("expected both to error")
	}

	// The revoked slug must surface ErrRevoked (distinguishable), the unknown
	// slug ErrUnknownSlug — security monitoring can now tell revocation probing
	// from typos.
	if !strings.Contains(errRevoked.Error(), ErrRevoked) {
		t.Fatalf("Resolve on a revoked slug must return ErrRevoked, got: %v", errRevoked)
	}
	if !strings.Contains(errUnknown.Error(), ErrUnknownSlug) {
		t.Fatalf("Resolve on an unknown slug must return ErrUnknownSlug, got: %v", errUnknown)
	}
	if errRevoked.Error() == errUnknown.Error() {
		t.Fatal("Resolve must distinguish a revoked slug from an unknown slug")
	}
}

// Attack 30: Binding with KeyEpoch=0 — zero epoch should still match if claimed
func TestAdversarial_BindingKeyEpochZero_Accepted(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	b := liveBinding(t, "mira", agentPub, 0) // key epoch 0
	reg := freshRegistry(5, b)
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"x"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	ok, err := VerifyMessage(reg, "mira", 0, canonical, sig, fixedNow)
	if err != nil {
		t.Fatalf("VerifyMessage: %v", err)
	}
	if !ok {
		t.Fatal("VerifyMessage should accept epoch 0 when binding is epoch 0")
	}
	// Now try with epoch 1 — should be rejected
	if _, err := VerifyMessage(reg, "mira", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("SECURITY: VerifyMessage accepted epoch 1 for binding epoch 0 — epoch bypass!")
	}
}

// Attack 31: Negative KeyEpoch in a binding — the trust gate must reject a
// registry carrying an out-of-range (negative) binding key_epoch, both at sign
// time and at load time.
func TestAdversarial_NegativeKeyEpoch(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	b := liveBinding(t, "mira", agentPub, -1) // negative key epoch
	reg := freshRegistry(5, b)

	// SignRegistry must refuse to mint a negative binding key_epoch.
	if _, err := SignRegistry(rootPriv, reg); err == nil {
		t.Fatal("SECURITY: SignRegistry accepted a binding with negative key_epoch — epoch floor bypass")
	}

	// And even if an attacker hand-signs such a body, VerifyAndLoad must reject
	// it on the load path. Build a body whose binding key_epoch is -1, sign it
	// with the real root key, and confirm the load gate rejects it.
	body := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"X",` +
		`"key_epoch":-1,"pubkey":"` + base64Std(agentPub) + `","slug":"mira",` +
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
		t.Fatal("SECURITY: VerifyAndLoad accepted a hand-signed binding with negative key_epoch")
	}
}
