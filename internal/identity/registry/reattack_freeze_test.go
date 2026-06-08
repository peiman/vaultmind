package registry

import (
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
)

// ═══════════════════════════════════════════════════════════════════════════════
// RE-ATTACK: FREEZE/ECLIPSE — adversarial re-verification of the fix in
// 5086a9bfc40d9eb3185cc3414c376fc8d79e828c.
//
// Goal: confirm the ENTIRE class of future-valid_from / stale-registry attacks
// is closed, not just the specific cases that were reported. Test both the
// registry level (VerifyAndLoad) and the binding level (Resolve, VerifyMessage).
// Also confirm that honest, currently-valid registries still load (no
// over-rejection).
// ═══════════════════════════════════════════════════════════════════════════════

// ---------------------------------------------------------------------------
// A. REGISTRY-LEVEL: VerifyAndLoad rejects future valid_from
// ---------------------------------------------------------------------------

// TestReattack_RegistryValidFrom_NowPlus1Second — smallest possible future offset.
// If the fix only checks for "large" offsets, this catches it.
func TestReattack_RegistryValidFrom_NowPlus1Second(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	reg := Registry{
		Epoch:      5,
		ValidFrom:  fixedNow.Unix() + 1, // just 1 second in the future
		ValidUntil: fixedNow.Unix() + 86400,
		Agents:     []AgentBinding{liveBinding(t, "mira", agentPub, 1)},
	}
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	_, _, err = VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	if err == nil {
		t.Fatal("SECURITY: VerifyAndLoad accepted a registry with valid_from = now+1s — " +
			"the 1-second-future case must be rejected (freeze/eclipse)")
	}
	if !strings.Contains(err.Error(), ErrStale) {
		t.Fatalf("expected ErrStale, got: %v", err)
	}
}

// TestReattack_RegistryValidFrom_NowPlus1Year — large future offset.
func TestReattack_RegistryValidFrom_NowPlus1Year(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	reg := Registry{
		Epoch:      5,
		ValidFrom:  fixedNow.Unix() + 86400*365,
		ValidUntil: fixedNow.Unix() + 86400*365*2,
		Agents:     []AgentBinding{liveBinding(t, "mira", agentPub, 1)},
	}
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	_, _, err = VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	if err == nil {
		t.Fatal("SECURITY: VerifyAndLoad accepted a registry with valid_from = now+1yr")
	}
}

// TestReattack_RegistryValidFrom_NanosecondBeforeBoundary — now has 999999999ns
// but now.Unix() truncates to the same second as valid_from. This tests
// the inclusive boundary: valid_from == now.Unix() should PASS.
func TestReattack_RegistryValidFrom_NanosecondBeforeBoundary(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	// Registry valid_from == fixedNow.Unix() exactly
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// now at 0.999999999s past the valid_from second — still in the same unix second
	nowWithNanos := time.Unix(reg.ValidFrom, 999_999_999)
	_, _, err = VerifyAndLoad(rootPub, env, 4, nowWithNanos, time.Hour*24*365)
	if err != nil {
		t.Fatalf("valid_from is inclusive: registry must load when now.Unix() == valid_from, "+
			"even with sub-second offset. Got: %v", err)
	}
}

// TestReattack_RegistryValidFrom_ExactlyAtNow — now.Unix() == reg.ValidFrom.
// This is the honest case and MUST be accepted (no over-rejection).
func TestReattack_RegistryValidFrom_ExactlyAtNow(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	atStart := time.Unix(reg.ValidFrom, 0)
	loaded, _, err := VerifyAndLoad(rootPub, env, 4, atStart, time.Hour*24*365)
	if err != nil {
		t.Fatalf("OVER-REJECTION: VerifyAndLoad rejected a registry at exactly valid_from: %v", err)
	}
	if len(loaded.Agents) != 1 {
		t.Fatal("loaded registry has wrong agent count")
	}
}

// ---------------------------------------------------------------------------
// B. BINDING-LEVEL: Resolve rejects not-yet-active binding valid_from
// ---------------------------------------------------------------------------

