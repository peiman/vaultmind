package envelope

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

// updateMsgFixture, when set (-update), regenerates the committed message-signing
// vectors. Without it, the fixture is a stable artifact, only loaded + asserted.
var updateMsgFixture = flag.Bool("update", false, "regenerate the committed message-signing fixture")

// msgFixturePath is the committed cross-language message-signing acceptance
// fixture — the artifact workhorse's chat-mcp + daemon (Rust verifier) bind to.
const msgFixturePath = "testdata/message_signing_vectors.json"

// --- Fixture JSON schema (the cross-language message-signing contract) ---

// msgFixture is the top-level fixture: a pinned root pubkey, a reference time, a
// signed REGISTRY (so the verifier can resolve the signer's binding), a single
// VALID signed envelope, and an array of named REJECT cases. Every byte is
// deterministic (fixed seeds).
type msgFixture struct {
	RootPubKey      string          `json:"root_pubkey"`
	Now             int64           `json:"now"`
	MaxStalenessSec int64           `json:"max_staleness_secs"`
	Registry        json.RawMessage `json:"registry"`
	PersistedEpoch  int             `json:"persisted_highest_epoch"`
	Valid           msgValidCase    `json:"valid"`
	Reject          []msgRejectCase `json:"reject"`
}

// wireFields is the JSON shape of the envelope's SIGNED-SUBSET fields, plus sig
// and the convenience from_pubkey hint. room/to_agent are omitempty pointers so
// the absent one is OMITTED (never null) — mirroring the canonical contract.
type wireFields struct {
	AlgVersion int64   `json:"alg_version"`
	Body       string  `json:"body"`
	FromAgent  string  `json:"from_agent"`
	KeyEpoch   int64   `json:"key_epoch"`
	Nonce      string  `json:"nonce"`
	Room       *string `json:"room,omitempty"`
	Seq        int64   `json:"seq"`
	ToAgent    *string `json:"to_agent,omitempty"`
	TS         int64   `json:"ts"`
	// Sig is base64-std of the ed25519 signature over the canonical signed subset.
	Sig string `json:"sig"`
	// FromPubKey is the convenience hint (DERIVED, NOT signed).
	FromPubKey string `json:"from_pubkey"`
}

// toFields converts the wire form back into Fields for the verify path.
func (w wireFields) toFields() Fields {
	return Fields{
		AlgVersion: w.AlgVersion,
		Body:       w.Body,
		FromAgent:  w.FromAgent,
		KeyEpoch:   w.KeyEpoch,
		Nonce:      w.Nonce,
		Room:       w.Room,
		ToAgent:    w.ToAgent,
		Seq:        w.Seq,
		TS:         w.TS,
	}
}

// msgValidCase is a complete signed envelope plus its canonical signed bytes
// (base64) so a cross-language verifier can cross-check its own canonicalization.
type msgValidCase struct {
	Envelope       wireFields `json:"envelope"`
	CanonicalBytes string     `json:"canonical_signed_bytes"`
}

