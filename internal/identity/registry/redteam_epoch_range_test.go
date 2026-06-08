package registry

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
)

// =============================================================================
// ADVERSARIAL RE-ATTACK: EPOCH RANGE VALIDATION
//
// Target: commit 5086a9bfc40d9eb3185cc3414c376fc8d79e828c
// Claim under test: epochs outside [1, 2^53] are REJECTED by BOTH SignRegistry
// AND VerifyAndLoad. No JCS precision-loss path remains where a signed epoch
// differs from the loaded epoch. In-range epochs (1, 2^53) still work.
// =============================================================================

// --- SECTION 1: SignRegistry rejects every out-of-range epoch ---

func TestReattack_SignRegistry_RejectsOutOfRange(t *testing.T) {
	_, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	outOfRange := []struct {
		name  string
		epoch int
	}{
		{"zero", 0},
		{"negative_minus1", -1},
		{"negative_minus42", -42},
		{"negative_MinInt64", math.MinInt64},
		{"2^53+1", (1 << 53) + 1},
		{"2^53+3", (1 << 53) + 3},
		{"2^54", 1 << 54},
		{"2^62", 1 << 62},
		{"MaxInt64", math.MaxInt64},
		{"MaxInt64/2", math.MaxInt64 >> 1},
	}

	for _, tc := range outOfRange {
		t.Run("registry_epoch/"+tc.name, func(t *testing.T) {
			reg := freshRegistry(tc.epoch, liveBinding(t, "mira", agentPub, 1))
			_, err := SignRegistry(rootPriv, reg)
			if err == nil {
				t.Fatalf("SECURITY: SignRegistry minted registry epoch %d (%s) — must be rejected (outside [1,2^53])",
					tc.epoch, tc.name)
			}
			if !strings.Contains(err.Error(), ErrEpochRange) {
				t.Fatalf("wrong error for epoch %d: got %v, want substring %q", tc.epoch, err, ErrEpochRange)
			}
		})

		t.Run("binding_key_epoch/"+tc.name, func(t *testing.T) {
			b := liveBinding(t, "mira", agentPub, tc.epoch)
			reg := freshRegistry(5, b)
			_, err := SignRegistry(rootPriv, reg)
			if err == nil {
				t.Fatalf("SECURITY: SignRegistry minted binding key_epoch %d (%s) — must be rejected (outside [1,2^53])",
					tc.epoch, tc.name)
			}
			if !strings.Contains(err.Error(), ErrEpochRange) {
				t.Fatalf("wrong error for key_epoch %d: got %v, want substring %q", tc.epoch, err, ErrEpochRange)
			}
		})
	}
}

// --- SECTION 2: VerifyAndLoad rejects hand-crafted bodies with out-of-range epochs ---
//
// THREAT MODEL: An attacker who controls the offline root key (or compromises
// it) can produce arbitrary SignedRegistry envelopes. VerifyAndLoad is the
// defense-in-depth layer that must reject out-of-range epochs in the
// AUTHENTICATED body, even when the root signature is valid.
//
// JCS canonicalization normalizes numbers through float64, so:
// - epoch 2^53+1 in raw JSON => canonical bytes contain 2^53 (precision loss)
// - epoch 0, -1 in raw JSON => canonical bytes contain 0, -1 (preserved)
//
// For epochs that JCS normalizes to an in-range value (like 2^53+1 => 2^53),
// the attack is actually CLOSED by the JCS normalization itself: the consumer
// loads epoch 2^53, which is valid. There is no epoch confusion because the
// signed bytes and decoded bytes agree. The real security property is:
// SignRegistry REFUSES to sign such epochs, preventing them from entering the
// system in the first place.
//
// For epochs that remain out-of-range after JCS (0, -1, very large), the
// defense-in-depth in VerifyAndLoad must catch them.