// TestReattack_BindingValidFrom_NowPlus1Second_Resolve — binding valid_from
// is 1 second in the future. Resolve must skip it.
func TestReattack_BindingValidFrom_NowPlus1Second_Resolve(t *testing.T) {
	agentPub, _ := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	b.ValidFrom = fixedNow.Unix() + 1
	b.ValidUntil = fixedNow.Unix() + 86400

	reg := freshRegistry(5, b)
	_, err := Resolve(reg, "mira", fixedNow)
	if err == nil {
		t.Fatal("SECURITY: Resolve accepted a binding with valid_from = now+1s — " +
			"the not-yet-active binding must not resolve")
	}
}

// TestReattack_BindingValidFrom_NowPlus1Year_Resolve
func TestReattack_BindingValidFrom_NowPlus1Year_Resolve(t *testing.T) {
	agentPub, _ := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	b.ValidFrom = fixedNow.Unix() + 86400*365
	b.ValidUntil = fixedNow.Unix() + 86400*365*2

	reg := freshRegistry(5, b)
	_, err := Resolve(reg, "mira", fixedNow)
	if err == nil {
		t.Fatal("SECURITY: Resolve accepted a binding with valid_from = now+1yr")
	}
}

// TestReattack_BindingValidFrom_ExactlyAtNow_Resolve — boundary inclusiveness.
// A binding that starts exactly now MUST resolve (no over-rejection).
func TestReattack_BindingValidFrom_ExactlyAtNow_Resolve(t *testing.T) {
	agentPub, _ := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	// b.ValidFrom default is 1_770_000_000; fixedNow is 1_770_000_500.
	// Set ValidFrom to exactly now.
	b.ValidFrom = fixedNow.Unix()
	b.ValidUntil = fixedNow.Unix() + 86400

	reg := freshRegistry(5, b)
	resolved, err := Resolve(reg, "mira", fixedNow)
	if err != nil {
		t.Fatalf("OVER-REJECTION: Resolve rejected a binding at exactly valid_from == now: %v", err)
	}
	if resolved.Slug != "mira" {
		t.Fatalf("resolved wrong slug: %q", resolved.Slug)
	}
}

// TestReattack_BindingValidFrom_NanosecondPrecision_Resolve — now has nanoseconds
// but now.Unix() truncates to the same second as valid_from. Still accepted.
func TestReattack_BindingValidFrom_NanosecondPrecision_Resolve(t *testing.T) {
	agentPub, _ := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	startSecond := fixedNow.Unix()
	b.ValidFrom = startSecond
	b.ValidUntil = startSecond + 86400

	reg := freshRegistry(5, b)
	// now is in the same second but with high nanoseconds
	nowNanos := time.Unix(startSecond, 999_999_999)
	_, err := Resolve(reg, "mira", nowNanos)
	if err != nil {
		t.Fatalf("OVER-REJECTION: Resolve rejected when now.Unix() == valid_from (sub-second): %v", err)
	}
}

// ---------------------------------------------------------------------------
// C. BINDING-LEVEL: VerifyMessage / resolveTuple rejects not-yet-active binding
// ---------------------------------------------------------------------------

// TestReattack_BindingValidFrom_NowPlus1Second_VerifyMessage — E2E through
// the verify path.
func TestReattack_BindingValidFrom_NowPlus1Second_VerifyMessage(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	b.ValidFrom = fixedNow.Unix() + 1
	b.ValidUntil = fixedNow.Unix() + 86400

	reg := freshRegistry(5, b)
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"premature"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)

	_, err := VerifyMessage(reg, "mira", 1, canonical, sig, fixedNow)
	if err == nil {
		t.Fatal("SECURITY: VerifyMessage accepted a message under a binding with valid_from = now+1s")
	}
	if !strings.Contains(err.Error(), ErrNotYetValid) {
		t.Fatalf("expected ErrNotYetValid, got: %v", err)
	}
}

