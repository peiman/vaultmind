package enrollment

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peiman/vaultmind/internal/identity"
)

// updateEnrollFixture, when set (-update), regenerates the committed
// enrollment-request vectors. Without it, the fixture is a stable artifact, only
// loaded + asserted.
var updateEnrollFixture = flag.Bool("update", false, "regenerate the committed enrollment-request fixture")

// enrollFixturePath is the committed cross-language enrollment-request acceptance
// fixture — the artifact workhorse's Rust enrollment daemon binds to.
const enrollFixturePath = "testdata/enrollment_request_vectors.json"

// --- Fixture JSON schema (the cross-language enrollment-request contract) ---

// enrollFixture is the top-level fixture: the enrolling agent's signing pubkey
// (so the Rust side knows which key the proof-of-possession is for), a single
// VALID self-signed enrollment request (with its canonical signed bytes so the
// Rust side cross-checks its canonicalization), and an array of named REJECT
// cases. Every byte is deterministic (fixed seeds).
type enrollFixture struct {
	SigningPubKey string             `json:"signing_pubkey"`
	Valid         enrollValidCase    `json:"valid"`
	Reject        []enrollRejectCase `json:"reject"`
}

// wireRequest is the JSON shape of the enrollment request's SIGNED-SUBSET fields
// plus the transport sig. transport_endpoint is an omitempty pointer so the
// absent value is OMITTED (never null) — mirroring the canonical contract.
type wireRequest struct {
	AlgVersion        int64   `json:"alg_version"`
	Created           int64   `json:"created"`
	DisplayName       string  `json:"display_name"`
	KeyEpoch          int64   `json:"key_epoch"`
	NetworkID         string  `json:"network_id"`
	Nonce             string  `json:"nonce"`
	PubKey            string  `json:"pubkey"`
	Slug              string  `json:"slug"`
	TransportEndpoint *string `json:"transport_endpoint,omitempty"`
	TransportPubKey   string  `json:"transport_pubkey"`
	// Sig is base64-std of the ed25519 signature over the canonical signed subset
	// (NOT part of the signed bytes).
	Sig string `json:"sig"`
}

// toFields converts the wire form back into Fields for the verify path.
func (w wireRequest) toFields() Fields {
	return Fields{
		AlgVersion:        w.AlgVersion,
		Created:           w.Created,
		DisplayName:       w.DisplayName,
		KeyEpoch:          w.KeyEpoch,
		NetworkID:         w.NetworkID,
		Nonce:             w.Nonce,
		PubKey:            w.PubKey,
		Slug:              w.Slug,
		TransportEndpoint: w.TransportEndpoint,
		TransportPubKey:   w.TransportPubKey,
	}
}

// enrollValidCase is a complete self-signed request plus its canonical signed
// bytes (base64) so a cross-language verifier can cross-check its own
// canonicalization.
type enrollValidCase struct {
	Request        wireRequest `json:"request"`
	CanonicalBytes string      `json:"canonical_signed_bytes"`
}

// enrollRejectCase is a negative case that MUST fail VerifyEnrollment (or, for
// the gate-only cases, CanonicalizeEnrollment).
type enrollRejectCase struct {
	Name    string      `json:"name"`
	Reason  string      `json:"reason"`
	Expect  string      `json:"expect"` // always "reject"
	Request wireRequest `json:"request"`
}

// signWire gates+canonicalizes f and signs it with priv, returning the wire form
// with sig populated. It signs DIRECTLY (not via the UDS client) because the
// fixture generator is deterministic and key-holding by design.
func signWire(t *testing.T, priv ed25519.PrivateKey, f Fields) wireRequest {
	t.Helper()
	canonical, err := CanonicalizeEnrollment(f)
	require.NoError(t, err)
	sig, err := identity.SignCanonical(priv, canonical)
	require.NoError(t, err)
	return toWire(f, b64(sig))
}

// rawWire builds a wire form for a gate-only reject case (no valid signature is
// needed because the gate fires before any signature check). A dummy sig keeps
// the structure complete.
func rawWire(f Fields) wireRequest {
	return toWire(f, b64(make([]byte, ed25519.SignatureSize)))
}