func TestReattack_VerifyAndLoad_RejectsHandCraftedOutOfRange(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	agentB64 := base64Std(agentPub)

	// Epochs that remain out-of-range after JCS normalization.
	outOfRange := []struct {
		name  string
		epoch string // JSON literal
	}{
		{"zero", "0"},
		{"negative", "-1"},
		// 2^54 is a power of two, so float64 represents it exactly => JCS preserves it.
		// It is above MaxSafeEpoch, so VerifyAndLoad must reject it.
		{"2^54", "18014398509481984"},
	}

	for _, tc := range outOfRange {
		t.Run("registry_epoch/"+tc.name, func(t *testing.T) {
			body := fmt.Sprintf(
				`{"agents":[{"authorized_origin_daemons":["d"],"display_name":"X","key_epoch":1,"pubkey":"%s","slug":"mira","valid_from":1770000000,"valid_until":1780000000}],"epoch":%s,"valid_from":1770000000,"valid_until":1780000000}`,
				agentB64, tc.epoch,
			)
			env := handSign(t, rootPriv, body)
			_, _, err := VerifyAndLoad(rootPub, env, 0, fixedNow, time.Hour*24*365)
			if err == nil {
				t.Fatalf("SECURITY: VerifyAndLoad accepted hand-signed registry epoch %s — must be rejected", tc.name)
			}
		})
	}

	// Out-of-range binding key_epoch in a hand-signed body (values preserved by JCS).
	outOfRangeKeyEpoch := []struct {
		name     string
		keyEpoch string
	}{
		{"zero", "0"},
		{"negative", "-1"},
		// 2^54 exact in float64 => JCS preserves it, still above MaxSafeEpoch.
		{"2^54", "18014398509481984"},
	}

	for _, tc := range outOfRangeKeyEpoch {
		t.Run("binding_key_epoch/"+tc.name, func(t *testing.T) {
			body := fmt.Sprintf(
				`{"agents":[{"authorized_origin_daemons":["d"],"display_name":"X","key_epoch":%s,"pubkey":"%s","slug":"mira","valid_from":1770000000,"valid_until":1780000000}],"epoch":5,"valid_from":1770000000,"valid_until":1780000000}`,
				tc.keyEpoch, agentB64,
			)
			env := handSign(t, rootPriv, body)
			_, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
			if err == nil {
				t.Fatalf("SECURITY: VerifyAndLoad accepted hand-signed binding key_epoch %s — must be rejected", tc.name)
			}
		})
	}
}

// --- SECTION 3: JCS precision-loss normalization closes the confusion path ---
// Confirm that an epoch above 2^53 (specifically 2^53+1) gets silently rounded
// by JCS to 2^53, so the canonical bytes + decoded body agree. The attacker
// cannot create a signed body where the canonical bytes say one epoch but the
// decoded body says another.

