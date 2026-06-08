package registry

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"flag"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// updateFixture, when set (-update), regenerates the committed cross-language
// vectors file. Without it, the fixture is treated as a stable artifact and only
// loaded/asserted.
var updateFixture = flag.Bool("update", false, "regenerate the committed cross-language fixture")

// fixturePath is the committed cross-language acceptance fixture — the artifact
// workhorse's Rust verifier binds to.
const fixturePath = "testdata/cross_language_vectors.json"

// --- Fixture JSON schema (the cross-language acceptance contract) ---

// crossLangFixture is the top-level fixture: a pinned root pubkey, a reference
// time + staleness bound, a single VALID distribution case, and an array of
// named REJECT cases. Every byte is deterministic (fixed seeds).
type crossLangFixture struct {
	RootPubKey      string       `json:"root_pubkey"`
	Now             int64        `json:"now"`
	MaxStalenessSec int64        `json:"max_staleness_secs"`
	Valid           validCase    `json:"valid"`
	Reject          []rejectCase `json:"reject"`
}

// validCase is a complete envelope that loads, plus the expected resolved
// binding for a slug and a sample message the resolved key verifies.
type validCase struct {
	Envelope       json.RawMessage `json:"envelope"`
	PersistedEpoch int             `json:"persisted_highest_epoch"`
	ExpectSlug     string          `json:"expect_slug"`
	ExpectKeyEpoch int             `json:"expect_key_epoch"`
	ExpectPubKey   string          `json:"expect_pubkey"`
	Message        messageCase     `json:"message"`
}

// messageCase is a {slug, key_epoch, canonical message bytes, signature} sample
// that VerifyMessage accepts under the resolved binding.
type messageCase struct {
	Slug             string `json:"slug"`
	KeyEpoch         int    `json:"key_epoch"`
	CanonicalMessage string `json:"canonical_message_bytes"`
	Signature        string `json:"signature"`
}

// rejectCase is a negative case: an envelope (or mutated field) that, under the
// supplied (or top-level) root pubkey/now/persisted epoch, MUST be rejected by
// the Go (and Rust) verify path. RootPubKey/Now/PersistedEpoch override the
// top-level defaults when non-zero/non-empty.
type rejectCase struct {
	Name           string          `json:"name"`
	Reason         string          `json:"reason"`
	Expect         string          `json:"expect"` // always "reject"
	Envelope       json.RawMessage `json:"envelope"`
	RootPubKey     string          `json:"root_pubkey,omitempty"`
	Now            *int64          `json:"now,omitempty"`
	PersistedEpoch *int            `json:"persisted_highest_epoch,omitempty"`
	// Message, when set, is a per-case message whose VerifyMessage must reject
	// (non-canonical S, revoked binding, etc.) AFTER the envelope itself loads.
	Message *messageCase `json:"message,omitempty"`
}

// --- Deterministic generation helpers ---

const (
	// ed25519 group order L = 2^252 + 27742317777372353535851937790883648493.
	groupOrderHex = "1000000000000000000000000000000014def9dea2f79cd65812631a5cf5d3ed"
	// smallOrderPubKeyHex is the all-zero (identity) small-order point: a
	// universal-forgery vector that VerifyCanonical/Rust verify_strict reject.
	smallOrderPubKeyHex = "0000000000000000000000000000000000000000000000000000000000000000"
)

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// genEnvelope signs reg with rootPriv and returns the distribution-envelope JSON.
func genEnvelope(t *testing.T, rootPriv ed25519.PrivateKey, reg Registry) json.RawMessage {
	t.Helper()
	env, err := SignRegistry(rootPriv, reg)
	require.NoError(t, err)
	out, err := MarshalDistribution(env)
	require.NoError(t, err)
	return out
}

// signCanonicalMsg signs an arbitrary canonical message with priv (used for the
// per-message samples). The message bytes are the EXACT bytes the binding key
// verifies — not schema-gated, mirroring VerifyMessage's CanonicalBytes input.
func signCanonicalMsg(t *testing.T, priv ed25519.PrivateKey, msg []byte) []byte {
	t.Helper()
	sig, err := identity.SignCanonical(priv, identity.CanonicalBytesFromTrusted(msg))
	require.NoError(t, err)
	return sig
}

