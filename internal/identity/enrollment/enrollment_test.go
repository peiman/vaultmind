package enrollment

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// strptr is a small helper for the *string transport_endpoint field.
func strptr(s string) *string { return &s }

// curve25519Key returns a deterministic 32-byte value used as the (length-only
// checked) WireGuard transport pubkey. It is NOT an ed25519 key — the contract
// only length-checks it.
func curve25519Key(fill byte) []byte {
	k := make([]byte, transportPubKeyLen)
	for i := range k {
		k[i] = fill
	}
	return k
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

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

// validFields returns a well-formed enrollment request for the agent whose
// identity pubkey is pub.
func validFields(pub ed25519.PublicKey) Fields {
	return Fields{
		AlgVersion:      AlgVersion,
		Created:         2_000_000,
		DisplayName:     "Mira ⭐",
		KeyEpoch:        1,
		NetworkID:       "vmnet1:0011223344556677889900aabbccddee",
		Nonce:           "YWJjZGVmZ2hpamtsbW5vcA==",
		PubKey:          b64(pub),
		Slug:            "mira",
		TransportPubKey: b64(curve25519Key(0x11)),
	}
}

// TestSignThenVerify_RoundTrips: the keyless signer signs the canonical subset
// and VerifyEnrollment (self-verify = proof-of-possession) accepts it.
func TestSignThenVerify_RoundTrips(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xB2)
	fake := &fakeSigner{priv: priv}
	f := validFields(pub)

	res, err := SignEnrollment(fake, f)
	require.NoError(t, err)
	assert.NotEmpty(t, res.Sig)

	// The signer was handed EXACTLY the canonical signed bytes.
	canonical, err := CanonicalizeEnrollment(f)
	require.NoError(t, err)
	assert.Equal(t, canonical.Bytes(), fake.gotMsg, "signer must sign the canonical signed subset")

	sig := mustB64(t, res.Sig)
	ok, err := VerifyEnrollment(f, sig)
	require.NoError(t, err)
	assert.True(t, ok, "the self-signed request must verify (proof-of-possession)")
}

// TestSignThenVerify_WithTransportEndpoint exercises the OPTIONAL
// transport_endpoint branch (present -> emitted into the signed bytes).
func TestSignThenVerify_WithTransportEndpoint(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xC3)
	fake := &fakeSigner{priv: priv}
	f := validFields(pub)
	f.TransportEndpoint = strptr("203.0.113.7:51820")

	res, err := SignEnrollment(fake, f)
	require.NoError(t, err)

	ok, err := VerifyEnrollment(f, mustB64(t, res.Sig))
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestCanonicalize_OmitsAbsentTransportEndpoint proves the OMITTED optional key
// is absent from the canonical bytes (absent != null is load-bearing in JCS),
// and the transport sig is never in the signed bytes.
func TestCanonicalize_OmitsAbsentTransportEndpoint(t *testing.T) {
	pub, _ := fixedEd25519(t, 0xB2)
	f := validFields(pub) // TransportEndpoint nil
	canonical, err := CanonicalizeEnrollment(f)
	require.NoError(t, err)
	s := string(canonical.Bytes())
	assert.NotContains(t, s, "transport_endpoint", "absent optional key must be OMITTED, never null")
	assert.NotContains(t, s, "null")
	assert.NotContains(t, s, `"sig"`, "the transport sig is never part of the signed bytes")
	// pubkey IS in the signed subset (proof-of-possession key).
	assert.Contains(t, s, FieldPubKey)
	assert.Contains(t, s, FieldTransportPubKey)
}

// TestCanonicalize_EmitsTransportEndpointWhenPresent proves the present optional
// key IS in the signed bytes.
func TestCanonicalize_EmitsTransportEndpointWhenPresent(t *testing.T) {
	pub, _ := fixedEd25519(t, 0xB2)
	f := validFields(pub)
	f.TransportEndpoint = strptr("203.0.113.7:51820")
	canonical, err := CanonicalizeEnrollment(f)
	require.NoError(t, err)
	assert.Contains(t, string(canonical.Bytes()), `"transport_endpoint":"203.0.113.7:51820"`)
}

// TestSignEnrollment_FailsClosed_OnSignerError: a signer error surfaces, no sig.
func TestSignEnrollment_FailsClosed_OnSignerError(t *testing.T) {
	pub, _ := fixedEd25519(t, 0x01)
	fake := &fakeSigner{failErr: errors.New("signer unreachable")}
	_, err := SignEnrollment(fake, validFields(pub))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signer unreachable")
}

// TestSignEnrollment_FailsClosed_OnGate: a gate failure short-circuits before the
// signer is ever called.
func TestSignEnrollment_FailsClosed_OnGate(t *testing.T) {
	pub, priv := fixedEd25519(t, 0x01)
	fake := &fakeSigner{priv: priv}
	f := validFields(pub)
	f.AlgVersion = 2 // downgrade
	_, err := SignEnrollment(fake, f)
	require.Error(t, err)
	assert.Nil(t, fake.gotMsg, "signer must not be called when a gate fails")
}

