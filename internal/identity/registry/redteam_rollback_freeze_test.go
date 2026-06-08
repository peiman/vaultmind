package registry

import (
	"math"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
)

// ═══════════════════════════════════════════════════════════════════════════
// ATTACK VECTOR: ROLLBACK — Can a consumer be made to accept an OLD registry?
// ═══════════════════════════════════════════════════════════════════════════

// TestRedteam_Rollback_EpochEqualPersistedMustReject verifies that
// epoch == persistedHighestEpoch is REJECTED (strictly greater required).
func TestRedteam_Rollback_EpochEqualPersistedMustReject(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// persistedHighestEpoch == 5, registry epoch == 5 => MUST reject
	_, newHighest, err := VerifyAndLoad(rootPub, env, 5, fixedNow, time.Hour*24*365)
	if err == nil {
		t.Fatal("VULNERABILITY: epoch == persistedHighestEpoch was ACCEPTED — allows replaying the same registry")
	}
	// newHighest must not advance
	if newHighest != 5 {
		t.Fatalf("newHighest advanced to %d despite rejection", newHighest)
	}
}

// TestRedteam_Rollback_EpochBelowPersistedMustReject verifies that
// epoch < persistedHighestEpoch is REJECTED.
func TestRedteam_Rollback_EpochBelowPersistedMustReject(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(3, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// persistedHighestEpoch == 10, registry epoch == 3 => MUST reject
	_, _, err = VerifyAndLoad(rootPub, env, 10, fixedNow, time.Hour*24*365)
	if err == nil {
		t.Fatal("VULNERABILITY: epoch < persistedHighestEpoch was ACCEPTED — allows rollback to old registry")
	}
}

// TestRedteam_Rollback_EpochZeroMustReject asserts epoch 0 is rejected. The
// epoch FLOOR of 1 makes SignRegistry refuse to mint epoch 0 outright (the
// very first load must have epoch >= 1); a hand-signed epoch-0 body is likewise
// rejected by VerifyAndLoad.
func TestRedteam_Rollback_EpochZeroMustReject(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(0, liveBinding(t, "mira", agentPub, 1))
	if _, err := SignRegistry(rootPriv, reg); err == nil {
		t.Fatal("SECURITY: SignRegistry minted epoch 0 — epoch floor bypass")
	}

	// Hand-sign an epoch-0 body and confirm the load gate rejects it too.
	body := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"X",` +
		`"key_epoch":1,"pubkey":"` + base64Std(agentPub) + `","slug":"mira",` +
		`"valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":0,"valid_from":1770000000,"valid_until":1780000000}`
	canonical, err := identity.Canonicalize([]byte(body))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	sig, err := identity.SignCanonical(rootPriv, canonical)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	env := SignedRegistry{Registry: canonical.Bytes(), RootSig: sig, RootKeyEpoch: 0}
	if _, _, err := VerifyAndLoad(rootPub, env, 0, fixedNow, time.Hour*24*365); err == nil {
		t.Fatal("VULNERABILITY: epoch 0 with persistedHighest 0 was ACCEPTED — the very first load must have epoch > 0")
	}
}

// TestRedteam_Rollback_EpochOneMustAcceptWhenPersistedZero tests that epoch 1
// with persisted 0 is the legitimate first-use case.
func TestRedteam_Rollback_EpochOneMustAcceptWhenPersistedZero(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(1, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	_, newHighest, err := VerifyAndLoad(rootPub, env, 0, fixedNow, time.Hour*24*365)
	if err != nil {
		t.Fatalf("epoch=1 with persisted=0 should be accepted: %v", err)
	}
	if newHighest != 1 {
		t.Fatalf("newHighest = %d, want 1", newHighest)
	}
}

// TestRedteam_Rollback_NegativeEpochMustReject asserts negative epochs cannot
// reset the monotonic counter: the epoch floor of 1 makes SignRegistry refuse
// to mint a negative epoch, and a hand-signed negative-epoch body is rejected
// by VerifyAndLoad.
func TestRedteam_Rollback_NegativeEpochMustReject(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(-1, liveBinding(t, "mira", agentPub, 1))
	if _, err := SignRegistry(rootPriv, reg); err == nil {
		t.Fatal("SECURITY: SignRegistry minted a negative epoch — epoch floor bypass")
	}

	// Hand-sign a negative-epoch body and confirm the load gate rejects it.
	body := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"X",` +
		`"key_epoch":1,"pubkey":"` + base64Std(agentPub) + `","slug":"mira",` +
		`"valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":-1,"valid_from":1770000000,"valid_until":1780000000}`
	canonical, err := identity.Canonicalize([]byte(body))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	sig, err := identity.SignCanonical(rootPriv, canonical)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	env := SignedRegistry{Registry: canonical.Bytes(), RootSig: sig, RootKeyEpoch: 0}
	if _, _, err := VerifyAndLoad(rootPub, env, 0, fixedNow, time.Hour*24*365); err == nil {
		t.Fatal("VULNERABILITY: negative epoch was ACCEPTED with persisted=0")
	}
}