// nonCanonicalS takes a valid (R||S) signature and returns R||(S+L). S+L ≡ S
// (mod L) so it is the "same" scalar, but its 32-byte little-endian encoding is
// non-canonical (>= L), which ZIP-215 / verify_strict reject (signature
// malleability defense).
func nonCanonicalS(t *testing.T, sig []byte) []byte {
	t.Helper()
	require.Len(t, sig, ed25519.SignatureSize)
	order := new(big.Int)
	order.SetString(groupOrderHex, 16)

	// S is the high 32 bytes, little-endian.
	sLE := make([]byte, 32)
	copy(sLE, sig[32:])
	sBE := reverse(sLE)
	s := new(big.Int).SetBytes(sBE)
	s.Add(s, order) // S + L, still < 2^256 for honest signatures

	outBE := s.Bytes()
	// Left-pad to 32 bytes big-endian, then reverse to little-endian.
	padded := make([]byte, 32)
	copy(padded[32-len(outBE):], outBE)
	sLEout := reverse(padded)

	out := make([]byte, ed25519.SignatureSize)
	copy(out, sig[:32])
	copy(out[32:], sLEout)
	return out
}

func reverse(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[len(b)-1-i] = b[i]
	}
	return out
}

// buildFixture deterministically constructs the entire fixture from fixed seeds.
func buildFixture(t *testing.T) crossLangFixture {
	t.Helper()
	rootPub, rootPriv := fixedEd25519(t, 0xA1)
	agentPub, agentPriv := fixedEd25519(t, 0xB2)
	smallPub, err := ed25519HexToPub(smallOrderPubKeyHex)
	require.NoError(t, err)

	apk, err := NewPublicKey(agentPub)
	require.NoError(t, err)

	const (
		now          int64 = 2_000_000
		maxStale     int64 = 86_400 // 1 day
		regFrom      int64 = 1_990_000
		regUntil     int64 = 3_000_000
		bindFrom     int64 = 1_990_000
		bindUntil    int64 = 3_000_000
		validEpoch         = 10
		validKeyEp         = 1
		persistedLow       = 9
	)

	baseBinding := AgentBinding{
		Slug: "mira", DisplayName: "Mira ⭐", PubKey: apk, KeyEpoch: validKeyEp,
		ValidFrom: bindFrom, ValidUntil: bindUntil, AuthorizedOriginDaemons: []string{"daemon-1"},
	}
	baseReg := Registry{
		Epoch: validEpoch, ValidFrom: regFrom, ValidUntil: regUntil,
		Agents: []AgentBinding{baseBinding},
	}

	// VALID envelope + a real signed message under the agent key.
	validEnv := genEnvelope(t, rootPriv, baseReg)
	msgBytes := []byte(`{"body":"hello","from":"mira"}`)
	msgSig := signCanonicalMsg(t, agentPriv, msgBytes)

	fx := crossLangFixture{
		RootPubKey:      b64(rootPub),
		Now:             now,
		MaxStalenessSec: maxStale,
		Valid: validCase{
			Envelope:       validEnv,
			PersistedEpoch: persistedLow,
			ExpectSlug:     "mira",
			ExpectKeyEpoch: validKeyEp,
			ExpectPubKey:   b64(agentPub),
			Message: messageCase{
				Slug: "mira", KeyEpoch: validKeyEp,
				CanonicalMessage: b64(msgBytes), Signature: b64(msgSig),
			},
		},
	}

	// --- REJECT cases ---
	var rejects []rejectCase
	add := func(name, reason string, env json.RawMessage) {
		rejects = append(rejects, rejectCase{Name: name, Reason: reason, Expect: "reject", Envelope: env})
	}

	// 1. Small-order ROOT pubkey: the valid envelope, but verified against a
	// small-order pinned root key (verify_strict must reject the root key itself).
	rejects = append(rejects, rejectCase{
		Name: "small_order_root_pubkey", Reason: "pinned root pubkey is small-order (universal-forgery vector)",
		Expect: "reject", Envelope: validEnv, RootPubKey: b64(smallPub),
	})

	// 2. Small-order AGENT pubkey in a binding: a registry whose binding carries a
	// small-order agent key. We must sign it with a wire-level injection because
	// SignRegistry/NewPublicKey reject small-order keys; mutate the canonical
	// bytes, re-sign with the root, then the agent-key validation (fromWire)
	// rejects at load.
	rejects = append(rejects, genSmallOrderAgent(t, rootPriv, baseReg, smallPub))

	// 3. Future valid_from at REGISTRY level.
	futureReg := baseReg
	futureReg.ValidFrom = now + 100_000
	futureReg.Epoch = validEpoch
	add("future_valid_from_registry", "registry valid_from is in the future (not yet fresh)",
		genEnvelope(t, rootPriv, futureReg))

	// 4. Future valid_from at BINDING level (registry fresh, binding not yet valid).
	futureBindReg := baseReg
	fb := baseBinding
	fb.ValidFrom = now + 100_000
	futureBindReg.Agents = []AgentBinding{fb}
	rejects = append(rejects, rejectCase{
		Name: "future_valid_from_binding", Reason: "binding valid_from is in the future (resolve/verify must reject)",
		Expect: "reject", Envelope: genEnvelope(t, rootPriv, futureBindReg),
		Message: &messageCase{Slug: "mira", KeyEpoch: validKeyEp, CanonicalMessage: b64(msgBytes), Signature: b64(msgSig)},
	})

	// 5. Rollback: epoch <= persisted highest. Use persisted == validEpoch so the
	// valid envelope's epoch is not strictly greater.
	pe := validEpoch
	rejects = append(rejects, rejectCase{
		Name: "rollback_epoch_at_or_below_persisted", Reason: "registry epoch <= persisted highest seen (anti-rollback)",
		Expect: "reject", Envelope: validEnv, PersistedEpoch: &pe,
	})

	// 6a. Stale: now past valid_until.
	pastNow := regUntil + 1
	rejects = append(rejects, rejectCase{
		Name: "stale_past_valid_until", Reason: "now is past registry valid_until (fail closed)",
		Expect: "reject", Envelope: validEnv, Now: &pastNow,
	})
	// 6b. Stale: now - valid_from exceeds max_staleness (within valid_until).
	staleNow := regFrom + maxStale + 1
	rejects = append(rejects, rejectCase{
		Name: "stale_beyond_max_staleness", Reason: "now - valid_from exceeds max_staleness_secs (fail closed)",
		Expect: "reject", Envelope: validEnv, Now: &staleNow,
	})

	// 7. Revoked binding: resolve/verify must reject.
	revAt := int(bindFrom + 1000)
	revReg := baseReg
	rb := baseBinding
	rb.RevokedAt = &revAt
	revReg.Agents = []AgentBinding{rb}
	rejects = append(rejects, rejectCase{
		Name: "revoked_binding", Reason: "binding is revoked (resolve/verify must reject)",
		Expect: "reject", Envelope: genEnvelope(t, rootPriv, revReg),
		Message: &messageCase{Slug: "mira", KeyEpoch: validKeyEp, CanonicalMessage: b64(msgBytes), Signature: b64(msgSig)},
	})

	// 8. Tampered registry body: valid envelope with one byte of the registry flipped
	// so root_sig no longer matches.
	add("tampered_registry_body", "registry bytes mutated; root_sig no longer matches",
		tamperEnvelope(t, validEnv))

	// 9. Non-canonical S in the MESSAGE signature: envelope loads, but the
	// per-message sample carries S+L which verify_strict rejects.
	rejects = append(rejects, rejectCase{
		Name: "non_canonical_s_message_sig", Reason: "message signature has non-canonical S (S >= L); verify_strict rejects",
		Expect: "reject", Envelope: validEnv,
		Message: &messageCase{Slug: "mira", KeyEpoch: validKeyEp, CanonicalMessage: b64(msgBytes), Signature: b64(nonCanonicalS(t, msgSig))},
	})

	fx.Reject = append(rejects, fx.Reject...)
	return fx
}