// TestReattack_BindingValidFrom_ExactlyAtNow_VerifyMessage — honest case.
// A binding that starts exactly now MUST verify (no over-rejection).
func TestReattack_BindingValidFrom_ExactlyAtNow_VerifyMessage(t *testing.T) {
	agentPub, agentPriv := genKey(t)
	b := liveBinding(t, "mira", agentPub, 1)
	b.ValidFrom = fixedNow.Unix()
	b.ValidUntil = fixedNow.Unix() + 86400

	reg := freshRegistry(5, b)
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"on time"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)

	ok, err := VerifyMessage(reg, "mira", 1, canonical, sig, fixedNow)
	if err != nil {
		t.Fatalf("OVER-REJECTION: VerifyMessage rejected at exactly valid_from == now: %v", err)
	}
	if !ok {
		t.Fatal("OVER-REJECTION: VerifyMessage returned false for a valid message at exact boundary")
	}
}

// ---------------------------------------------------------------------------
// D. FULL E2E: sign → load → resolve → verify with future binding in an
//    otherwise-valid registry. The registry itself is in-window, but the
//    binding's valid_from is in the future. Resolve and VerifyMessage must
//    both reject the premature binding.
// ---------------------------------------------------------------------------

func TestReattack_FullE2E_FutureBinding_InWindowRegistry(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, agentPriv := genKey(t)

	// Registry is valid NOW, but the binding starts 1 hour from now.
	b := liveBinding(t, "mira", agentPub, 1)
	b.ValidFrom = fixedNow.Unix() + 3600
	b.ValidUntil = fixedNow.Unix() + 86400

	reg := freshRegistry(5, b)
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// Registry loads (it is in-window).
	loaded, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad should accept an in-window registry: %v", err)
	}

	// Resolve must reject the future binding.
	if _, err := Resolve(loaded, "mira", fixedNow); err == nil {
		t.Fatal("SECURITY: Resolve accepted a future-valid_from binding inside a loaded registry")
	}

	// VerifyMessage must also reject it.
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"early"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	if _, err := VerifyMessage(loaded, "mira", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("SECURITY: VerifyMessage accepted a message under a future-valid_from binding")
	}

	// SAME binding at its valid_from MUST work (no over-rejection).
	futureNow := time.Unix(b.ValidFrom, 0)
	resolved, err := Resolve(loaded, "mira", futureNow)
	if err != nil {
		t.Fatalf("OVER-REJECTION: Resolve rejected binding at its valid_from: %v", err)
	}
	if resolved.Slug != "mira" {
		t.Fatalf("resolved wrong slug: %q", resolved.Slug)
	}
	ok, err := VerifyMessage(loaded, "mira", 1, canonical, sig, futureNow)
	if err != nil {
		t.Fatalf("OVER-REJECTION: VerifyMessage rejected at binding valid_from: %v", err)
	}
	if !ok {
		t.Fatal("OVER-REJECTION: VerifyMessage returned false at binding valid_from")
	}
}

// ---------------------------------------------------------------------------
// E. WITHHELD/STALE REGISTRY — an attacker serves a legitimately-signed
//    registry that is past its freshness window. Must still fail closed.
// ---------------------------------------------------------------------------

func TestReattack_WithheldRegistry_StillFailsClosed(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// Consumer's clock is 1 second past valid_until — the registry is stale.
	staleNow := time.Unix(reg.ValidUntil+1, 0)
	if _, _, err := VerifyAndLoad(rootPub, env, 4, staleNow, time.Hour*24*365); err == nil {
		t.Fatal("SECURITY: withheld registry (past valid_until) was accepted")
	}

	// Consumer's clock is within valid_until but staleness exceeds maxStaleness.
	// reg.ValidFrom = 1_770_000_000; set now to valid_from + 25 hours, within
	// valid_until (1_780_000_000), but with maxStaleness = 24h.
	staleByMax := time.Unix(reg.ValidFrom+int64((time.Hour*25).Seconds()), 0)
	if _, _, err := VerifyAndLoad(rootPub, env, 4, staleByMax, time.Hour*24); err == nil {
		t.Fatal("SECURITY: withheld registry (past maxStaleness) was accepted")
	}
}

// ---------------------------------------------------------------------------
// F. HONEST REGISTRY — confirm no over-rejection after the fix.
// ---------------------------------------------------------------------------