// TestRedteam_Rollback_NegativeEpochPersistedNegative asserts the epoch FLOOR:
// a negative epoch must be rejected even when it is strictly greater than a more
// negative persisted value (-1 > -2). The [1, MaxSafeEpoch] range closes the
// "reset the monotonic counter with a negative epoch" angle. SignRegistry must
// refuse to mint it too.
func TestRedteam_Rollback_NegativeEpochPersistedNegative(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(-1, liveBinding(t, "mira", agentPub, 1))

	if _, err := SignRegistry(rootPriv, reg); err == nil {
		t.Fatal("SECURITY: SignRegistry minted a negative epoch — epoch floor bypass")
	}

	// Hand-sign a body with epoch -1 and confirm the load gate rejects it on the
	// epoch-range floor regardless of the (more-negative) persisted value.
	body := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"X",` +
		`"key_epoch":1,"pubkey":"` + base64Std(agentPub) + `","slug":"mira",` +
		`"valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":-1,"valid_from":1770000000,"valid_until":1780000000}`
	canonical, err := identity.Canonicalize([]byte(body))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	sig, err := identity.SignCanonical(rootPriv, canonical)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	env := SignedRegistry{Registry: canonical.Bytes(), RootSig: sig, RootKeyEpoch: 0}
	if _, _, err := VerifyAndLoad(rootPub, env, -2, fixedNow, time.Hour*24*365); err == nil {
		t.Fatal("SECURITY: VerifyAndLoad accepted a negative epoch (no floor check)")
	}
}

// TestRedteam_Rollback_IntOverflowMaxIntEpoch asserts the epoch CEILING closes
// the int-overflow angle: a MaxInt epoch (which JCS would render in scientific
// notation, overflowing the int unmarshal and/or rounding) is REJECTED at sign
// time, so the monotonic counter can never be driven to MaxInt where MaxInt+1
// would wrap.
func TestRedteam_Rollback_IntOverflowMaxIntEpoch(t *testing.T) {
	_, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	reg := freshRegistry(math.MaxInt, liveBinding(t, "mira", agentPub, 1))
	if _, err := SignRegistry(rootPriv, reg); err == nil {
		t.Fatal("SECURITY: SignRegistry minted a MaxInt epoch — int-overflow / JCS-rounding vector")
	}
}

// TestRedteam_Rollback_EpochOverflowViaJSON asserts that a hand-signed body whose
// epoch is MaxInt64 is rejected on the load path. JCS renders MaxInt64 in
// scientific notation (9223372036854776000), which exceeds int64 and fails the
// JSON unmarshal — a DoS the epoch-range gate turns into a clean reject.
func TestRedteam_Rollback_EpochOverflowViaJSON(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	// SignRegistry must refuse MaxInt outright.
	reg := freshRegistry(math.MaxInt, liveBinding(t, "mira", agentPub, 1))
	if _, err := SignRegistry(rootPriv, reg); err == nil {
		t.Fatal("SECURITY: SignRegistry minted a MaxInt epoch")
	}

	// A hand-signed MaxInt64 body must also be rejected by VerifyAndLoad.
	body := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"X",` +
		`"key_epoch":1,"pubkey":"` + base64Std(agentPub) + `","slug":"mira",` +
		`"valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":9223372036854775807,"valid_from":1770000000,"valid_until":1780000000}`
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
		t.Fatal("SECURITY: VerifyAndLoad accepted a hand-signed MaxInt64 epoch body")
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// ATTACK VECTOR: FREEZE / ECLIPSE — Can a stale registry be honored?
// ═══════════════════════════════════════════════════════════════════════════