// ed25519HexToPub decodes a hex pubkey string into an ed25519.PublicKey.
func ed25519HexToPub(h string) (ed25519.PublicKey, error) {
	raw := make([]byte, ed25519.PublicKeySize)
	_, err := hexDecode(h, raw)
	if err != nil {
		return nil, err
	}
	return ed25519.PublicKey(raw), nil
}

// hexDecode decodes hex string h into dst, returning the byte count.
func hexDecode(h string, dst []byte) (int, error) {
	b, err := hexBytes(h)
	if err != nil {
		return 0, err
	}
	return copy(dst, b), nil
}

func hexBytes(h string) ([]byte, error) {
	out := make([]byte, len(h)/2)
	for i := 0; i < len(out); i++ {
		var hi, lo byte
		var err error
		if hi, err = hexNibble(h[2*i]); err != nil {
			return nil, err
		}
		if lo, err = hexNibble(h[2*i+1]); err != nil {
			return nil, err
		}
		out[i] = hi<<4 | lo
	}
	return out, nil
}

func hexNibble(c byte) (byte, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	}
	return 0, assert.AnError
}

// genSmallOrderAgent crafts an envelope whose binding carries a small-order
// agent pubkey, re-signed by the legitimate root so root_sig verifies but the
// agent-key validation (fromWire) rejects at load.
func genSmallOrderAgent(t *testing.T, rootPriv ed25519.PrivateKey, base Registry, smallPub ed25519.PublicKey) rejectCase {
	t.Helper()
	// Build the wire registry directly, inject the small-order pubkey, canonicalize,
	// and sign with the real root key.
	w := toWire(base)
	w.Agents[0].PubKey = b64(smallPub)
	raw, err := json.Marshal(w)
	require.NoError(t, err)
	canonical, err := identity.Canonicalize(raw)
	require.NoError(t, err)
	sig, err := identity.SignCanonical(rootPriv, canonical)
	require.NoError(t, err)
	env := SignedRegistry{Registry: canonical.Bytes(), RootSig: sig, RootKeyEpoch: 0}
	out, err := MarshalDistribution(env)
	require.NoError(t, err)
	return rejectCase{
		Name: "small_order_agent_pubkey", Reason: "binding agent pubkey is small-order; load rejects the binding",
		Expect: "reject", Envelope: out,
	}
}