func TestReattack_JCS_PrecisionLoss_NormalizesTo2Pow53(t *testing.T) {
	// 2^53+1 = 9007199254740993 — cannot be represented exactly as float64.
	// float64(9007199254740993) rounds to 9007199254740992.0
	input := `{"epoch":9007199254740993}`
	canonical, err := identity.Canonicalize([]byte(input))
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	output := string(canonical.Bytes())

	// JCS must round 2^53+1 to 2^53 (confirmed by gowebpki/jcs v1.0.1 behavior).
	if !strings.Contains(output, "9007199254740992") {
		t.Fatalf("JCS did not normalize 2^53+1 to 2^53 — unexpected output: %s", output)
	}
	if strings.Contains(output, "9007199254740993") {
		t.Fatal("JCS preserved 2^53+1 exactly — the epoch-range check on canonical bytes would see the original value")
	}

	// The decoded epoch from canonical bytes is 2^53, not 2^53+1.
	var decoded struct {
		Epoch int `json:"epoch"`
	}
	if err := json.Unmarshal(canonical.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Epoch != 1<<53 {
		t.Fatalf("decoded epoch = %d, want %d (2^53)", decoded.Epoch, 1<<53)
	}
	t.Logf("CONFIRMED: JCS normalized 2^53+1 => 2^53. No epoch confusion in canonical bytes.")
}

// TestReattack_JCS_2pow53plus1_CannotBypass demonstrates the full attack path:
// an attacker crafts a body with epoch 2^53+1, hand-signs it, and presents it
// to VerifyAndLoad. The JCS normalization rounds the epoch to 2^53 in the
// canonical bytes, so VerifyAndLoad sees epoch 2^53 (in-range) and LOADS it.
// This is NOT a bypass — the consumer genuinely gets epoch 2^53, and the
// anti-rollback monotonic counter works correctly on that value. The confusion
// (attacker intended 2^53+1 but consumer got 2^53) is prevented by
// SignRegistry refusing to sign 2^53+1.
func TestReattack_JCS_2pow53plus1_NoEpochConfusion(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	agentB64 := base64Std(agentPub)

	// Step 1: SignRegistry REFUSES to mint 2^53+1.
	reg := freshRegistry((1<<53)+1, liveBinding(t, "mira", agentPub, 1))
	if _, err := SignRegistry(rootPriv, reg); err == nil {
		t.Fatal("SECURITY: SignRegistry accepted epoch 2^53+1 — the primary defense is broken")
	}

	// Step 2: An attacker who hand-signs a body with epoch 2^53+1 gets JCS
	// normalization to 2^53. Verify that the loaded registry epoch is 2^53.
	body := fmt.Sprintf(
		`{"agents":[{"authorized_origin_daemons":["d"],"display_name":"X","key_epoch":1,"pubkey":"%s","slug":"mira","valid_from":1770000000,"valid_until":1780000000}],"epoch":9007199254740993,"valid_from":1770000000,"valid_until":1780000000}`,
		agentB64,
	)
	env := handSign(t, rootPriv, body)
	loaded, newH, err := VerifyAndLoad(rootPub, env, (1<<53)-1, fixedNow, time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad: %v (this is expected to LOAD because canonical epoch is 2^53)", err)
	}

	// The loaded epoch MUST be 2^53, not 2^53+1.
	if loaded.Epoch != 1<<53 {
		t.Fatalf("loaded epoch = %d, want %d (2^53) — epoch confusion!", loaded.Epoch, 1<<53)
	}
	if newH != 1<<53 {
		t.Fatalf("newHighest = %d, want %d", newH, 1<<53)
	}
	t.Logf("CONFIRMED: hand-signed 2^53+1 body loads as epoch 2^53 — no confusion, anti-rollback works on the actual value")
}

// --- SECTION 4: In-range epochs work end-to-end (no over-rejection) ---

func TestReattack_InRange_Epoch1_E2E(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, agentPriv := genKey(t)

	reg := freshRegistry(1, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry at epoch 1: %v", err)
	}
	loaded, newH, err := VerifyAndLoad(rootPub, env, 0, fixedNow, time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad at epoch 1: %v", err)
	}
	if newH != 1 {
		t.Fatalf("newHighest = %d, want 1", newH)
	}
	if loaded.Epoch != 1 {
		t.Fatalf("loaded epoch = %d, want 1", loaded.Epoch)
	}

	_, err = Resolve(loaded, "mira", fixedNow)
	if err != nil {
		t.Fatalf("Resolve at epoch 1: %v", err)
	}
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"epoch-1"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	ok, err := VerifyMessage(loaded, "mira", 1, canonical, sig, fixedNow)
	if err != nil {
		t.Fatalf("VerifyMessage at epoch 1: %v", err)
	}
	if !ok {
		t.Fatal("VerifyMessage at epoch 1 = false, want true")
	}
}

func TestReattack_InRange_MaxSafeEpoch_E2E(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, agentPriv := genKey(t)

	epoch := MaxSafeEpoch
	keyEpoch := MaxSafeEpoch

	reg := freshRegistry(epoch, liveBinding(t, "mira", agentPub, keyEpoch))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry at MaxSafeEpoch: %v", err)
	}
	loaded, newH, err := VerifyAndLoad(rootPub, env, epoch-1, fixedNow, time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad at MaxSafeEpoch: %v", err)
	}
	if newH != epoch {
		t.Fatalf("newHighest = %d, want %d", newH, epoch)
	}
	if loaded.Epoch != epoch {
		t.Fatalf("loaded epoch = %d, want %d", loaded.Epoch, epoch)
	}

	_, err = Resolve(loaded, "mira", fixedNow)
	if err != nil {
		t.Fatalf("Resolve at MaxSafeEpoch: %v", err)
	}
	canonical, _ := identity.Canonicalize([]byte(`{"msg":"max-safe"}`))
	sig, _ := identity.SignCanonical(agentPriv, canonical)
	ok, err := VerifyMessage(loaded, "mira", keyEpoch, canonical, sig, fixedNow)
	if err != nil {
		t.Fatalf("VerifyMessage at MaxSafeEpoch: %v", err)
	}
	if !ok {
		t.Fatal("VerifyMessage at MaxSafeEpoch = false, want true")
	}
}

