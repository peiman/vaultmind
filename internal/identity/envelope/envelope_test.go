package envelope

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peiman/vaultmind/internal/identity/registry"
)

// fixedEd25519 returns a deterministic keypair from a one-byte seed filler.
func fixedEd25519(t *testing.T, fill byte) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = fill
	}
	priv := ed25519.NewKeyFromSeed(seed)
	return priv.Public().(ed25519.PublicKey), priv
}

// strptr is a small helper for the *string routing fields.
func strptr(s string) *string { return &s }

// fakeSigner signs locally with a held key, recording the bytes it was asked to
// sign so a test can assert the signature is over the canonical bytes. It proves
// the sign path goes through the SignerClient seam (keyless), never a file.
type fakeSigner struct {
	priv    ed25519.PrivateKey
	failErr error
	gotMsg  []byte
}

func (f *fakeSigner) Sign(canonicalBytes []byte) ([]byte, error) {
	if f.failErr != nil {
		return nil, f.failErr
	}
	f.gotMsg = append([]byte(nil), canonicalBytes...)
	return ed25519.Sign(f.priv, canonicalBytes), nil
}

// validFields returns a well-formed room-addressed envelope for the agent.
func validFields() Fields {
	return Fields{
		AlgVersion: AlgVersion,
		Body:       "hello ⭐",
		FromAgent:  "mira",
		KeyEpoch:   1,
		Nonce:      "YWJjZGVmZ2hpamtsbW5vcA==",
		Room:       strptr("dev"),
		Seq:        7,
		TS:         2_000_000,
	}
}

// regWith builds a single-binding registry for slug at key_epoch=1 holding pub.
func regWith(t *testing.T, slug string, pub ed25519.PublicKey) registry.Registry {
	t.Helper()
	pk, err := registry.NewPublicKey(pub)
	require.NoError(t, err)
	return registry.Registry{
		Epoch: 1, ValidFrom: 0, ValidUntil: 1 << 40,
		Agents: []registry.AgentBinding{{
			Slug: slug, DisplayName: "Mira ⭐", PubKey: pk, KeyEpoch: 1,
			ValidFrom: 0, ValidUntil: 1 << 40,
		}},
	}
}

// TestSignThenVerify_RoundTrips: the keyless signer signs the canonical subset
// and VerifyEnvelope accepts it under the registry binding.
func TestSignThenVerify_RoundTrips(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xB2)
	fake := &fakeSigner{priv: priv}
	f := validFields()

	res, err := SignEnvelope(fake, f, pub)
	require.NoError(t, err)
	assert.Equal(t, 1, res.KeyEpoch)
	assert.NotEmpty(t, res.Sig)
	assert.NotEmpty(t, res.FromPubKey)

	// The signer was handed EXACTLY the canonical signed bytes.
	canonical, err := CanonicalizeEnvelope(f)
	require.NoError(t, err)
	assert.Equal(t, canonical.Bytes(), fake.gotMsg, "signer must sign the canonical signed subset")

	sig := mustB64(t, res.Sig)
	reg := regWith(t, f.FromAgent, pub)
	ok, err := VerifyEnvelope(reg, f, sig, time.Unix(2_000_000, 0))
	require.NoError(t, err)
	assert.True(t, ok, "the round-tripped envelope must verify")
}

// TestVerify_ToAgentRoute: a to_agent-addressed envelope (the other branch of
// the exactly-one routing rule) also round-trips.
func TestVerify_ToAgentRoute(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xC3)
	fake := &fakeSigner{priv: priv}
	f := validFields()
	f.Room = nil
	f.ToAgent = strptr("workhorse")

	res, err := SignEnvelope(fake, f, pub)
	require.NoError(t, err)

	reg := regWith(t, f.FromAgent, pub)
	ok, err := VerifyEnvelope(reg, f, mustB64(t, res.Sig), time.Unix(2_000_000, 0))
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestCanonicalize_OmitsAbsentRoutingKey proves the OMITTED routing key is absent
// from the canonical bytes (absent != null is load-bearing in JCS).
func TestCanonicalize_OmitsAbsentRoutingKey(t *testing.T) {
	f := validFields() // Room set, ToAgent nil
	canonical, err := CanonicalizeEnvelope(f)
	require.NoError(t, err)
	s := string(canonical.Bytes())
	assert.Contains(t, s, `"room":"dev"`)
	assert.NotContains(t, s, "to_agent", "absent routing key must be OMITTED, never null")
	assert.NotContains(t, s, "null")
	// from_pubkey and transport fields are never in the signed bytes.
	assert.NotContains(t, s, "from_pubkey")
	assert.NotContains(t, s, "sig")
}

