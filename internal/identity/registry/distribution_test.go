package registry

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedEd25519 returns a deterministic ed25519 keypair from a one-byte seed
// filler so test vectors are stable across runs.
func fixedEd25519(t *testing.T, fill byte) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = fill
	}
	priv := ed25519.NewKeyFromSeed(seed)
	return priv.Public().(ed25519.PublicKey), priv
}

// TestMarshalDistribution_RoundTrips: a SignedRegistry marshals to the envelope
// JSON and parses back byte-identically (registry bytes, root sig, epoch).
func TestMarshalDistribution_RoundTrips(t *testing.T) {
	rootPub, rootPriv := fixedEd25519(t, 0x11)
	agentPub, _ := fixedEd25519(t, 0x22)
	apk, err := NewPublicKey(agentPub)
	require.NoError(t, err)

	reg := Registry{
		Epoch:      2,
		ValidFrom:  1000,
		ValidUntil: 9000,
		Agents: []AgentBinding{{
			Slug:                    "mira",
			DisplayName:             "Mira ⭐",
			PubKey:                  apk,
			KeyEpoch:                1,
			ValidFrom:               1000,
			ValidUntil:              9000,
			AuthorizedOriginDaemons: []string{"d1"},
		}},
	}
	env, err := SignRegistry(rootPriv, reg)
	require.NoError(t, err)
	_ = rootPub

	out, err := MarshalDistribution(env)
	require.NoError(t, err)

	got, err := ParseDistribution(out)
	require.NoError(t, err)
	assert.Equal(t, env.Registry, got.Registry, "registry bytes must round-trip")
	assert.Equal(t, env.RootSig, got.RootSig, "root sig must round-trip")
	assert.Equal(t, env.RootKeyEpoch, got.RootKeyEpoch, "root key epoch must round-trip")
}

// TestMarshalDistribution_NonZeroRootKeyEpoch round-trips a non-zero root key
// epoch (root rotation), confirming the integer is encoded bare, not as a float.
func TestMarshalDistribution_NonZeroRootKeyEpoch(t *testing.T) {
	env := SignedRegistry{
		Registry:     []byte(`{}`),
		RootSig:      make([]byte, ed25519.SignatureSize),
		RootKeyEpoch: 7,
	}
	out, err := MarshalDistribution(env)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"root_key_epoch":7`, "root_key_epoch must encode bare")
	got, err := ParseDistribution(out)
	require.NoError(t, err)
	assert.Equal(t, 7, got.RootKeyEpoch)
}

// TestMarshalDistribution_FieldNamesAndBase64: the envelope uses the SSOT field
// names and base64-std encodes the registry + sig.
func TestMarshalDistribution_FieldNamesAndBase64(t *testing.T) {
	_, rootPriv := fixedEd25519(t, 0x11)
	reg := Registry{Epoch: 1, ValidFrom: 1, ValidUntil: 2}
	env, err := SignRegistry(rootPriv, reg)
	require.NoError(t, err)

	out, err := MarshalDistribution(env)
	require.NoError(t, err)

	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out, &m))
	_, hasReg := m[fieldDistRegistry]
	_, hasSig := m[fieldDistRootSig]
	_, hasEpoch := m[fieldDistRootKeyEpoch]
	assert.True(t, hasReg, "envelope must carry the registry field")
	assert.True(t, hasSig, "envelope must carry the root_sig field")
	assert.True(t, hasEpoch, "envelope must carry the root_key_epoch field")

	var b64 string
	require.NoError(t, json.Unmarshal(m[fieldDistRegistry], &b64))
	decoded, err := base64.StdEncoding.DecodeString(b64)
	require.NoError(t, err, "registry field must be base64-std")
	assert.Equal(t, env.Registry, decoded, "registry field must decode to the exact signed bytes")
}

// TestParseDistribution_FailsClosed_BadJSON: non-JSON input is a fail-closed
// error and a zero SignedRegistry.
func TestParseDistribution_FailsClosed_BadJSON(t *testing.T) {
	got, err := ParseDistribution([]byte("not json{"))
	require.Error(t, err)
	assert.Equal(t, SignedRegistry{}, got, "bad JSON must yield a zero SignedRegistry, never a partial")
}

// TestParseDistribution_FailsClosed_MissingFields: each missing required field
// is a fail-closed error.
func TestParseDistribution_FailsClosed_MissingFields(t *testing.T) {
	goodReg := base64.StdEncoding.EncodeToString([]byte(`{"agents":[],"epoch":1,"valid_from":1,"valid_until":2}`))
	goodSig := base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))

	cases := map[string]string{
		"missing registry":      `{"root_sig":"` + goodSig + `","root_key_epoch":0}`,
		"missing root_sig":      `{"registry":"` + goodReg + `","root_key_epoch":0}`,
		"empty registry string": `{"registry":"","root_sig":"` + goodSig + `","root_key_epoch":0}`,
		"empty root_sig string": `{"registry":"` + goodReg + `","root_sig":"","root_key_epoch":0}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseDistribution([]byte(body))
			require.Error(t, err, "missing/empty required field must fail closed")
			assert.Equal(t, SignedRegistry{}, got)
		})
	}
}