// TestRedteam_Freeze_ExactlyAtValidUntilBoundary tests the off-by-one at
// valid_until. The check is `now.Unix() > reg.ValidUntil`. This means
// now.Unix() == ValidUntil is ACCEPTED. Is this correct?
func TestRedteam_Freeze_ExactlyAtValidUntilBoundary(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// Boundary convention (documented on VerifyAndLoad): valid_until is INCLUSIVE
	// — the registry is honored AT exactly now == valid_until and rejected one
	// second past it. Pin that intended, consistent behavior.
	exactlyAt := time.Unix(reg.ValidUntil, 0)
	if _, _, err := VerifyAndLoad(rootPub, env, 4, exactlyAt, time.Hour*24*365); err != nil {
		t.Fatalf("valid_until is inclusive: registry must load AT exactly valid_until, got: %v", err)
	}
}

// TestRedteam_Freeze_OneSecondPastValidUntilMustReject confirms the registry
// is rejected 1 second after valid_until.
func TestRedteam_Freeze_OneSecondPastValidUntilMustReject(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	oneAfter := time.Unix(reg.ValidUntil+1, 0)
	_, _, err = VerifyAndLoad(rootPub, env, 4, oneAfter, time.Hour*24*365)
	if err == nil {
		t.Fatal("VULNERABILITY: registry accepted 1 second past valid_until")
	}
}

// TestRedteam_Freeze_ExactlyAtMaxStalenessBoundary tests the off-by-one at
// maxStaleness. The check is `now.Sub(time.Unix(reg.ValidFrom, 0)) > maxStaleness`.
// This means exactly AT maxStaleness it PASSES. Is this the intended behavior?
func TestRedteam_Freeze_ExactlyAtMaxStalenessBoundary(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	maxStale := time.Hour * 24 // 24 hours
	// Boundary convention (documented on VerifyAndLoad): maxStaleness is
	// INCLUSIVE — staleness == exactly maxStaleness is honored, one tick past is
	// rejected. Pin that intended, consistent behavior.
	exactlyStale := time.Unix(reg.ValidFrom, 0).Add(maxStale)
	if _, _, err := VerifyAndLoad(rootPub, env, 4, exactlyStale, maxStale); err != nil {
		t.Fatalf("maxStaleness is inclusive: registry must load AT exactly maxStaleness, got: %v", err)
	}
}

// TestRedteam_Freeze_OneNanosecondPastMaxStalenessMustReject tests that
// exceeding maxStaleness by even a nanosecond triggers rejection.
func TestRedteam_Freeze_OneNanosecondPastMaxStalenessMustReject(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	maxStale := time.Hour * 24
	// One nanosecond past: now.Sub(valid_from) = maxStale + 1ns > maxStale => reject
	pastStale := time.Unix(reg.ValidFrom, 0).Add(maxStale + time.Nanosecond)
	_, _, err = VerifyAndLoad(rootPub, env, 4, pastStale, maxStale)
	if err == nil {
		t.Fatal("VULNERABILITY: registry accepted past maxStaleness")
	}
}

// TestRedteam_Freeze_ValidUntilZeroDefaultsToNeverExpires tests whether a
// valid_until of 0 creates a "never expires" hole. If valid_until == 0, then
// now.Unix() > 0 is almost always true (since Unix epoch 0 is 1970), so it
// SHOULD fail. But let's verify.
func TestRedteam_Freeze_ValidUntilZeroDefaultsToNeverExpires(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	reg := Registry{
		Epoch:      5,
		ValidFrom:  fixedNow.Unix() - 100, // 100 seconds ago
		ValidUntil: 0,                     // ZERO — does this mean "never" or "already expired"?
		Agents:     []AgentBinding{liveBinding(t, "mira", agentPub, 1)},
	}
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	_, _, err = VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	if err == nil {
		t.Fatal("VULNERABILITY: valid_until=0 was ACCEPTED — this means zero defaults to 'never expires', creating a freeze hole")
	}
	t.Logf("CORRECT: valid_until=0 correctly treated as already-expired (now > 0)")
}