// tamperEnvelope parses the envelope, flips one byte of the registry payload,
// re-marshals (keeping the now-mismatched root_sig), so root_sig fails to verify.
func tamperEnvelope(t *testing.T, env json.RawMessage) json.RawMessage {
	t.Helper()
	parsed, err := ParseDistribution(env)
	require.NoError(t, err)
	tampered := make([]byte, len(parsed.Registry))
	copy(tampered, parsed.Registry)
	tampered[len(tampered)/2] ^= 0xFF
	out, err := MarshalDistribution(SignedRegistry{Registry: tampered, RootSig: parsed.RootSig, RootKeyEpoch: parsed.RootKeyEpoch})
	require.NoError(t, err)
	return out
}

// TestGenerate_CrossLanguageFixture writes the deterministic fixture to disk
// when -update is set. It is the SSOT generator for the committed artifact.
func TestGenerate_CrossLanguageFixture(t *testing.T) {
	if !*updateFixture {
		t.Skip("run with -update to regenerate the committed fixture")
	}
	fx := buildFixture(t)
	out, err := json.MarshalIndent(fx, "", "  ")
	require.NoError(t, err)
	out = append(out, '\n')
	require.NoError(t, os.MkdirAll(filepath.Dir(fixturePath), 0o755))
	require.NoError(t, os.WriteFile(fixturePath, out, 0o644))
	t.Logf("wrote %s (%d bytes, %d reject cases)", fixturePath, len(out), len(fx.Reject))
}

