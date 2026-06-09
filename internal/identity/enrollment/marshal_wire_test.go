package enrollment

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lowEntropySigB64 is a deterministic, LOW-ENTROPY base64-std signature-shaped
// value for MarshalWire tests (64 × 0xC4). MarshalWire never verifies it — it
// only stamps it into the wire JSON — so a placeholder is sufficient and keeps
// the gitleaks entropy scanner quiet.
const lowEntropySigB64 = "xMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMTExMQ=="

// TestMarshalWire_RoundTrips marshals a request with transport_endpoint ABSENT,
// re-parses it, and asserts every signed-subset field plus the sig survive, and
// that transport_endpoint is OMITTED (absent != null).
func TestMarshalWire_RoundTrips(t *testing.T) {
	pub, _ := fixedEd25519(t, 0xD4)
	f := validFields(pub)

	raw, err := MarshalWire(f, lowEntropySigB64)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))

	assert.EqualValues(t, AlgVersion, got[FieldAlgVersion])
	assert.EqualValues(t, f.Created, got[FieldCreated])
	assert.Equal(t, f.DisplayName, got[FieldDisplayName])
	assert.EqualValues(t, f.KeyEpoch, got[FieldKeyEpoch])
	assert.Equal(t, f.NetworkID, got[FieldNetworkID])
	assert.Equal(t, f.Nonce, got[FieldNonce])
	assert.Equal(t, f.PubKey, got[FieldPubKey])
	assert.Equal(t, f.Slug, got[FieldSlug])
	assert.Equal(t, f.TransportPubKey, got[FieldTransportPubKey])
	assert.Equal(t, lowEntropySigB64, got[FieldSig])

	_, present := got[FieldTransportEndpoint]
	assert.False(t, present, "transport_endpoint must be OMITTED when nil (absent != null)")
}

// TestMarshalWire_TransportEndpointPresent emits the optional transport_endpoint
// when it is set.
func TestMarshalWire_TransportEndpointPresent(t *testing.T) {
	pub, _ := fixedEd25519(t, 0xD5)
	f := validFields(pub)
	f.TransportEndpoint = strptr("[2001:db8::1]:51820")

	raw, err := MarshalWire(f, lowEntropySigB64)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, "[2001:db8::1]:51820", got[FieldTransportEndpoint])
}

// TestMarshalWire_StripSigRecanonicalizeEqualsCanonical is the security-critical
// property: dropping `sig` from the wire JSON and re-canonicalizing the result
// reproduces EXACTLY CanonicalizeEnrollment(fields). This proves MarshalWire
// carries the signed subset faithfully (the admin re-derives the same bytes the
// signature covers).
func TestMarshalWire_StripSigRecanonicalizeEqualsCanonical(t *testing.T) {
	for _, name := range []string{"endpoint-absent", "endpoint-present"} {
		t.Run(name, func(t *testing.T) {
			pub, _ := fixedEd25519(t, 0xD6)
			f := validFields(pub)
			if name == "endpoint-present" {
				f.TransportEndpoint = strptr("relay.example:51820")
			}

			raw, err := MarshalWire(f, lowEntropySigB64)
			require.NoError(t, err)

			var obj map[string]any
			require.NoError(t, json.Unmarshal(raw, &obj))
			delete(obj, FieldSig)

			stripped, err := json.Marshal(obj)
			require.NoError(t, err)
			recanon, err := identity.Canonicalize(stripped)
			require.NoError(t, err)

			want, err := CanonicalizeEnrollment(f)
			require.NoError(t, err)
			assert.True(t, bytes.Equal(want.Bytes(), recanon.Bytes()),
				"strip-sig + re-canonicalize must equal CanonicalizeEnrollment\nwant=%s\ngot =%s",
				want.Bytes(), recanon.Bytes())
		})
	}
}

// TestMarshalWire_GateFailurePropagates proves MarshalWire enforces the same
// pre-emit gates: a field that CanonicalizeEnrollment rejects must fail here too
// (never emit an ungateable request).
func TestMarshalWire_GateFailurePropagates(t *testing.T) {
	pub, _ := fixedEd25519(t, 0xD7)
	f := validFields(pub)
	f.AlgVersion = 2 // anti-downgrade gate

	_, err := MarshalWire(f, lowEntropySigB64)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrAlgVersion)
}