// TestRedteam_Freeze_ValidFromZeroMaxStalenessHuge tests whether valid_from=0
// with a large maxStaleness allows an infinitely old registry.
func TestRedteam_Freeze_ValidFromZeroMaxStalenessHuge(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	// valid_from=0 means the registry was "created" at Unix epoch
	reg := Registry{
		Epoch:      5,
		ValidFrom:  0,
		ValidUntil: fixedNow.Unix() + 86400, // 1 day in future
		Agents:     []AgentBinding{liveBinding(t, "mira", agentPub, 1)},
	}
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// Freshness is bounded by the caller's maxStaleness, which is correct: with a
	// 100-year maxStaleness a valid_from=0 (now - 0 < 100y) is, by definition,
	// within the caller's freshness tolerance and is accepted. The defense
	// against an absurdly old valid_from is a sane maxStaleness (caller policy),
	// not a hardcoded floor. Pin this documented behavior.
	hugeMax := time.Hour * 24 * 365 * 100
	if _, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, hugeMax); err != nil {
		t.Fatalf("valid_from=0 within a 100-year maxStaleness must load (caller-bounded freshness), got: %v", err)
	}
	// And a TIGHT maxStaleness must reject the same registry, proving the bound
	// is what gates it.
	if _, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour); err == nil {
		t.Fatal("valid_from=0 must be rejected under a 1-hour maxStaleness")
	}
}

// TestRedteam_Freeze_MissngValidUntilInWire tests whether a missing valid_until
// field in the JSON (which defaults to Go zero value 0) creates a freeze hole.
// This is tested indirectly: the wire format has valid_until as int64, so a
// missing field defaults to 0, which fails the freshness check.
func TestRedteam_Freeze_MissingValidUntilInWire(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	// Hand-craft a registry JSON WITHOUT valid_until at the registry level
	agentPub, _ := genKey(t)
	_ = agentPub
	// The wireRegistry has valid_until; if the field is missing from JSON,
	// Go's json.Unmarshal defaults it to 0. Let's verify that 0 fails.
	reg := Registry{
		Epoch:      5,
		ValidFrom:  fixedNow.Unix() - 100,
		ValidUntil: 0, // simulates missing/defaulted field
		Agents:     []AgentBinding{liveBinding(t, "mira", agentPub, 1)},
	}
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	_, _, err = VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	if err == nil {
		t.Fatal("VULNERABILITY: missing/zero valid_until accepted — creates a 'never expires' freeze hole")
	}
}

// TestRedteam_Freeze_NanosecondPrecisionLoss tests whether the Unix() truncation
// to seconds creates a window where sub-second timing matters. valid_until is in
// unix seconds (int64), but now is a time.Time with nanosecond precision.
// The comparison is now.Unix() > reg.ValidUntil — now.Unix() truncates.
func TestRedteam_Freeze_NanosecondPrecisionLoss(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// now is valid_until + 0.999999999 seconds. now.Unix() truncates to
	// valid_until, so the comparison is at the inclusive boundary and the
	// registry loads. This sub-second window is inherent to unix-second
	// precision (valid_until is int64 seconds) and is documented, not a bug.
	almostPast := time.Unix(reg.ValidUntil, 999_999_999)
	if _, _, err := VerifyAndLoad(rootPub, env, 4, almostPast, time.Hour*24*365); err != nil {
		t.Fatalf("unix-second truncation: registry must load within the same second as valid_until, got: %v", err)
	}
}