// TestCrossLanguageFixture_MatchesGenerator is a CI-guard that fails if the
// committed testdata/cross_language_vectors.json diverges byte-for-byte from
// what buildFixture() + json.MarshalIndent currently generates. If this test
// fails, regenerate with: go test ./internal/identity/registry/ -run
// TestGenerate_CrossLanguageFixture -update
func TestCrossLanguageFixture_MatchesGenerator(t *testing.T) {
	if *updateFixture {
		t.Skip("skipping match check during -update regeneration")
	}
	committed, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "committed fixture must exist; run with -update to generate it")

	fx := buildFixture(t)
	generated, err := json.MarshalIndent(fx, "", "  ")
	require.NoError(t, err)
	generated = append(generated, '\n')

	assert.Equal(t, string(committed), string(generated),
		"committed fixture diverges from generator output — regenerate with: "+
			"go test ./internal/identity/registry/ -run TestGenerate_CrossLanguageFixture -update")
}

// TestCrossLanguageFixture_ValidLoadsAndVerifies proves, against the COMMITTED
// artifact, that the valid case loads + resolves + verifies — so the Go side
// guarantees correctness before Rust consumes the bytes.
func TestCrossLanguageFixture_ValidLoadsAndVerifies(t *testing.T) {
	fx := loadFixture(t)
	rootPub := mustB64Pub(t, fx.RootPubKey)

	parsed, err := ParseDistribution(fx.Valid.Envelope)
	require.NoError(t, err)

	reg, _, err := VerifyAndLoad(rootPub, parsed, fx.Valid.PersistedEpoch,
		time.Unix(fx.Now, 0), time.Duration(fx.MaxStalenessSec)*time.Second)
	require.NoError(t, err, "the valid envelope must load")

	b, err := Resolve(reg, fx.Valid.ExpectSlug, time.Unix(fx.Now, 0))
	require.NoError(t, err, "the valid slug must resolve")
	assert.Equal(t, fx.Valid.ExpectKeyEpoch, b.KeyEpoch)
	assert.Equal(t, fx.Valid.ExpectPubKey, b64(b.PubKey.Bytes()), "resolved binding pubkey must match expected")

	msg, err := base64.StdEncoding.DecodeString(fx.Valid.Message.CanonicalMessage)
	require.NoError(t, err)
	sig, err := base64.StdEncoding.DecodeString(fx.Valid.Message.Signature)
	require.NoError(t, err)
	ok, err := VerifyMessage(reg, fx.Valid.Message.Slug, fx.Valid.Message.KeyEpoch,
		identity.CanonicalBytesFromTrusted(msg), sig, time.Unix(fx.Now, 0))
	require.NoError(t, err)
	assert.True(t, ok, "the valid message sample must verify under the resolved binding")
}

// TestCrossLanguageFixture_AllRejectsAreRejected proves EVERY committed reject
// case is rejected by ParseDistribution/VerifyAndLoad/Resolve/VerifyMessage —
// the gating acceptance contract the Rust verifier must match.
func TestCrossLanguageFixture_AllRejectsAreRejected(t *testing.T) {
	fx := loadFixture(t)
	require.NotEmpty(t, fx.Reject, "fixture must carry reject cases")

	for _, rc := range fx.Reject {
		t.Run(rc.Name, func(t *testing.T) {
			assert.Equal(t, "reject", rc.Expect, "every negative case must declare expect=reject")
			rootPub := mustB64Pub(t, firstNonEmpty(rc.RootPubKey, fx.RootPubKey))
			now := fx.Now
			if rc.Now != nil {
				now = *rc.Now
			}
			persisted := fx.Valid.PersistedEpoch
			if rc.PersistedEpoch != nil {
				persisted = *rc.PersistedEpoch
			}

			staleness := time.Duration(fx.MaxStalenessSec) * time.Second
			rejected := caseIsRejected(t, rc, rootPub, persisted, now, staleness)
			assert.True(t, rejected, "case %q (%s) MUST be rejected somewhere in parse/load/resolve/verify", rc.Name, rc.Reason)
		})
	}
}