// msgRejectCase is a negative case that MUST fail VerifyEnvelope (or, for the
// gate-only cases, CanonicalizeEnvelope) under the fixture's registry/now.
type msgRejectCase struct {
	Name     string     `json:"name"`
	Reason   string     `json:"reason"`
	Expect   string     `json:"expect"` // always "reject"
	Envelope wireFields `json:"envelope"`
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// signWire gates+canonicalizes f and signs it with priv, returning the wire form
// with sig + from_pubkey populated. It signs DIRECTLY (not via the UDS client)
// because the fixture generator is deterministic and key-holding by design.
func signWire(t *testing.T, priv ed25519.PrivateKey, pub ed25519.PublicKey, f Fields) wireFields {
	t.Helper()
	canonical, err := CanonicalizeEnvelope(f)
	require.NoError(t, err)
	sig, err := identity.SignCanonical(priv, canonical)
	require.NoError(t, err)
	return wireFields{
		AlgVersion: f.AlgVersion, Body: f.Body, FromAgent: f.FromAgent,
		KeyEpoch: f.KeyEpoch, Nonce: f.Nonce, Room: f.Room, Seq: f.Seq,
		ToAgent: f.ToAgent, TS: f.TS,
		Sig: b64(sig), FromPubKey: b64(pub),
	}
}

// rawWire builds a wire form for a gate-only reject case (no valid signature is
// needed because the gate fires before any signature check). A dummy sig keeps
// the structure complete.
func rawWire(f Fields, pub ed25519.PublicKey) wireFields {
	return wireFields{
		AlgVersion: f.AlgVersion, Body: f.Body, FromAgent: f.FromAgent,
		KeyEpoch: f.KeyEpoch, Nonce: f.Nonce, Room: f.Room, Seq: f.Seq,
		ToAgent: f.ToAgent, TS: f.TS,
		Sig: b64(make([]byte, ed25519.SignatureSize)), FromPubKey: b64(pub),
	}
}

// buildMsgFixture deterministically constructs the entire fixture from fixed seeds.
func buildMsgFixture(t *testing.T) msgFixture {
	t.Helper()
	rootPub, rootPriv := fixedEd25519(t, 0xA1)
	agentPub, agentPriv := fixedEd25519(t, 0xB2)
	wrongPub, wrongPriv := fixedEd25519(t, 0xEE)

	const (
		now       int64 = 2_000_000
		maxStale  int64 = 86_400
		regFrom   int64 = 1_990_000
		regUntil  int64 = 3_000_000
		regEpoch        = 10
		persisted       = 9
	)

	apk, err := registry.NewPublicKey(agentPub)
	require.NoError(t, err)
	reg := registry.Registry{
		Epoch: regEpoch, ValidFrom: regFrom, ValidUntil: regUntil,
		Agents: []registry.AgentBinding{{
			Slug: "mira", DisplayName: "Mira ⭐", PubKey: apk, KeyEpoch: 1,
			ValidFrom: regFrom, ValidUntil: regUntil, AuthorizedOriginDaemons: []string{"daemon-1"},
		}},
	}
	signedReg, err := registry.SignRegistry(rootPriv, reg)
	require.NoError(t, err)
	regEnvelope, err := registry.MarshalDistribution(signedReg)
	require.NoError(t, err)

	base := Fields{
		AlgVersion: AlgVersion, Body: "hello ⭐", FromAgent: "mira", KeyEpoch: 1,
		Nonce: "YWJjZGVmZ2hpamtsbW5vcA==", Room: strptr("dev"), Seq: 7, TS: now,
	}

	// VALID signed envelope.
	validWire := signWire(t, agentPriv, agentPub, base)
	canonical, err := CanonicalizeEnvelope(base)
	require.NoError(t, err)

	fx := msgFixture{
		RootPubKey:      b64(rootPub),
		Now:             now,
		MaxStalenessSec: maxStale,
		Registry:        regEnvelope,
		PersistedEpoch:  persisted,
		Valid: msgValidCase{
			Envelope:       validWire,
			CanonicalBytes: b64(canonical.Bytes()),
		},
	}

	add := func(name, reason string, w wireFields) {
		fx.Reject = append(fx.Reject, msgRejectCase{Name: name, Reason: reason, Expect: "reject", Envelope: w})
	}

	// 1. tampered_body: valid sig, but the body is mutated after signing.
	tb := validWire
	tb.Body = "goodbye"
	add("tampered_body", "body mutated after signing; signature no longer matches", tb)

	// 2. wrong_key_epoch: sig is for key_epoch=1 but presented as epoch 2 (registry
	// has no epoch-2 binding -> epoch-mismatch default-deny).
	wke := validWire
	wke.KeyEpoch = 2
	add("wrong_key_epoch", "key_epoch presented as 2 but signed/bound at 1 (registry epoch mismatch)", wke)

	// 3. downgraded_alg_version: gate re-fires at verify (anti-downgrade).
	dav := base
	dav.AlgVersion = 2
	add("downgraded_alg_version", "alg_version downgraded to 2 (gate rejects at verify)", rawWire(dav, agentPub))

	// 4. epoch_above_2pow53: key_epoch above the JCS-safe ceiling.
	eak := base
	eak.KeyEpoch = MaxSafeInt + 1
	add("key_epoch_above_2pow53", "key_epoch above 2^53 (JCS-unsafe; typed reject)", rawWire(eak, agentPub))

	// 4b. seq above 2^53 (the other integer-range branch).
	eas := base
	eas.Seq = MaxSafeInt + 1
	add("seq_above_2pow53", "seq above 2^53 (JCS-unsafe; typed reject)", rawWire(eas, agentPub))

	// 5. non_nfc_body: a non-NFC body is rejected (no silent normalization).
	nnb := base
	nnb.Body = "é" // 'e' + COMBINING ACUTE (NFD), not NFC
	add("non_nfc_body", "body is NFD (not NFC); rejected, never silently normalized", rawWire(nnb, agentPub))

	// 6. both_room_and_to_agent.
	both := base
	both.ToAgent = strptr("workhorse")
	add("both_room_and_to_agent", "both room and to_agent present (exactly-one rule)", rawWire(both, agentPub))

	// 7. neither_room_nor_to_agent.
	neither := base
	neither.Room = nil
	add("neither_room_nor_to_agent", "neither room nor to_agent present (exactly-one rule)", rawWire(neither, agentPub))

	// 8. tampered_sig: one byte of a valid signature flipped.
	ts := validWire
	sigBytes, err := base64.StdEncoding.DecodeString(validWire.Sig)
	require.NoError(t, err)
	sigBytes[0] ^= 0xFF
	ts.Sig = b64(sigBytes)
	add("tampered_sig", "one signature byte flipped; does not verify", ts)

	// 9. wrong_signer_key: the envelope is signed by a DIFFERENT key than the one
	// bound for "mira" in the registry.
	wsk := signWire(t, wrongPriv, wrongPub, base)
	add("wrong_signer_key", "signed by a key not bound to from_agent in the registry", wsk)
	_ = wrongPub

	return fx
}

// --- Generator + guard (lockstep with the committed artifact) ---

// TestGenerate_MessageSigningFixture writes the deterministic fixture when -update
// is set. SSOT generator for the committed artifact.
func TestGenerate_MessageSigningFixture(t *testing.T) {
	if !*updateMsgFixture {
		t.Skip("run with -update to regenerate the committed fixture")
	}
	fx := buildMsgFixture(t)
	out, err := json.MarshalIndent(fx, "", "  ")
	require.NoError(t, err)
	out = append(out, '\n')
	require.NoError(t, os.MkdirAll(filepath.Dir(msgFixturePath), 0o755))
	require.NoError(t, os.WriteFile(msgFixturePath, out, 0o644)) //nolint:gosec // committed test fixture
	t.Logf("wrote %s (%d bytes, %d reject cases)", msgFixturePath, len(out), len(fx.Reject))
}

// TestMessageSigningFixture_MatchesGenerator fails if the committed fixture
// diverges byte-for-byte from the generator output. Regenerate with:
// go test ./internal/identity/envelope/ -run TestGenerate_MessageSigningFixture -update
func TestMessageSigningFixture_MatchesGenerator(t *testing.T) {
	if *updateMsgFixture {
		t.Skip("skipping match check during -update regeneration")
	}
	committed, err := os.ReadFile(msgFixturePath)
	require.NoError(t, err, "committed fixture must exist; run with -update to generate it")

	fx := buildMsgFixture(t)
	generated, err := json.MarshalIndent(fx, "", "  ")
	require.NoError(t, err)
	generated = append(generated, '\n')

	assert.Equal(t, string(committed), string(generated),
		"committed fixture diverges from generator output — regenerate with: "+
			"go test ./internal/identity/envelope/ -run TestGenerate_MessageSigningFixture -update")
}

// --- Acceptance proofs against the committed artifact ---

// loadMsgFixture loads the committed fixture and its verified registry.
func loadMsgFixture(t *testing.T) (msgFixture, registry.Registry) {
	t.Helper()
	data, err := os.ReadFile(msgFixturePath)
	require.NoError(t, err, "committed fixture must exist; run with -update to generate it")
	var fx msgFixture
	require.NoError(t, json.Unmarshal(data, &fx))

	rootPub, err := base64.StdEncoding.DecodeString(fx.RootPubKey)
	require.NoError(t, err)
	parsed, err := registry.ParseDistribution(fx.Registry)
	require.NoError(t, err)
	reg, _, err := registry.VerifyAndLoad(rootPub, parsed, fx.PersistedEpoch,
		time.Unix(fx.Now, 0), time.Duration(fx.MaxStalenessSec)*time.Second)
	require.NoError(t, err, "the fixture registry must load")
	return fx, reg
}

// TestMessageSigningFixture_ValidVerifies proves the valid signed envelope is
// ACCEPTED by VerifyEnvelope under the resolved binding, and that the recorded
// canonical bytes match this implementation's canonicalization.
func TestMessageSigningFixture_ValidVerifies(t *testing.T) {
	fx, reg := loadMsgFixture(t)

	f := fx.Valid.Envelope.toFields()
	canonical, err := CanonicalizeEnvelope(f)
	require.NoError(t, err)
	assert.Equal(t, fx.Valid.CanonicalBytes, b64(canonical.Bytes()),
		"recorded canonical bytes must match this implementation")

	sig, err := base64.StdEncoding.DecodeString(fx.Valid.Envelope.Sig)
	require.NoError(t, err)
	ok, err := VerifyEnvelope(reg, f, sig, time.Unix(fx.Now, 0))
	require.NoError(t, err)
	assert.True(t, ok, "the valid signed envelope must verify")
}

// TestMessageSigningFixture_AllRejectsAreRejected proves EVERY committed reject
// case fails — the gating acceptance contract the Rust verifier must match. A
// case is "rejected" if VerifyEnvelope returns an error OR (false, nil).
func TestMessageSigningFixture_AllRejectsAreRejected(t *testing.T) {
	fx, reg := loadMsgFixture(t)
	require.NotEmpty(t, fx.Reject, "fixture must carry reject cases")

	for _, rc := range fx.Reject {
		t.Run(rc.Name, func(t *testing.T) {
			assert.Equal(t, "reject", rc.Expect, "every negative case must declare expect=reject")
			f := rc.Envelope.toFields()
			sig, err := base64.StdEncoding.DecodeString(rc.Envelope.Sig)
			require.NoError(t, err)
			ok, verr := VerifyEnvelope(reg, f, sig, time.Unix(fx.Now, 0))
			assert.False(t, ok && verr == nil, "case %q (%s) MUST be rejected by VerifyEnvelope", rc.Name, rc.Reason)
		})
	}
}