// TestSignEnvelope_FailsClosed_OnSignerError: a signer error surfaces, no sig.
func TestSignEnvelope_FailsClosed_OnSignerError(t *testing.T) {
	pub, _ := fixedEd25519(t, 0x01)
	fake := &fakeSigner{failErr: errors.New("signer unreachable")}
	_, err := SignEnvelope(fake, validFields(), pub)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signer unreachable")
}

// TestSignEnvelope_FailsClosed_OnGate: a gate failure short-circuits before the
// signer is ever called.
func TestSignEnvelope_FailsClosed_OnGate(t *testing.T) {
	pub, priv := fixedEd25519(t, 0x01)
	fake := &fakeSigner{priv: priv}
	f := validFields()
	f.AlgVersion = 2 // downgrade
	_, err := SignEnvelope(fake, f, pub)
	require.Error(t, err)
	assert.Nil(t, fake.gotMsg, "signer must not be called when a gate fails")
}

// TestGates_Reject covers every typed pre-sign reject branch in CanonicalizeEnvelope.
func TestGates_Reject(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Fields)
		errMsg string
	}{
		{"downgraded_alg_version", func(f *Fields) { f.AlgVersion = 2 }, ErrAlgVersion},
		{"alg_version_zero", func(f *Fields) { f.AlgVersion = 0 }, ErrAlgVersion},
		{"key_epoch_zero", func(f *Fields) { f.KeyEpoch = 0 }, ErrKeyEpochRange},
		{"key_epoch_negative", func(f *Fields) { f.KeyEpoch = -1 }, ErrKeyEpochRange},
		{"key_epoch_above_2pow53", func(f *Fields) { f.KeyEpoch = int(MaxSafeInt) + 1 }, ErrKeyEpochRange},
		{"seq_negative", func(f *Fields) { f.Seq = -1 }, ErrIntRange},
		{"seq_above_2pow53", func(f *Fields) { f.Seq = int(MaxSafeInt) + 1 }, ErrIntRange},
		{"ts_negative", func(f *Fields) { f.TS = -1 }, ErrIntRange},
		{"ts_above_2pow53", func(f *Fields) { f.TS = MaxSafeInt + 1 }, ErrIntRange},
		{"non_nfc_body", func(f *Fields) { f.Body = "é" }, ErrBodyNotNFC},
		{"invalid_utf8_body", func(f *Fields) { f.Body = string([]byte{0xff, 0xfe}) }, ErrBodyUTF8},
		{"empty_from_agent", func(f *Fields) { f.FromAgent = "" }, ErrFromAgentEmpty},
		{"empty_nonce", func(f *Fields) { f.Nonce = "" }, ErrNonceEmpty},
		{"non_ascii_nonce", func(f *Fields) { f.Nonce = "nØnce" }, ErrNonceASCII},
		{"both_room_and_to_agent", func(f *Fields) { f.ToAgent = strptr("workhorse") }, ErrRoutingExactlyOne},
		{"neither_room_nor_to_agent", func(f *Fields) { f.Room = nil }, ErrRoutingExactlyOne},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := validFields()
			tc.mutate(&f)
			_, err := CanonicalizeEnvelope(f)
			require.Error(t, err)
			assert.EqualError(t, err, tc.errMsg)
		})
	}
}