func TestReattack_HonestRegistry_StillLoadsAndVerifies(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, agentPriv := genKey(t)

	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	loaded, newHighest, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	if err != nil {
		t.Fatalf("OVER-REJECTION: honest registry rejected after fix: %v", err)
	}
	if newHighest != 5 {
		t.Fatalf("newHighest = %d, want 5", newHighest)
	}
	if len(loaded.Agents) != 1 {
		t.Fatal("wrong agent count")
	}

	resolved, err := Resolve(loaded, "mira", fixedNow)
	if err != nil {
		t.Fatalf("OVER-REJECTION: Resolve rejected honest binding: %v", err)
	}
	if resolved.Slug != "mira" {
		t.Fatalf("resolved wrong slug: %q", resolved.Slug)
	}

	canonical, _ := identity.Canonicalize([]byte(`{"msg":"honest"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	ok, err := VerifyMessage(loaded, "mira", 1, canonical, sig, fixedNow)
	if err != nil {
		t.Fatalf("OVER-REJECTION: VerifyMessage rejected honest message: %v", err)
	}
	if !ok {
		t.Fatal("OVER-REJECTION: VerifyMessage returned false for honest message")
	}
}

// TestReattack_HonestRegistry_MultipleAgents — rotation scenario. One revoked
// binding and one live binding for the same slug. The fix must not break this.
func TestReattack_HonestRegistry_MultipleAgents(t *testing.T) {
	rootPub, rootPriv := genKey(t)
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
	reg := freshRegistry(6, old, fresh)
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	loaded, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	if err != nil {
		t.Fatalf("OVER-REJECTION: rotation registry rejected: %v", err)
	}

	// Live binding (epoch 2) resolves.
	resolved, err := Resolve(loaded, "mira", fixedNow)
	if err != nil {
		t.Fatalf("OVER-REJECTION: live binding in rotation rejected: %v", err)
	}
	if resolved.KeyEpoch != 2 {
		t.Fatalf("resolved wrong epoch: %d", resolved.KeyEpoch)
	}

	// Verify message under the new key.
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"rotated"}`))
	sig, _ := identity.SignCanonical(newPriv, canonical)
	ok, err := VerifyMessage(loaded, "mira", 2, canonical, sig, fixedNow)
	if err != nil {
		t.Fatalf("OVER-REJECTION: VerifyMessage rejected rotated binding: %v", err)
	}
	if !ok {
		t.Fatal("OVER-REJECTION: VerifyMessage returned false for rotated binding")
	}

	// Old revoked epoch must still be rejected.
	if _, err := VerifyMessage(loaded, "mira", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("SECURITY: revoked old binding still verifies after fix")
	}
}

// ---------------------------------------------------------------------------
// G. COMBINED ATTACK: future valid_from + negative staleness bypass attempt.
//    The fix adds `now.Unix() < reg.ValidFrom` BEFORE the staleness check.
//    Verify that this ordering holds even with an extremely generous staleness.
// ---------------------------------------------------------------------------

func TestReattack_FutureValidFrom_WithMaxDurationStaleness(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	reg := Registry{
		Epoch:      5,
		ValidFrom:  fixedNow.Unix() + 1, // 1 second in the future
		ValidUntil: fixedNow.Unix() + 86400*365*100,
		Agents:     []AgentBinding{liveBinding(t, "mira", agentPub, 1)},
	}
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// maxStaleness = maximum possible Duration value.
	// Without the now < valid_from guard, this would pass because:
	//   now - future = negative < maxStaleness = always true
	maxDur := time.Duration(1<<63 - 1) // math.MaxInt64 nanoseconds
	if _, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, maxDur); err == nil {
		t.Fatal("SECURITY: future valid_from bypassed with max duration staleness — " +
			"the now < valid_from guard must fire first")
	}
}