// toWire maps Fields + a base64 sig into the wire form.
func toWire(f Fields, sig string) wireRequest {
	return wireRequest{
		AlgVersion: f.AlgVersion, Created: f.Created, DisplayName: f.DisplayName,
		KeyEpoch: f.KeyEpoch, NetworkID: f.NetworkID, Nonce: f.Nonce,
		PubKey: f.PubKey, Slug: f.Slug, TransportEndpoint: f.TransportEndpoint,
		TransportPubKey: f.TransportPubKey, Sig: sig,
	}
}

// buildEnrollFixture deterministically constructs the entire fixture from fixed
// seeds.
func buildEnrollFixture(t *testing.T) enrollFixture {
	t.Helper()
	agentPub, agentPriv := fixedEd25519(t, 0xB2)
	_, wrongPriv := fixedEd25519(t, 0xEE)

	base := validFields(agentPub)

	// VALID self-signed request.
	validWire := signWire(t, agentPriv, base)
	canonical, err := CanonicalizeEnrollment(base)
	require.NoError(t, err)

	fx := enrollFixture{
		SigningPubKey: b64(agentPub),
		Valid: enrollValidCase{
			Request:        validWire,
			CanonicalBytes: b64(canonical.Bytes()),
		},
	}

	add := func(name, reason string, w wireRequest) {
		fx.Reject = append(fx.Reject, enrollRejectCase{Name: name, Reason: reason, Expect: "reject", Request: w})
	}

	// 1. tampered_display_name: valid sig, but display_name mutated after signing.
	td := validWire
	td.DisplayName = "Evil"
	add("tampered_display_name", "display_name mutated after signing; signature no longer matches", td)

	// 2. wrong_key_self_sig: signed by a DIFFERENT key than the pubkey field —
	// proof-of-possession fails.
	add("wrong_key_self_sig", "signed by a key other than the pubkey field (proof-of-possession fails)",
		signWire(t, wrongPriv, base))

	// 3. downgraded_alg_version: gate re-fires at verify (anti-downgrade).
	dav := base
	dav.AlgVersion = 2
	add("downgraded_alg_version", "alg_version downgraded to 2 (gate rejects at verify)", rawWire(dav))

	// 4. key_epoch_above_2pow53: key_epoch above the JCS-safe ceiling.
	eak := base
	eak.KeyEpoch = MaxSafeInt + 1
	add("key_epoch_above_2pow53", "key_epoch above 2^53 (JCS-unsafe; typed reject)", rawWire(eak))

	// 5. created_above_2pow53: created above the JCS-safe ceiling.
	eac := base
	eac.Created = MaxSafeInt + 1
	add("created_above_2pow53", "created above 2^53 (JCS-unsafe; typed reject)", rawWire(eac))

	// 6. non_nfc_display_name: a non-NFC display_name (no silent normalization).
	nnd := base
	nnd.DisplayName = "Mira é" // 'e' + COMBINING ACUTE (NFD), not NFC
	add("non_nfc_display_name", "display_name is NFD (not NFC); rejected, never silently normalized", rawWire(nnd))

	// 7. missing_transport_pubkey: empty transport_pubkey is rejected.
	mtp := base
	mtp.TransportPubKey = ""
	add("missing_transport_pubkey", "transport_pubkey absent/empty (required, signed) — reject", rawWire(mtp))

	// 8. bad_transport_pubkey_len: base64 of 16 bytes (not 32).
	btp := base
	btp.TransportPubKey = b64(make([]byte, 16))
	add("bad_transport_pubkey_len", "transport_pubkey is 16 bytes, not a 32-byte Curve25519 key", rawWire(btp))

	// 9. tampered_network_id: a cross-network replay breaks the self-sig.
	tn := validWire
	tn.NetworkID = "vmnet1:ffffffffffffffffffffffffffffffff"
	add("tampered_network_id", "network_id mutated (cross-network replay); signature no longer matches", tn)

	// 10. empty_nonce.
	en := base
	en.Nonce = ""
	add("empty_nonce", "nonce empty (gate reject)", rawWire(en))

	// 11. non_ascii_nonce.
	nan := base
	nan.Nonce = "nØnce"
	add("non_ascii_nonce", "nonce contains non-ASCII (gate reject)", rawWire(nan))

	return fx
}

// --- Generator + guard (lockstep with the committed artifact) ---