// --- SECTION 5: Boundary fence-post tests (SignRegistry) ---

func TestReattack_BoundaryFencePosts(t *testing.T) {
	_, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	tests := []struct {
		name      string
		epoch     int
		wantAllow bool
	}{
		{"epoch_0_reject", 0, false},
		{"epoch_1_accept", 1, true},
		{"epoch_2_accept", 2, true},
		{"epoch_2^53-1_accept", (1 << 53) - 1, true},
		{"epoch_2^53_accept", 1 << 53, true},
		{"epoch_2^53+1_reject", (1 << 53) + 1, false},
		{"epoch_2^53+2_reject", (1 << 53) + 2, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := freshRegistry(tc.epoch, liveBinding(t, "mira", agentPub, 1))
			_, err := SignRegistry(rootPriv, reg)
			if tc.wantAllow && err != nil {
				t.Fatalf("over-rejection: epoch %d must be allowed, got: %v", tc.epoch, err)
			}
			if !tc.wantAllow && err == nil {
				t.Fatalf("SECURITY: epoch %d must be rejected (outside [1,2^53])", tc.epoch)
			}
		})
	}
}

// --- SECTION 6: epochInRange unit tests ---

func TestReattack_EpochInRange_Unit(t *testing.T) {
	tests := []struct {
		epoch int
		want  bool
	}{
		{math.MinInt64, false},
		{-1, false},
		{0, false},
		{1, true},
		{2, true},
		{(1 << 53) - 1, true},
		{1 << 53, true},
		{(1 << 53) + 1, false},
		{(1 << 53) + 3, false},
		{math.MaxInt64 >> 1, false},
		{math.MaxInt64, false},
	}

	for _, tc := range tests {
		name := fmt.Sprintf("epoch_%d", tc.epoch)
		t.Run(name, func(t *testing.T) {
			got := epochInRange(tc.epoch)
			if got != tc.want {
				t.Fatalf("epochInRange(%d) = %v, want %v", tc.epoch, got, tc.want)
			}
		})
	}
}

// --- SECTION 7: VerifyAndLoad defense-in-depth for JCS-preserved out-of-range epochs ---
// Epochs that are exact in float64 but above 2^53 (powers of 2: 2^54, 2^62)
// pass through JCS unchanged, so the VerifyAndLoad epoch-range check on the
// decoded body is the ONLY defense.

func TestReattack_VerifyAndLoad_JCSPreserved_LargeEpoch(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	agentB64 := base64Std(agentPub)

	// 2^54 = 18014398509481984 is exact in float64, so JCS preserves it.
	// It is above MaxSafeEpoch (2^53), so VerifyAndLoad MUST reject it.
	body := fmt.Sprintf(
		`{"agents":[{"authorized_origin_daemons":["d"],"display_name":"X","key_epoch":1,"pubkey":"%s","slug":"mira","valid_from":1770000000,"valid_until":1780000000}],"epoch":18014398509481984,"valid_from":1770000000,"valid_until":1780000000}`,
		agentB64,
	)
	env := handSign(t, rootPriv, body)
	_, _, err := VerifyAndLoad(rootPub, env, 0, fixedNow, time.Hour*24*365)
	if err == nil {
		t.Fatal("SECURITY: VerifyAndLoad accepted epoch 2^54 — JCS preserves it exactly but it is above MaxSafeEpoch")
	}
	if !strings.Contains(err.Error(), ErrEpochRange) {
		t.Fatalf("wrong error: got %v, want substring %q", err, ErrEpochRange)
	}
}

