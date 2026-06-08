package registry

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

// TestRedteam_Overflow_LargeEpochJSONRoundtrip tests what happens when a large
// epoch value round-trips through JSON. Go's encoding/json uses float64 for
// numbers, which loses precision above 2^53. An attacker could craft a JSON
// with epoch just above 2^53 that silently truncates/rounds.
func TestRedteam_Overflow_LargeEpochJSONRoundtrip(t *testing.T) {
	// 2^53 + 1 = 9007199254740993 — this cannot be represented exactly as float64.
	// Go json.Unmarshal into int uses a different path than float64 on modern Go,
	// but let's verify.
	type testReg struct {
		Epoch int `json:"epoch"`
	}
	raw := `{"epoch":9007199254740993}`
	var tr testReg
	if err := json.Unmarshal([]byte(raw), &tr); err != nil {
		t.Logf("INFO: Go rejects epoch 2^53+1 via json.Unmarshal into int: %v", err)
		return
	}
	if tr.Epoch != 9007199254740993 {
		t.Fatalf("VULNERABILITY: epoch 2^53+1 round-tripped as %d — precision loss in JSON", tr.Epoch)
	}
	t.Logf("CORRECT: Go's json.Unmarshal into int handles 2^53+1 correctly: %d", tr.Epoch)
}

// TestRedteam_Overflow_MaxIntEpochSignAndVerify asserts that a JCS-UNSAFE large
// epoch (above 2^53, here ~MaxInt64/2) is REJECTED at sign time. Such epochs
// round under IEEE-754 double formatting (epoch confusion across languages), so
// they must never be minted.
func TestRedteam_Overflow_MaxIntEpochSignAndVerify(t *testing.T) {
	_, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	largeEpoch := int(math.MaxInt64 >> 1) // ~4.6e18, far above MaxSafeEpoch (2^53)
	reg := freshRegistry(largeEpoch, liveBinding(t, "mira", agentPub, 1))
	if _, err := SignRegistry(rootPriv, reg); err == nil {
		t.Fatal("SECURITY: SignRegistry minted a JCS-unsafe epoch above 2^53 — epoch-confusion vector")
	}
}

// TestRedteam_Overflow_MaxIntViaJSONMarshal asserts that a MaxInt64 epoch — which
// JCS renders in scientific notation and json.Unmarshal then cannot parse back
// into an int (a DoS on the load path) — is REJECTED at sign time, so it never
// reaches the consumer.
func TestRedteam_Overflow_MaxIntViaJSONMarshal(t *testing.T) {
	_, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	reg := freshRegistry(math.MaxInt, liveBinding(t, "mira", agentPub, 1))
	if _, err := SignRegistry(rootPriv, reg); err == nil {
		t.Fatal("SECURITY: SignRegistry minted a MaxInt epoch — JCS scientific-notation DoS vector")
	}
}

// TestRedteam_FutureValidFrom_PerpetualFreshness demonstrates the future
// valid_from attack more concretely: a registry issued with valid_from set
// 10 years in the future, with valid_until 20 years in the future, passes
// ALL freshness checks for a consumer running at any time in the next 10 years.
func TestRedteam_FutureValidFrom_PerpetualFreshness(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)

	tenYears := int64(86400 * 365 * 10)
	reg := Registry{
		Epoch:      5,
		ValidFrom:  fixedNow.Unix() + tenYears,   // 10 years in the future
		ValidUntil: fixedNow.Unix() + tenYears*2, // 20 years in the future
		Agents:     []AgentBinding{liveBinding(t, "mira", agentPub, 1)},
	}
	env, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	// Consumer with a 24-hour maxStaleness
	maxStale := time.Hour * 24

	// At every point in the next 10 years (all BEFORE valid_from), the registry
	// MUST be rejected — the now < valid_from guard denies perpetual freshness.
	checkTimes := []struct {
		name string
		t    time.Time
	}{
		{"now", fixedNow},
		{"1 year from now", fixedNow.Add(time.Hour * 24 * 365)},
		{"5 years from now", fixedNow.Add(time.Hour * 24 * 365 * 5)},
		{"9 years from now", fixedNow.Add(time.Hour * 24 * 365 * 9)},
	}

	for _, tc := range checkTimes {
		if _, _, err := VerifyAndLoad(rootPub, env, 4, tc.t, maxStale); err == nil {
			t.Fatalf("SECURITY: future valid_from registry accepted at %s — perpetual-freshness bypass", tc.name)
		}
	}
}