// TestRedteam_Freeze_ClockSkew_FutureValidFrom tests whether a consumer whose
// clock is behind can be tricked by a valid_from set in the future (from the
// consumer's perspective). The staleness check is now - valid_from > maxStaleness.
// If valid_from is in the FUTURE, now - valid_from is NEGATIVE, which is < maxStaleness,
// so the check PASSES. Combined with a far-future valid_until, this creates a registry
// that appears perpetually fresh.
func TestRedteam_Freeze_ClockSkew_FutureValidFrom(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	// valid_from is 1 hour in the future from the consumer's perspective
	reg := Registry{
		Epoch:      5,
		ValidFrom:  fixedNow.Unix() + 3600,             // 1 hour in future
		ValidUntil: fixedNow.Unix() + 3600 + 86400*365, // 1 year in future
		Agents:     []AgentBinding{liveBinding(t, "mira", agentPub, 1)},
	}
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// now - valid_from is NEGATIVE (about -1 hour). The now < valid_from guard
	// must reject this BEFORE the negative duration can satisfy maxStaleness.
	if _, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24); err == nil {
		t.Fatal("SECURITY: registry with a future valid_from accepted — " +
			"the now < valid_from gate must reject it (perpetual-freshness bypass)")
	}
}

// TestRedteam_Freeze_BindingValidUntilZero tests whether a binding with
// valid_until=0 creates a "never expires" hole at the Resolve level.
func TestRedteam_Freeze_BindingValidUntilZero(t *testing.T) {
	agentPub, _ := genKey(t)
	pk, err := NewPublicKey(agentPub)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	b := AgentBinding{
		Slug: "mira", DisplayName: "M", PubKey: pk, KeyEpoch: 1,
		ValidFrom:               1_770_000_000,
		ValidUntil:              0, // ZERO valid_until on the binding
		AuthorizedOriginDaemons: []string{"d"},
	}
	reg := freshRegistry(5, b)

	_, err = Resolve(reg, "mira", fixedNow)
	if err == nil {
		t.Fatal("VULNERABILITY: binding with valid_until=0 resolved successfully — " +
			"a zero valid_until should mean 'already expired', not 'never expires'")
	}
	t.Logf("CORRECT: binding with valid_until=0 rejected by Resolve: %v", err)
}

// TestRedteam_Freeze_MaxStalenessZero tests whether maxStaleness=0 creates
// an impossible-to-satisfy condition or a bypass.
func TestRedteam_Freeze_MaxStalenessZero(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// maxStaleness=0 means: now - valid_from must be <= 0. Since now is always
	// after valid_from (in normal use), this should always reject.
	_, _, err = VerifyAndLoad(rootPub, env, 4, fixedNow, 0)
	if err == nil {
		t.Fatal("VULNERABILITY: maxStaleness=0 did not reject — all registries should be 'too stale'")
	}
}

// TestRedteam_Freeze_NegativeMaxStaleness tests whether negative maxStaleness
// creates a bypass. A negative duration means now - valid_from > negative is
// almost always true (unless now is before valid_from).
func TestRedteam_Freeze_NegativeMaxStaleness(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// Negative maxStaleness: now - valid_from (positive) > negative => ALWAYS true => ALWAYS reject.
	// This is actually SAFE (always rejects), but a caller mistake.
	_, _, err = VerifyAndLoad(rootPub, env, 4, fixedNow, -time.Hour)
	if err == nil {
		t.Fatal("VULNERABILITY: negative maxStaleness allowed registry through")
	}
}

// TestRedteam_Freeze_ConsumerBypassesCheckOrder tests that the check order
// matters: root sig BEFORE anti-rollback BEFORE freshness. If a consumer
// could somehow skip the freshness check (e.g., by failing on signature
// verification first), the registry is still rejected. This tests that
// ALL checks run in order.
func TestRedteam_Freeze_ConsumerBypassesCheckOrder(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	// Create a valid but STALE registry
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// Use a time well past valid_until
	farFuture := time.Unix(reg.ValidUntil+86400*365, 0) // 1 year past valid_until

	// Even though the sig is valid and epoch is fine, freshness must fail
	_, _, err = VerifyAndLoad(rootPub, env, 4, farFuture, time.Hour)
	if err == nil {
		t.Fatal("VULNERABILITY: stale registry accepted despite being past valid_until")
	}

	// Now test: bad sig AND stale — should still reject (sig check first)
	_, otherPriv := genKey(t)
	badEnv, _ := SignRegistry(otherPriv, reg)
	_, _, err = VerifyAndLoad(rootPub, badEnv, 4, farFuture, time.Hour)
	if err == nil {
		t.Fatal("VULNERABILITY: bad-sig + stale registry was accepted")
	}
}