// TestParseDistribution_FailsClosed_BadBase64: non-base64 registry/sig fields
// fail closed.
func TestParseDistribution_FailsClosed_BadBase64(t *testing.T) {
	goodReg := base64.StdEncoding.EncodeToString([]byte(`{}`))
	goodSig := base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))

	cases := map[string]string{
		"bad base64 registry": `{"registry":"!!!not base64!!!","root_sig":"` + goodSig + `","root_key_epoch":0}`,
		"bad base64 root_sig": `{"registry":"` + goodReg + `","root_sig":"@@@","root_key_epoch":0}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseDistribution([]byte(body))
			require.Error(t, err, "bad base64 must fail closed")
			assert.Equal(t, SignedRegistry{}, got)
		})
	}
}

// TestParseDistribution_FailsClosed_WrongSigLen: a sig that is not
// ed25519.SignatureSize bytes fails closed at parse (never reaching the verify
// path with a malformed sig).
func TestParseDistribution_FailsClosed_WrongSigLen(t *testing.T) {
	goodReg := base64.StdEncoding.EncodeToString([]byte(`{}`))
	shortSig := base64.StdEncoding.EncodeToString(make([]byte, 10))
	body := `{"registry":"` + goodReg + `","root_sig":"` + shortSig + `","root_key_epoch":0}`
	got, err := ParseDistribution([]byte(body))
	require.Error(t, err)
	assert.ErrorContains(t, err, ErrDistBadSigLen)
	assert.Equal(t, SignedRegistry{}, got)
}

// TestParseDistribution_FailsClosed_UnknownField: an extra/unknown field is a
// fail-closed reject (strict decoding — no silent acceptance of smuggled keys).
func TestParseDistribution_FailsClosed_UnknownField(t *testing.T) {
	goodReg := base64.StdEncoding.EncodeToString([]byte(`{}`))
	goodSig := base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	body := `{"registry":"` + goodReg + `","root_sig":"` + goodSig + `","root_key_epoch":0,"x":1}`
	got, err := ParseDistribution([]byte(body))
	require.Error(t, err, "unknown field must fail closed")
	assert.Equal(t, SignedRegistry{}, got)
}

// TestParseDistribution_FailsClosed_WrongFieldType: a required field present
// with the wrong JSON type (registry as a number, root_key_epoch as a string)
// fails closed.
func TestParseDistribution_FailsClosed_WrongFieldType(t *testing.T) {
	goodReg := base64.StdEncoding.EncodeToString([]byte(`{}`))
	goodSig := base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))

	cases := map[string]string{
		"registry not a string":     `{"registry":123,"root_sig":"` + goodSig + `","root_key_epoch":0}`,
		"root_key_epoch not an int": `{"registry":"` + goodReg + `","root_sig":"` + goodSig + `","root_key_epoch":"x"}`,
		"root_sig not a string":     `{"registry":"` + goodReg + `","root_sig":true,"root_key_epoch":0}`,
		"top-level not an object":   `["registry"]`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseDistribution([]byte(body))
			require.Error(t, err, "wrong field type must fail closed")
			assert.Equal(t, SignedRegistry{}, got)
		})
	}
}

// TestParseDistribution_OmittedRootKeyEpoch_DefaultsZero: an envelope with the
// required fields but no root_key_epoch parses with epoch defaulting to 0 (a
// bare optional integer), since slice-3 only uses epoch 0 today.
func TestParseDistribution_OmittedRootKeyEpoch_DefaultsZero(t *testing.T) {
	goodReg := base64.StdEncoding.EncodeToString([]byte(`{}`))
	goodSig := base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	body := `{"registry":"` + goodReg + `","root_sig":"` + goodSig + `"}`
	got, err := ParseDistribution([]byte(body))
	require.NoError(t, err)
	assert.Equal(t, 0, got.RootKeyEpoch)
}

// TestParseDistribution_ThenVerifyAndLoad: ParseDistribution feeds straight into
// the slice-3 trust gate, which loads the valid envelope.
func TestParseDistribution_ThenVerifyAndLoad(t *testing.T) {
	rootPub, rootPriv := fixedEd25519(t, 0x11)
	agentPub, _ := fixedEd25519(t, 0x22)
	apk, err := NewPublicKey(agentPub)
	require.NoError(t, err)

	reg := Registry{
		Epoch: 5, ValidFrom: 1000, ValidUntil: 100000,
		Agents: []AgentBinding{{
			Slug: "mira", DisplayName: "Mira", PubKey: apk, KeyEpoch: 1,
			ValidFrom: 1000, ValidUntil: 100000, AuthorizedOriginDaemons: []string{"d1"},
		}},
	}
	env, err := SignRegistry(rootPriv, reg)
	require.NoError(t, err)

	wire, err := MarshalDistribution(env)
	require.NoError(t, err)
	parsed, err := ParseDistribution(wire)
	require.NoError(t, err)

	loaded, newHighest, err := VerifyAndLoad(rootPub, parsed, 4, time.Unix(2000, 0), time.Hour*24*365)
	require.NoError(t, err)
	assert.Equal(t, 5, loaded.Epoch)
	assert.Equal(t, 5, newHighest)
}

// TestParseDistribution_NotDoubleEncoded ensures MarshalDistribution does not
// accidentally JSON-quote the canonical registry inside the base64 (the bytes
// must be the EXACT root-signed bytes, not re-marshaled).
func TestParseDistribution_NotDoubleEncoded(t *testing.T) {
	_, rootPriv := fixedEd25519(t, 0x11)
	reg := Registry{Epoch: 1, ValidFrom: 1, ValidUntil: 2}
	env, err := SignRegistry(rootPriv, reg)
	require.NoError(t, err)
	out, err := MarshalDistribution(env)
	require.NoError(t, err)
	// The canonical registry begins with '{' — confirm the decoded payload is
	// raw JCS, not a quoted string.
	parsed, err := ParseDistribution(out)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(parsed.Registry), "{"), "decoded registry must be raw JCS JSON")
}
