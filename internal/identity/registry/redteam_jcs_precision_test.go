package registry

import (
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
)

// TestRedteam_JCS_EpochPrecisionLoss asserts the epoch RANGE invariant that
// closes the JCS precision-loss hole: SignRegistry accepts epochs in
// [1, 2^53] (JCS renders them losslessly) and REJECTS any epoch above 2^53
// (which would silently round under IEEE-754 double formatting → epoch
// confusion / cross-language parity break).
func TestRedteam_JCS_EpochPrecisionLoss(t *testing.T) {
	testCases := []struct {
		name      string
		epoch     int
		wantAllow bool
	}{
		{"2^52", 1 << 52, true},
		{"2^53 - 1", (1 << 53) - 1, true},
		{"2^53", 1 << 53, true},            // MaxSafeEpoch boundary — allowed
		{"2^53 + 1", (1 << 53) + 1, false}, // first unsafe value
		{"2^53 + 2", (1 << 53) + 2, false},
		{"2^53 + 3", (1 << 53) + 3, false},
		{"2^62", 1 << 62, false},
		{"MaxInt64/2", int(1<<63-1) >> 1, false},
	}

	_, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reg := freshRegistry(tc.epoch, liveBinding(t, "mira", agentPub, 1))
			_, err := SignRegistry(rootPriv, reg)
			if tc.wantAllow && err != nil {
				t.Fatalf("epoch %d (%s) is JCS-safe and must be signable, got: %v", tc.epoch, tc.name, err)
			}
			if !tc.wantAllow && err == nil {
				t.Fatalf("SECURITY: epoch %d (%s) is JCS-UNSAFE (> 2^53) and must be rejected", tc.epoch, tc.name)
			}
		})
	}
}

// TestRedteam_JCS_EpochPrecisionLoss_FullPath asserts the full path is closed at
// epoch 2^53+1: SignRegistry refuses to mint it, and even a hand-signed body
// carrying it is rejected by VerifyAndLoad — no epoch confusion or DoS reaches
// the consumer.
func TestRedteam_JCS_EpochPrecisionLoss_FullPath(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	// 2^53 + 1 = 9007199254740993
	epoch := (1 << 53) + 1
	reg := freshRegistry(epoch, liveBinding(t, "mira", agentPub, 1))
	if _, err := SignRegistry(rootPriv, reg); err == nil {
		t.Fatal("SECURITY: SignRegistry minted epoch 2^53+1 — JCS precision-loss vector")
	}

	// Hand-sign a body at a registry-level epoch ABOVE 2^53 that JCS renders
	// exactly (2^54 = 18014398509481984, a power of two) and confirm the load
	// gate rejects it via the epoch-range check on the authenticated body.
	const aboveSafe = 1 << 54 // 18014398509481984, > MaxSafeEpoch, JCS-exact
	body := `{"agents":[{"authorized_origin_daemons":["d"],"display_name":"X",` +
		`"key_epoch":1,"pubkey":"` + base64Std(agentPub) + `","slug":"mira",` +
		`"valid_from":1770000000,"valid_until":1780000000}],` +
		`"epoch":18014398509481984,"valid_from":1770000000,"valid_until":1780000000}`
	canonical, err := identity.Canonicalize([]byte(body))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	sig, err := identity.SignCanonical(rootPriv, canonical)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	env := SignedRegistry{Registry: canonical.Bytes(), RootSig: sig, RootKeyEpoch: 0}
	if _, _, err := VerifyAndLoad(rootPub, env, aboveSafe-2, fixedNow, time.Hour*24*365); err == nil {
		t.Fatal("SECURITY: VerifyAndLoad accepted a hand-signed epoch above 2^53")
	}
}

// TestRedteam_JCS_SafeEpochRange_2pow53 demonstrates that epochs within the
// JSON-safe integer range (up to 2^53) work correctly end to end.
func TestRedteam_JCS_SafeEpochRange_2pow53(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	epoch := 1 << 53 // Exactly 2^53, the maximum safe integer
	reg := freshRegistry(epoch, liveBinding(t, "mira", agentPub, 1))
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	_, newHighest, err := VerifyAndLoad(rootPub, env, epoch-1, fixedNow, time.Hour*24*365)
	if err != nil {
		t.Fatalf("VerifyAndLoad at 2^53 should succeed: %v", err)
	}
	if newHighest != epoch {
		t.Fatalf("newHighest = %d, want %d", newHighest, epoch)
	}
	t.Logf("CORRECT: epoch 2^53 (%d) works correctly end to end", epoch)
}

// TestRedteam_Freshness_FutureValidFromBypassesStaleness is a focused
// demonstration that a future valid_from completely defeats maxStaleness.
func TestRedteam_Freshness_FutureValidFromBypassesStaleness(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	// Even with a 1-SECOND maxStaleness (extremely tight), a future valid_from
	// bypasses it because now - future = negative < 1 second.
	reg := Registry{
		Epoch:      5,
		ValidFrom:  fixedNow.Unix() + 86400*365, // 1 year in the future
		ValidUntil: fixedNow.Unix() + 86400*730, // 2 years in the future
		Agents:     []AgentBinding{liveBinding(t, "mira", agentPub, 1)},
	}
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// 1-second maxStaleness: a future valid_from must be rejected by the
	// now < valid_from guard, not slip through on a negative staleness duration.
	if _, _, err := VerifyAndLoad(rootPub, env, 4, fixedNow, time.Second); err == nil {
		t.Fatal("SECURITY: future valid_from (1 year ahead) accepted with 1-second maxStaleness — " +
			"staleness-bound bypass")
	}
}