// caseIsRejected runs a reject case through the full consumer path and reports
// whether it is rejected at any stage: parse -> load -> (if a message) resolve +
// VerifyMessage. A case is "rejected" if any stage errors or VerifyMessage
// returns (false, _).
func caseIsRejected(t *testing.T, rc rejectCase, rootPub ed25519.PublicKey, persisted int, now int64, staleness time.Duration) bool {
	t.Helper()
	parsed, err := ParseDistribution(rc.Envelope)
	if err != nil {
		return true
	}
	reg, _, err := VerifyAndLoad(rootPub, parsed, persisted, time.Unix(now, 0), staleness)
	if err != nil {
		return true
	}
	if rc.Message == nil {
		// No message: the envelope itself was expected to be rejected at load,
		// but it loaded — that is NOT a rejection.
		return false
	}
	// Envelope loaded; the rejection must come from resolve/verify of the message.
	if _, rerr := Resolve(reg, rc.Message.Slug, time.Unix(now, 0)); rerr != nil {
		return true
	}
	msg, err := base64.StdEncoding.DecodeString(rc.Message.CanonicalMessage)
	require.NoError(t, err)
	sig, err := base64.StdEncoding.DecodeString(rc.Message.Signature)
	require.NoError(t, err)
	ok, verr := VerifyMessage(reg, rc.Message.Slug, rc.Message.KeyEpoch,
		identity.CanonicalBytesFromTrusted(msg), sig, time.Unix(now, 0))
	return verr != nil || !ok
}

// TestCrossLanguageFixture_MessageCasesLoadThenRejectAtMessage proves the
// message-layer reject cases (future-valid binding, revoked binding,
// non-canonical S) are NOT accidentally rejected at the envelope layer: their
// envelope must LOAD, and the rejection must come from resolve/VerifyMessage.
// This pins the failure stage so the Rust verifier binds to the same contract.
func TestCrossLanguageFixture_MessageCasesLoadThenRejectAtMessage(t *testing.T) {
	fx := loadFixture(t)
	rootPub := mustB64Pub(t, fx.RootPubKey)
	staleness := time.Duration(fx.MaxStalenessSec) * time.Second

	wantMessageLayer := map[string]bool{
		"future_valid_from_binding":   true,
		"revoked_binding":             true,
		"non_canonical_s_message_sig": true,
	}
	seen := map[string]bool{}
	for _, rc := range fx.Reject {
		if !wantMessageLayer[rc.Name] {
			continue
		}
		seen[rc.Name] = true
		require.NotNil(t, rc.Message, "case %q must carry a message sample", rc.Name)
		parsed, err := ParseDistribution(rc.Envelope)
		require.NoError(t, err, "case %q envelope must parse", rc.Name)
		now := fx.Now
		if rc.Now != nil {
			now = *rc.Now
		}
		// The envelope itself must LOAD (root sig + freshness + anti-rollback OK).
		_, _, err = VerifyAndLoad(rootPub, parsed, fx.Valid.PersistedEpoch, time.Unix(now, 0), staleness)
		require.NoError(t, err, "case %q envelope must load; rejection belongs to the message layer", rc.Name)
	}
	for name := range wantMessageLayer {
		assert.True(t, seen[name], "expected message-layer case %q in the fixture", name)
	}
}

// --- fixture load helpers ---

func loadFixture(t *testing.T) crossLangFixture {
	t.Helper()
	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "committed fixture must exist; run with -update to generate it")
	var fx crossLangFixture
	require.NoError(t, json.Unmarshal(data, &fx))
	return fx
}

func mustB64Pub(t *testing.T, s string) ed25519.PublicKey {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(s)
	require.NoError(t, err)
	require.Len(t, raw, ed25519.PublicKeySize)
	return ed25519.PublicKey(raw)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