// TestVerifyEnvelope_TamperedBody: flipping the body after signing breaks verify
// (honest non-match -> (false, nil)).
func TestVerifyEnvelope_TamperedBody(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xB2)
	fake := &fakeSigner{priv: priv}
	f := validFields()
	res, err := SignEnvelope(fake, f, pub)
	require.NoError(t, err)

	reg := regWith(t, f.FromAgent, pub)
	tampered := f
	tampered.Body = "goodbye"
	ok, err := VerifyEnvelope(reg, tampered, mustB64(t, res.Sig), time.Unix(2_000_000, 0))
	require.NoError(t, err)
	assert.False(t, ok, "a tampered body must not verify")
}

// TestVerifyEnvelope_WrongKeyEpoch: sig made for epoch 1 but presented as epoch 2
// -> registry epoch-mismatch default-deny (typed error).
func TestVerifyEnvelope_WrongKeyEpoch(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xB2)
	fake := &fakeSigner{priv: priv}
	f := validFields()
	res, err := SignEnvelope(fake, f, pub)
	require.NoError(t, err)

	reg := regWith(t, f.FromAgent, pub) // only key_epoch 1 exists
	mismatch := f
	mismatch.KeyEpoch = 2
	ok, err := VerifyEnvelope(reg, mismatch, mustB64(t, res.Sig), time.Unix(2_000_000, 0))
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), registry.ErrKeyEpochMismatch)
}

// TestVerifyEnvelope_WrongSigner: a signature by a DIFFERENT key fails to verify
// under the registered binding (honest non-match).
func TestVerifyEnvelope_WrongSigner(t *testing.T) {
	regPub, _ := fixedEd25519(t, 0xB2)
	_, otherPriv := fixedEd25519(t, 0xEE)
	other := &fakeSigner{priv: otherPriv}
	f := validFields()
	res, err := SignEnvelope(other, f, regPub)
	require.NoError(t, err)

	reg := regWith(t, f.FromAgent, regPub) // binding holds regPub, not otherPriv's pub
	ok, err := VerifyEnvelope(reg, f, mustB64(t, res.Sig), time.Unix(2_000_000, 0))
	require.NoError(t, err)
	assert.False(t, ok, "a signature by a non-bound key must not verify")
}

// TestVerifyEnvelope_UnknownSigner: from_agent not in the registry -> default-deny.
func TestVerifyEnvelope_UnknownSigner(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xB2)
	fake := &fakeSigner{priv: priv}
	f := validFields()
	res, err := SignEnvelope(fake, f, pub)
	require.NoError(t, err)

	reg := regWith(t, "someone-else", pub)
	ok, err := VerifyEnvelope(reg, f, mustB64(t, res.Sig), time.Unix(2_000_000, 0))
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), registry.ErrUnknownSlug)
}

// TestVerifyEnvelope_BadSigLen: a wrong-length signature is a typed reject before
// the registry is consulted.
func TestVerifyEnvelope_BadSigLen(t *testing.T) {
	pub, _ := fixedEd25519(t, 0xB2)
	f := validFields()
	reg := regWith(t, f.FromAgent, pub)
	ok, err := VerifyEnvelope(reg, f, []byte{0x01, 0x02}, time.Unix(2_000_000, 0))
	require.Error(t, err)
	assert.False(t, ok)
	assert.EqualError(t, err, errSigBadLen)
}

// TestVerifyEnvelope_GateReSurfacedAtVerify: a tampered alg_version is caught at
// verify (the gate runs again, not just at sign).
func TestVerifyEnvelope_GateReSurfacedAtVerify(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xB2)
	fake := &fakeSigner{priv: priv}
	f := validFields()
	res, err := SignEnvelope(fake, f, pub)
	require.NoError(t, err)

	reg := regWith(t, f.FromAgent, pub)
	down := f
	down.AlgVersion = 2
	ok, err := VerifyEnvelope(reg, down, mustB64(t, res.Sig), time.Unix(2_000_000, 0))
	require.Error(t, err)
	assert.False(t, ok)
	assert.EqualError(t, err, ErrAlgVersion)
}

func mustB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	require.NoError(t, err)
	return b
}