// TestReattack_FutureValidFrom_ExactlyNowPlus1Nanosecond — the time.Time has
// nanosecond precision but valid_from is in seconds. A now that is 1ns past the
// second boundary of valid_from-1 means now.Unix() is still valid_from-1 < valid_from,
// so the registry IS valid. But if now is at the valid_from second + 0ns,
// now.Unix() == valid_from, which should be accepted.
func TestReattack_RegistryValidFrom_SubSecondEdge(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	target := fixedNow.Unix() + 10
	reg := Registry{
		Epoch:      5,
		ValidFrom:  target,
		ValidUntil: target + 86400,
		Agents:     []AgentBinding{liveBinding(t, "mira", agentPub, 1)},
	}
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// 1ns before the valid_from second: now.Unix() = target-1 < target => REJECT
	justBefore := time.Unix(target-1, 999_999_999)
	if _, _, err := VerifyAndLoad(rootPub, env, 4, justBefore, time.Hour*24*365); err == nil {
		t.Fatal("SECURITY: 1ns before valid_from second accepted (now.Unix() < valid_from)")
	}

	// At exactly the valid_from second: now.Unix() = target == target => ACCEPT
	atExact := time.Unix(target, 0)
	if _, _, err := VerifyAndLoad(rootPub, env, 4, atExact, time.Hour*24*365); err != nil {
		t.Fatalf("OVER-REJECTION: exactly at valid_from second rejected: %v", err)
	}

	// 1ns past the valid_from second (still same second): now.Unix() = target => ACCEPT
	justAfterSameSecond := time.Unix(target, 1)
	if _, _, err := VerifyAndLoad(rootPub, env, 4, justAfterSameSecond, time.Hour*24*365); err != nil {
		t.Fatalf("OVER-REJECTION: 1ns past valid_from (same second) rejected: %v", err)
	}
}

// ---------------------------------------------------------------------------
// H. STALE REGISTRY CANNOT HIDE REVOCATION — the original motivation for the
//    freeze/eclipse check. Verify that an attacker who withholds a newer
//    registry (containing a revocation) and serves a stale one cannot succeed.
// ---------------------------------------------------------------------------

func TestReattack_StaleRegistryCannotHideRevocation(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, agentPriv := genKey(t)

	// "Old" registry: agent is live, signed at epoch 5, valid for 1 hour.
	oldReg := Registry{
		Epoch:      5,
		ValidFrom:  fixedNow.Unix() - 7200, // 2 hours ago
		ValidUntil: fixedNow.Unix() - 3600, // expired 1 hour ago
		Agents:     []AgentBinding{liveBinding(t, "mira", agentPub, 1)},
	}
	oldEnv, err := SignRegistry(rootPriv, oldReg)
	if err != nil {
		t.Fatalf("SignRegistry old: %v", err)
	}

	// An attacker serves the old (expired) registry to hide the revocation.
	_, _, err = VerifyAndLoad(rootPub, oldEnv, 4, fixedNow, time.Hour*24*365)
	if err == nil {
		t.Fatal("SECURITY: stale/expired registry was accepted — " +
			"attacker can hide revocations by serving old registries")
	}

	// Even if the attacker's registry is not past valid_until but exceeds
	// maxStaleness, it must still fail.
	looseReg := Registry{
		Epoch:      5,
		ValidFrom:  fixedNow.Unix() - 86400*2, // 2 days ago
		ValidUntil: fixedNow.Unix() + 86400,   // still within valid_until
		Agents:     []AgentBinding{liveBinding(t, "mira", agentPub, 1)},
	}
	looseEnv, err := SignRegistry(rootPriv, looseReg)
	if err != nil {
		t.Fatalf("SignRegistry loose: %v", err)
	}
	_, _, err = VerifyAndLoad(rootPub, looseEnv, 4, fixedNow, time.Hour*24)
	if err == nil {
		t.Fatal("SECURITY: registry past maxStaleness was accepted despite being within valid_until")
	}

	// Meanwhile, the CURRENT registry (with revocation) must be accepted AND
	// the revoked binding must not resolve.
	revokedAt := int(int(fixedNow.Unix()) - 100)
	revokedBinding := liveBinding(t, "mira", agentPub, 1)
	revokedBinding.RevokedAt = &revokedAt
	currentReg := freshRegistry(6, revokedBinding)
	currentEnv, err := SignRegistry(rootPriv, currentReg)
	if err != nil {
		t.Fatalf("SignRegistry current: %v", err)
	}
	loaded, _, err := VerifyAndLoad(rootPub, currentEnv, 4, fixedNow, time.Hour*24*365)
	if err != nil {
		t.Fatalf("current registry should load: %v", err)
	}
	// The revoked binding must not verify messages.
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"try me"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	if _, err := VerifyMessage(loaded, "mira", 1, canonical, sig, fixedNow); err == nil {
		t.Fatal("SECURITY: revoked binding still verifies after loading the current registry")
	}
}