// TestGates_Reject covers every typed pre-sign reject branch in
// CanonicalizeEnrollment.
func TestGates_Reject(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Fields)
		errMsg string
	}{
		{"downgraded_alg_version", func(f *Fields) { f.AlgVersion = 2 }, ErrAlgVersion},
		{"alg_version_zero", func(f *Fields) { f.AlgVersion = 0 }, ErrAlgVersion},
		{"created_negative", func(f *Fields) { f.Created = -1 }, ErrIntRange},
		{"created_above_2pow53", func(f *Fields) { f.Created = MaxSafeInt + 1 }, ErrIntRange},
		{"key_epoch_zero", func(f *Fields) { f.KeyEpoch = 0 }, ErrKeyEpochRange},
		{"key_epoch_negative", func(f *Fields) { f.KeyEpoch = -1 }, ErrKeyEpochRange},
		{"key_epoch_above_2pow53", func(f *Fields) { f.KeyEpoch = MaxSafeInt + 1 }, ErrKeyEpochRange},
		{"non_nfc_display_name", func(f *Fields) { f.DisplayName = "é" }, ErrDisplayNameNotNFC},
		{"invalid_utf8_display_name", func(f *Fields) { f.DisplayName = string([]byte{0xff, 0xfe}) }, ErrDisplayNameUTF8},
		{"empty_nonce", func(f *Fields) { f.Nonce = "" }, ErrNonceEmpty},
		{"non_ascii_nonce", func(f *Fields) { f.Nonce = "nØnce" }, ErrNonceASCII},
		{"empty_slug", func(f *Fields) { f.Slug = "" }, ErrSlugEmpty},
		{"non_ascii_slug", func(f *Fields) { f.Slug = "mîra" }, ErrSlugASCII},
		{"empty_network_id", func(f *Fields) { f.NetworkID = "" }, ErrNetworkIDEmpty},
		{"empty_pubkey", func(f *Fields) { f.PubKey = "" }, ErrPubKey},
		{"bad_base64_pubkey", func(f *Fields) { f.PubKey = "not-base64!!!" }, ErrPubKey},
		{"small_order_pubkey", func(f *Fields) { f.PubKey = b64(make([]byte, ed25519.PublicKeySize)) }, ErrPubKey},
		{"empty_transport_pubkey", func(f *Fields) { f.TransportPubKey = "" }, ErrTransportPubKey},
		{"bad_base64_transport_pubkey", func(f *Fields) { f.TransportPubKey = "not-base64!!!" }, ErrTransportPubKey},
		{"bad_len_transport_pubkey", func(f *Fields) { f.TransportPubKey = b64(make([]byte, 16)) }, ErrTransportPubKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pub, _ := fixedEd25519(t, 0xB2)
			f := validFields(pub)
			tc.mutate(&f)
			_, err := CanonicalizeEnrollment(f)
			require.Error(t, err)
			assert.EqualError(t, err, tc.errMsg)
		})
	}
}

// TestVerifyEnrollment_TamperedDisplayName: mutating display_name after signing
// breaks self-verify (honest non-match -> (false, nil)).
func TestVerifyEnrollment_TamperedDisplayName(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xB2)
	fake := &fakeSigner{priv: priv}
	f := validFields(pub)
	res, err := SignEnrollment(fake, f)
	require.NoError(t, err)

	tampered := f
	tampered.DisplayName = "Evil"
	ok, err := VerifyEnrollment(tampered, mustB64(t, res.Sig))
	require.NoError(t, err)
	assert.False(t, ok, "a tampered display_name must not verify")
}

// TestVerifyEnrollment_WrongKeySelfSig: the request is signed by a DIFFERENT key
// than the pubkey field — proof-of-possession fails (honest non-match).
func TestVerifyEnrollment_WrongKeySelfSig(t *testing.T) {
	pub, _ := fixedEd25519(t, 0xB2)
	_, wrongPriv := fixedEd25519(t, 0xEE)
	wrong := &fakeSigner{priv: wrongPriv}
	f := validFields(pub) // pubkey field is pub, but we sign with wrongPriv
	res, err := SignEnrollment(wrong, f)
	require.NoError(t, err)

	ok, err := VerifyEnrollment(f, mustB64(t, res.Sig))
	require.NoError(t, err)
	assert.False(t, ok, "a sig by a key other than the pubkey field must not verify")
}

// TestVerifyEnrollment_BadSigLen: a wrong-length signature is a typed reject.
func TestVerifyEnrollment_BadSigLen(t *testing.T) {
	pub, _ := fixedEd25519(t, 0xB2)
	f := validFields(pub)
	ok, err := VerifyEnrollment(f, []byte{0x01, 0x02})
	require.Error(t, err)
	assert.False(t, ok)
	assert.EqualError(t, err, errSigBadLen)
}

// TestVerifyEnrollment_GateReSurfacedAtVerify: a tampered alg_version is caught at
// verify (the gate runs again, not just at sign).
func TestVerifyEnrollment_GateReSurfacedAtVerify(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xB2)
	fake := &fakeSigner{priv: priv}
	f := validFields(pub)
	res, err := SignEnrollment(fake, f)
	require.NoError(t, err)

	down := f
	down.AlgVersion = 2
	ok, err := VerifyEnrollment(down, mustB64(t, res.Sig))
	require.Error(t, err)
	assert.False(t, ok)
	assert.EqualError(t, err, ErrAlgVersion)
}

// TestVerifyEnrollment_TamperedNetworkID: changing network_id after signing (a
// cross-network replay) breaks the self-sig (honest non-match).
func TestVerifyEnrollment_TamperedNetworkID(t *testing.T) {
	pub, priv := fixedEd25519(t, 0xB2)
	fake := &fakeSigner{priv: priv}
	f := validFields(pub)
	res, err := SignEnrollment(fake, f)
	require.NoError(t, err)

	replay := f
	replay.NetworkID = "vmnet1:ffffffffffffffffffffffffffffffff"
	ok, err := VerifyEnrollment(replay, mustB64(t, res.Sig))
	require.NoError(t, err)
	assert.False(t, ok, "a cross-network replay must break the self-sig")
}

func mustB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	require.NoError(t, err)
	return b
}