func TestReattack_VerifyAndLoad_JCSPreserved_LargeKeyEpoch(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	agentB64 := base64Std(agentPub)

	// Same but for binding key_epoch at 2^54.
	body := fmt.Sprintf(
		`{"agents":[{"authorized_origin_daemons":["d"],"display_name":"X","key_epoch":18014398509481984,"pubkey":"%s","slug":"mira","valid_from":1770000000,"valid_until":1780000000}],"epoch":5,"valid_from":1770000000,"valid_until":1780000000}`,
		agentB64,
	)
	env := handSign(t, rootPriv, body)
	_, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Hour*24*365)
	if err == nil {
		t.Fatal("SECURITY: VerifyAndLoad accepted binding key_epoch 2^54")
	}
	if !strings.Contains(err.Error(), ErrEpochRange) {
		t.Fatalf("wrong error: got %v, want substring %q", err, ErrEpochRange)
	}
}

// --- SECTION 8: MaxInt64 DoS path ---
// MaxInt64 through JCS gets scientific notation (float64 overflow territory).
// Either canonicalize fails, unmarshal fails, or the epoch-range check catches it.

func TestReattack_VerifyAndLoad_RejectsMaxInt64InBody(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	agentB64 := base64Std(agentPub)

	body := fmt.Sprintf(
		`{"agents":[{"authorized_origin_daemons":["d"],"display_name":"X","key_epoch":1,"pubkey":"%s","slug":"mira","valid_from":1770000000,"valid_until":1780000000}],"epoch":9223372036854775807,"valid_from":1770000000,"valid_until":1780000000}`,
		agentB64,
	)

	canonical, err := identity.Canonicalize([]byte(body))
	if err != nil {
		t.Logf("CORRECT: Canonicalize rejected MaxInt64 epoch: %v", err)
		return
	}
	sig, err := identity.SignCanonical(rootPriv, canonical)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	env := SignedRegistry{Registry: canonical.Bytes(), RootSig: sig, RootKeyEpoch: 0}
	_, _, err = VerifyAndLoad(rootPub, env, 0, fixedNow, time.Hour*24*365)
	if err == nil {
		t.Fatal("SECURITY: VerifyAndLoad accepted MaxInt64 epoch from an authenticated body")
	}
	t.Logf("CORRECT: VerifyAndLoad rejected MaxInt64 epoch: %v", err)
}

// --- SECTION 9: Multiple bindings with mixed epoch validity ---

func TestReattack_SignRegistry_OneValidOneInvalidKeyEpoch(t *testing.T) {
	_, rootPriv := genKey(t)
	pub1, _ := genKey(t)
	pub2, _ := genKey(t)

	revokedAt := int(1_770_000_100)
	b1 := liveBinding(t, "ada", pub1, 1)
	b2 := liveBinding(t, "mira", pub2, (1<<53)+1)
	b2.RevokedAt = &revokedAt

	reg := freshRegistry(5, b1, b2)
	_, err := SignRegistry(rootPriv, reg)
	if err == nil {
		t.Fatal("SECURITY: SignRegistry minted a registry with one invalid binding key_epoch")
	}
	if !strings.Contains(err.Error(), ErrEpochRange) {
		t.Fatalf("wrong error: got %v, want substring %q", err, ErrEpochRange)
	}
}

// --- SECTION 10: JCS round-trip fidelity for in-range epochs ---