// TestGenerate_EnrollmentRequestFixture writes the deterministic fixture when
// -update is set. SSOT generator for the committed artifact.
func TestGenerate_EnrollmentRequestFixture(t *testing.T) {
	if !*updateEnrollFixture {
		t.Skip("run with -update to regenerate the committed fixture")
	}
	fx := buildEnrollFixture(t)
	out, err := json.MarshalIndent(fx, "", "  ")
	require.NoError(t, err)
	out = append(out, '\n')
	require.NoError(t, os.MkdirAll(filepath.Dir(enrollFixturePath), 0o755))
	require.NoError(t, os.WriteFile(enrollFixturePath, out, 0o644)) //nolint:gosec // committed test fixture
	t.Logf("wrote %s (%d bytes, %d reject cases)", enrollFixturePath, len(out), len(fx.Reject))
}

// TestEnrollmentRequestFixture_MatchesGenerator fails if the committed fixture
// diverges byte-for-byte from the generator output. Regenerate with:
// go test ./internal/identity/enrollment/ -run TestGenerate_EnrollmentRequestFixture -update
func TestEnrollmentRequestFixture_MatchesGenerator(t *testing.T) {
	if *updateEnrollFixture {
		t.Skip("skipping match check during -update regeneration")
	}
	committed, err := os.ReadFile(enrollFixturePath)
	require.NoError(t, err, "committed fixture must exist; run with -update to generate it")

	fx := buildEnrollFixture(t)
	generated, err := json.MarshalIndent(fx, "", "  ")
	require.NoError(t, err)
	generated = append(generated, '\n')

	assert.Equal(t, string(committed), string(generated),
		"committed fixture diverges from generator output — regenerate with: "+
			"go test ./internal/identity/enrollment/ -run TestGenerate_EnrollmentRequestFixture -update")
}

// --- Acceptance proofs against the committed artifact ---

// loadEnrollFixture loads the committed fixture.
func loadEnrollFixture(t *testing.T) enrollFixture {
	t.Helper()
	data, err := os.ReadFile(enrollFixturePath)
	require.NoError(t, err, "committed fixture must exist; run with -update to generate it")
	var fx enrollFixture
	require.NoError(t, json.Unmarshal(data, &fx))
	return fx
}

// TestEnrollmentRequestFixture_ValidVerifies proves the valid self-signed request
// is ACCEPTED by VerifyEnrollment (proof-of-possession), and that the recorded
// canonical bytes match this implementation's canonicalization.
func TestEnrollmentRequestFixture_ValidVerifies(t *testing.T) {
	fx := loadEnrollFixture(t)

	f := fx.Valid.Request.toFields()
	canonical, err := CanonicalizeEnrollment(f)
	require.NoError(t, err)
	assert.Equal(t, fx.Valid.CanonicalBytes, b64(canonical.Bytes()),
		"recorded canonical bytes must match this implementation")
	// The pubkey field IS the proof-of-possession key.
	assert.Equal(t, fx.SigningPubKey, f.PubKey, "the signing pubkey is the request's pubkey field")

	sig, err := base64.StdEncoding.DecodeString(fx.Valid.Request.Sig)
	require.NoError(t, err)
	ok, err := VerifyEnrollment(f, sig)
	require.NoError(t, err)
	assert.True(t, ok, "the valid self-signed request must verify")
}

// TestEnrollmentRequestFixture_AllRejectsAreRejected proves EVERY committed reject
// case fails — the gating acceptance contract the Rust verifier must match. A
// case is "rejected" if VerifyEnrollment returns an error OR (false, nil).
func TestEnrollmentRequestFixture_AllRejectsAreRejected(t *testing.T) {
	fx := loadEnrollFixture(t)
	require.NotEmpty(t, fx.Reject, "fixture must carry reject cases")

	for _, rc := range fx.Reject {
		t.Run(rc.Name, func(t *testing.T) {
			assert.Equal(t, "reject", rc.Expect, "every negative case must declare expect=reject")
			f := rc.Request.toFields()
			sig, err := base64.StdEncoding.DecodeString(rc.Request.Sig)
			require.NoError(t, err)
			ok, verr := VerifyEnrollment(f, sig)
			assert.False(t, ok && verr == nil, "case %q (%s) MUST be rejected by VerifyEnrollment", rc.Name, rc.Reason)
		})
	}
}
