package enrollment

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnmarshalWire_RoundTripsMarshalWire proves UnmarshalWire is the exact
// inverse of MarshalWire for a request with transport_endpoint ABSENT: every
// signed-subset field plus the separated sig survive the round-trip and the
// resulting Fields re-canonicalize to the same bytes.
func TestUnmarshalWire_RoundTripsMarshalWire(t *testing.T) {
	pub, _ := fixedEd25519(t, 0xE1)
	f := validFields(pub)

	raw, err := MarshalWire(f, lowEntropySigB64)
	require.NoError(t, err)

	got, sigB64, err := UnmarshalWire(raw)
	require.NoError(t, err)

	assert.Equal(t, f.AlgVersion, got.AlgVersion)
	assert.Equal(t, f.Created, got.Created)
	assert.Equal(t, f.DisplayName, got.DisplayName)
	assert.Equal(t, f.KeyEpoch, got.KeyEpoch)
	assert.Equal(t, f.NetworkID, got.NetworkID)
	assert.Equal(t, f.Nonce, got.Nonce)
	assert.Equal(t, f.PubKey, got.PubKey)
	assert.Equal(t, f.Slug, got.Slug)
	assert.Equal(t, f.TransportPubKey, got.TransportPubKey)
	assert.Nil(t, got.TransportEndpoint, "absent transport_endpoint must decode to nil (absent != null)")
	assert.Equal(t, lowEntropySigB64, sigB64, "the transport sig must be separated out")

	// The decoded fields re-canonicalize to the exact bytes the sig covers.
	wantCanon, err := CanonicalizeEnrollment(f)
	require.NoError(t, err)
	gotCanon, err := CanonicalizeEnrollment(got)
	require.NoError(t, err)
	assert.Equal(t, wantCanon.Bytes(), gotCanon.Bytes())
}

// TestUnmarshalWire_TransportEndpointPresent proves a present transport_endpoint
// decodes to a non-nil *string carrying the exact value.
func TestUnmarshalWire_TransportEndpointPresent(t *testing.T) {
	pub, _ := fixedEd25519(t, 0xE2)
	f := validFields(pub)
	f.TransportEndpoint = strptr("[2001:db8::2]:51820")

	raw, err := MarshalWire(f, lowEntropySigB64)
	require.NoError(t, err)

	got, _, err := UnmarshalWire(raw)
	require.NoError(t, err)
	require.NotNil(t, got.TransportEndpoint)
	assert.Equal(t, "[2001:db8::2]:51820", *got.TransportEndpoint)
}

// TestUnmarshalWire_SeparatesSig proves the sig is returned separately and is NOT
// modeled into Fields (Fields has no Sig field — the signed subset excludes it).
func TestUnmarshalWire_SeparatesSig(t *testing.T) {
	pub, _ := fixedEd25519(t, 0xE3)
	f := validFields(pub)

	raw, err := MarshalWire(f, lowEntropySigB64)
	require.NoError(t, err)

	_, sigB64, err := UnmarshalWire(raw)
	require.NoError(t, err)
	assert.Equal(t, lowEntropySigB64, sigB64)

	// A real-shape signature (64 deterministic bytes) round-trips verbatim and
	// base64-decodes back to ed25519.SignatureSize.
	realSigB64 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, ed25519.SignatureSize))
	raw2, err := MarshalWire(f, realSigB64)
	require.NoError(t, err)
	_, sig2B64, err := UnmarshalWire(raw2)
	require.NoError(t, err)
	rawSig, err := base64.StdEncoding.DecodeString(sig2B64)
	require.NoError(t, err)
	assert.Len(t, rawSig, ed25519.SignatureSize)
}

// TestUnmarshalWire_RejectsUnknownField proves a smuggled extra key is rejected
// (strict DisallowUnknownFields), not silently dropped.
func TestUnmarshalWire_RejectsUnknownField(t *testing.T) {
	raw := []byte(`{"alg_version":1,"created":2000000,"display_name":"Mira",` +
		`"key_epoch":1,"network_id":"vmnet1:aa","nonce":"YQ==","pubkey":"x",` +
		`"slug":"mira","transport_pubkey":"y","sig":"z","EXTRA":"smuggled"}`)

	_, _, err := UnmarshalWire(raw)
	require.Error(t, err)
}

// TestUnmarshalWire_RejectsTrailingData proves bytes after the JSON object are
// rejected (a trailing-data smuggling vector).
func TestUnmarshalWire_RejectsTrailingData(t *testing.T) {
	pub, _ := fixedEd25519(t, 0xE4)
	f := validFields(pub)
	raw, err := MarshalWire(f, lowEntropySigB64)
	require.NoError(t, err)

	withTrailer := append(append([]byte(nil), raw...), []byte(` {"more":1}`)...)
	_, _, err = UnmarshalWire(withTrailer)
	require.Error(t, err)
}

// TestUnmarshalWire_RejectsBadJSON proves malformed JSON fails closed.
func TestUnmarshalWire_RejectsBadJSON(t *testing.T) {
	_, _, err := UnmarshalWire([]byte(`{not json`))
	require.Error(t, err)
}

// TestUnmarshalWire_DecodesInt64Numerics proves alg_version/created/key_epoch
// decode as int64 — a >2^31 created survives without a 32-bit parse error, so the
// gate (not a JSON parse) is the single authority (Go<->Rust parity).
func TestUnmarshalWire_DecodesInt64Numerics(t *testing.T) {
	const bigCreated int64 = 1 << 40 // > 2^31, < MaxSafeInt
	pub, _ := fixedEd25519(t, 0xE5)
	f := validFields(pub)
	f.Created = bigCreated

	raw, err := MarshalWire(f, lowEntropySigB64)
	require.NoError(t, err)

	got, _, err := UnmarshalWire(raw)
	require.NoError(t, err)
	assert.Equal(t, bigCreated, got.Created)
	assert.Equal(t, int64(AlgVersion), got.AlgVersion)
	assert.Equal(t, f.KeyEpoch, got.KeyEpoch)
}