func TestReattack_JCS_NoConfusionInRange(t *testing.T) {
	_, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	safeEpochs := []struct {
		name  string
		epoch int
	}{
		{"1", 1},
		{"42", 42},
		{"1000000", 1_000_000},
		{"2^52", 1 << 52},
		{"2^53-1", (1 << 53) - 1},
		{"2^53", 1 << 53},
	}

	for _, tc := range safeEpochs {
		t.Run(tc.name, func(t *testing.T) {
			reg := freshRegistry(tc.epoch, liveBinding(t, "mira", agentPub, 1))
			cb, err := canonicalBytes(reg)
			if err != nil {
				t.Fatalf("canonicalBytes: %v", err)
			}
			epochStr := fmt.Sprintf(`"epoch":%d`, tc.epoch)
			if !strings.Contains(string(cb.Bytes()), epochStr) {
				t.Fatalf("JCS CONFUSION: canonical bytes do not contain %q — got: %s",
					epochStr, string(cb.Bytes()))
			}

			env, err := SignRegistry(rootPriv, reg)
			if err != nil {
				t.Fatalf("SignRegistry: %v", err)
			}
			rootPub := rootPriv.Public().(ed25519.PublicKey)
			loaded, newH, err := VerifyAndLoad(rootPub, env, tc.epoch-1, fixedNow, time.Hour*24*365)
			if err != nil {
				t.Fatalf("VerifyAndLoad: %v", err)
			}
			if loaded.Epoch != tc.epoch {
				t.Fatalf("loaded epoch = %d, want %d", loaded.Epoch, tc.epoch)
			}
			if newH != tc.epoch {
				t.Fatalf("newHighest = %d, want %d", newH, tc.epoch)
			}
		})
	}
}

// --- SECTION 11: MaxSafeEpoch constant value ---

func TestReattack_MaxSafeEpoch_Is2Pow53(t *testing.T) {
	if MaxSafeEpoch != 1<<53 {
		t.Fatalf("MaxSafeEpoch = %d, want %d (2^53)", MaxSafeEpoch, 1<<53)
	}
	if MaxSafeEpoch != 9007199254740992 {
		t.Fatalf("MaxSafeEpoch = %d, want 9007199254740992", MaxSafeEpoch)
	}
}

// --- SECTION 12: Cross-language parity concern ---
// The reason SignRegistry rejects > 2^53 even though JCS normalizes it: a Rust
// or JavaScript verifier parsing the ORIGINAL pre-canonical JSON would see
// 2^53+1, while Go after JCS sees 2^53. The fix prevents this parity break by
// refusing to mint such values at the source.

func TestReattack_CrossLanguageParity_SignRegistryPrevents(t *testing.T) {
	_, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	// 2^53+1: Go's json.Unmarshal into int handles it correctly (9007199254740993),
	// but JCS normalizes it to 9007199254740992. SignRegistry must refuse to create
	// this discrepancy.
	reg := freshRegistry((1<<53)+1, liveBinding(t, "mira", agentPub, 1))
	_, err := SignRegistry(rootPriv, reg)
	if err == nil {
		t.Fatal("SECURITY: SignRegistry minted 2^53+1 — cross-language parity break")
	}

	// Also 2^53+3: same issue, different odd number.
	reg = freshRegistry((1<<53)+3, liveBinding(t, "mira", agentPub, 1))
	_, err = SignRegistry(rootPriv, reg)
	if err == nil {
		t.Fatal("SECURITY: SignRegistry minted 2^53+3 — cross-language parity break")
	}
}

// --- helpers ---

// handSign canonicalizes body, signs with rootPriv, and returns a SignedRegistry.
func handSign(t *testing.T, rootPriv ed25519.PrivateKey, body string) SignedRegistry {
	t.Helper()
	canonical, err := identity.Canonicalize([]byte(body))
	if err != nil {
		t.Fatalf("handSign: canonicalize: %v", err)
	}
	sig, err := identity.SignCanonical(rootPriv, canonical)
	if err != nil {
		t.Fatalf("handSign: sign: %v", err)
	}
	return SignedRegistry{Registry: canonical.Bytes(), RootSig: sig, RootKeyEpoch: 0}
}
