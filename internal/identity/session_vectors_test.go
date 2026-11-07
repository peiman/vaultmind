package identity

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// updateSessionVectors, when set (-update), regenerates the committed
// hub-session vectors. Without it the file is a stable artifact.
//
// Regenerate only this package's binary:
//
//	go test ./internal/identity/ -run TestGenerate_SessionStatementVectors -update
//
// Do not pass -update to ./internal/identity/... — sibling packages use the
// same flag name to regenerate their own fixtures.
var updateSessionVectors = flag.Bool("update", false, "regenerate testdata/session_statement_vectors.json")

const sessionVectorPath = "testdata/session_statement_vectors.json"

// sessionVectorSeed is the fixed ed25519 seed for the vector key. 32 bytes.
const sessionVectorSeed = "vaultmind-hub-session-v1-seed!!!"

// sessionVectorFile is the cross-language contract. valid is one statement
// signed under SessionDomainTag. Each reject is a correct signature over
// different bytes, or the valid signature presented with a changed field.
type sessionVectorFile struct {
	DomainTag string                `json:"domain_tag"`
	Purpose   string                `json:"purpose"`
	Seed      string                `json:"seed"`
	Valid     sessionVectorValid    `json:"valid"`
	Reject    []sessionVectorReject `json:"reject"`
}

type sessionVectorValid struct {
	Statement        SessionStatement `json:"statement"`
	CanonicalJSONHex string           `json:"canonical_json_hex"`
	TaggedBytesHex   string           `json:"tagged_bytes_hex"`
	Sig              string           `json:"sig"`
	PubKey           string           `json:"pubkey"`
}

type sessionVectorReject struct {
	Name           string           `json:"name"`
	Reason         string           `json:"reason"`
	Expect         string           `json:"expect"`
	Statement      SessionStatement `json:"statement"`
	SignedBytesHex string           `json:"signed_bytes_hex"`
	Sig            string           `json:"sig"`
	PubKey         string           `json:"pubkey"`
}

func sessionVectorKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	require.Len(t, sessionVectorSeed, ed25519.SeedSize)
	priv := ed25519.NewKeyFromSeed([]byte(sessionVectorSeed))
	return priv.Public().(ed25519.PublicKey), priv
}

func vectorStatement() SessionStatement {
	return SessionStatement{
		Challenge: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32)),
		KeyEpoch:  3,
		NetworkID: "vmnet1:00112233445566778899aabbccddeeff",
		Purpose:   SessionPurpose,
		Slug:      "mira",
		TS:        1_700_000_000,
	}
}

func buildSessionVectors(t *testing.T) sessionVectorFile {
	t.Helper()
	pub, priv := sessionVectorKey(t)
	st := vectorStatement()
	tagged, err := SessionTaggedBytes(st)
	require.NoError(t, err)
	canon := tagged.Bytes()[len(SessionDomainTag):]
	sig := ed25519.Sign(priv, tagged.Bytes())
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	fx := sessionVectorFile{
		DomainTag: SessionDomainTag,
		Purpose:   SessionPurpose,
		Seed:      sessionVectorSeed,
		Valid: sessionVectorValid{
			Statement:        st,
			CanonicalJSONHex: hex.EncodeToString(canon),
			TaggedBytesHex:   hex.EncodeToString(tagged.Bytes()),
			Sig:              base64.StdEncoding.EncodeToString(sig),
			PubKey:           pubB64,
		},
	}

	add := func(name, reason string, statement SessionStatement, signed []byte) {
		t.Helper()
		fx.Reject = append(fx.Reject, sessionVectorReject{
			Name:           name,
			Reason:         reason,
			Expect:         "reject",
			Statement:      statement,
			SignedBytesHex: hex.EncodeToString(signed),
			Sig:            base64.StdEncoding.EncodeToString(ed25519.Sign(priv, signed)),
			PubKey:         pubB64,
		})
	}

	// Envelope signatures and "no tag" are the same bytes: an envelope signs
	// untagged JCS. Both cases are listed so a hub does not invent a prefix.
	add("wrong_domain_envelope",
		"correct signature over untagged JCS, which is the envelope domain; envelopes have no domain tag",
		st, canon)
	add("wrong_domain_no_tag",
		"correct signature over the canonical JSON with no tag; byte-identical to wrong_domain_envelope because the envelope domain is untagged JCS",
		st, append([]byte(nil), canon...))
	add("wrong_domain_doctor_selfverify",
		"correct signature over the doctor self-verify tag plus the same canonical JSON",
		st, append([]byte(doctorSelfVerifyDomainTag), canon...))

	// Signed with no session tag. The presented statement stays the valid one,
	// so the gate passes and the signature does not match.
	shapedRaw, err := json.Marshal(sessionEnvelopeShaped{
		AlgVersion: 1,
		Body:       "hello",
		FromAgent:  "mira",
		Nonce:      "YWJjZGVmZ2hpamtsbW5vcA==",
		Room:       "dev",
		Seq:        7,
		TS:         2_000_000,
	})
	require.NoError(t, err)
	shapedCanon, err := Canonicalize(shapedRaw)
	require.NoError(t, err)
	add("wrong_domain_envelope_shaped",
		"correct signature over a JCS object shaped like an envelope (alg_version, body, from_agent, nonce, room, seq, ts), not a session statement",
		st, shapedCanon.Bytes())

	tamperedNet := st
	tamperedNet.NetworkID = "vmnet1:ffffffffffffffffffffffffffffffff"
	add("tampered_network_id",
		"signature over the original statement; network_id changed after signing",
		tamperedNet, tagged.Bytes())

	tamperedChal := st
	tamperedChal.Challenge = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 16))
	add("tampered_challenge",
		"signature over the original statement; challenge changed after signing",
		tamperedChal, tagged.Bytes())

	tamperedTS := st
	tamperedTS.TS = st.TS + 1
	add("tampered_ts",
		"signature over the original statement; ts changed after signing",
		tamperedTS, tagged.Bytes())

	tamperedSlug := st
	tamperedSlug.Slug = "other"
	add("tampered_slug",
		"signature over the original statement; slug changed after signing",
		tamperedSlug, tagged.Bytes())

	tamperedEpoch := st
	tamperedEpoch.KeyEpoch = 4
	add("tampered_key_epoch",
		"signature over the original statement; key_epoch changed after signing, still inside [1, 2^53]",
		tamperedEpoch, tagged.Bytes())

	tamperedPurpose := st
	tamperedPurpose.Purpose = "login"
	add("tampered_purpose",
		"signature over the original statement; purpose changed after signing, so the session gate rejects it",
		tamperedPurpose, tagged.Bytes())

	return fx
}

// sessionEnvelopeShaped is a plausible envelope-shaped JCS object. Its keys
// are alg_version, body, from_agent, nonce, room, seq, and ts.
type sessionEnvelopeShaped struct {
	AlgVersion int64  `json:"alg_version"`
	Body       string `json:"body"`
	FromAgent  string `json:"from_agent"`
	Nonce      string `json:"nonce"`
	Room       string `json:"room"`
	Seq        int64  `json:"seq"`
	TS         int64  `json:"ts"`
}

func marshalSessionVectors(t *testing.T, fx sessionVectorFile) []byte {
	t.Helper()
	out, err := json.MarshalIndent(fx, "", "  ")
	require.NoError(t, err)
	return append(out, '\n')
}

func TestGenerate_SessionStatementVectors(t *testing.T) {
	if !*updateSessionVectors {
		t.Skip("run with -update to regenerate the committed fixture")
	}
	fx := buildSessionVectors(t)
	out := marshalSessionVectors(t, fx)
	require.NoError(t, os.MkdirAll(filepath.Dir(sessionVectorPath), 0o755))
	require.NoError(t, os.WriteFile(sessionVectorPath, out, 0o644)) //nolint:gosec // committed test fixture
	t.Logf("wrote %s (%d bytes, %d reject cases)", sessionVectorPath, len(out), len(fx.Reject))
}

func TestSessionStatementVectors_MatchGenerator(t *testing.T) {
	fx := buildSessionVectors(t)
	want := marshalSessionVectors(t, fx)
	got, err := os.ReadFile(sessionVectorPath)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func TestSessionStatementVectors_Verify(t *testing.T) {
	raw, err := os.ReadFile(sessionVectorPath)
	require.NoError(t, err)
	var fx sessionVectorFile
	require.NoError(t, json.Unmarshal(raw, &fx))
	assert.Equal(t, SessionDomainTag, fx.DomainTag)
	assert.Equal(t, SessionPurpose, fx.Purpose)
	require.NotEmpty(t, fx.Reject)

	pub, err := base64.StdEncoding.DecodeString(fx.Valid.PubKey)
	require.NoError(t, err)
	sig, err := base64.StdEncoding.DecodeString(fx.Valid.Sig)
	require.NoError(t, err)
	tagged, err := hex.DecodeString(fx.Valid.TaggedBytesHex)
	require.NoError(t, err)
	rebuilt, err := SessionTaggedBytes(fx.Valid.Statement)
	require.NoError(t, err)
	assert.Equal(t, rebuilt.Bytes(), tagged)
	assert.True(t, bytes.HasPrefix(tagged, []byte(SessionDomainTag)))

	ok, err := VerifySessionStatement(pub, fx.Valid.Statement, sig)
	require.NoError(t, err)
	assert.True(t, ok)

	names := make([]string, 0, len(fx.Reject))
	for _, rc := range fx.Reject {
		names = append(names, rc.Name)
		assert.Equal(t, "reject", rc.Expect)
		signed, err := hex.DecodeString(rc.SignedBytesHex)
		require.NoError(t, err)
		rejectSig, err := base64.StdEncoding.DecodeString(rc.Sig)
		require.NoError(t, err)
		rejectPub, err := base64.StdEncoding.DecodeString(rc.PubKey)
		require.NoError(t, err)
		// The signature is valid over the recorded bytes, which are not the
		// session-tagged bytes of the presented statement.
		ok, err := VerifyCanonical(rejectPub, CanonicalBytesFromTrusted(signed), rejectSig)
		require.NoError(t, err, rc.Name)
		assert.True(t, ok, rc.Name)
		ok, err = VerifySessionStatement(rejectPub, rc.Statement, rejectSig)
		// tampered_purpose changes purpose off hub-session, so the shared
		// gate fails before the signature is compared. The recorded signature
		// still verifies over signed_bytes_hex above.
		if rc.Name == "tampered_purpose" {
			require.ErrorContains(t, err, ErrSessionPurpose, rc.Name)
		} else {
			require.NoError(t, err, rc.Name)
		}
		assert.False(t, ok, rc.Name)
		if rc.Name == "wrong_domain_envelope_shaped" {
			var obj map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(signed, &obj), rc.Name)
			require.Len(t, obj, 7, rc.Name)
			for _, k := range []string{"alg_version", "body", "from_agent", "nonce", "room", "seq", "ts"} {
				_, present := obj[k]
				assert.True(t, present, "%s missing %s", rc.Name, k)
			}
		}
	}
	assert.Equal(t, []string{
		"wrong_domain_envelope",
		"wrong_domain_no_tag",
		"wrong_domain_doctor_selfverify",
		"wrong_domain_envelope_shaped",
		"tampered_network_id",
		"tampered_challenge",
		"tampered_ts",
		"tampered_slug",
		"tampered_key_epoch",
		"tampered_purpose",
	}, names)
}

// sessionVectorStablePins are the bytes of the cases that already shipped.
// Regeneration may append cases. It must not rewrite these.
var sessionVectorStablePins = struct {
	canonHex    string
	taggedHex   string
	doctorHex   string
	validSig    string
	envelopeSig string
	doctorSig   string
	pubKey      string
}{
	canonHex:    "7b226368616c6c656e6765223a22455245524552455245524552455245524552455245524552455245524552455245524552455245524552453d222c226b65795f65706f6368223a332c226e6574776f726b5f6964223a22766d6e6574313a3030313132323333343435353636373738383939616162626363646465656666222c22707572706f7365223a226875622d73657373696f6e222c22736c7567223a226d697261222c227473223a313730303030303030307d",
	taggedHex:   "7661756c746d696e642d6875622d73657373696f6e2d76313a7b226368616c6c656e6765223a22455245524552455245524552455245524552455245524552455245524552455245524552455245524552453d222c226b65795f65706f6368223a332c226e6574776f726b5f6964223a22766d6e6574313a3030313132323333343435353636373738383939616162626363646465656666222c22707572706f7365223a226875622d73657373696f6e222c22736c7567223a226d697261222c227473223a313730303030303030307d",
	doctorHex:   "7661756c746d696e642d646f63746f722d73656c667665726966792d76313a7b226368616c6c656e6765223a22455245524552455245524552455245524552455245524552455245524552455245524552455245524552453d222c226b65795f65706f6368223a332c226e6574776f726b5f6964223a22766d6e6574313a3030313132323333343435353636373738383939616162626363646465656666222c22707572706f7365223a226875622d73657373696f6e222c22736c7567223a226d697261222c227473223a313730303030303030307d",
	validSig:    "hl6l9LOJLbft2ogJ5KSaPn8g0nugg0RjHcANh+GEteUjGPmSnUl/E+4IGPIosOg2BKrj/dFmQHMhHiTJ7rUdDQ==",
	envelopeSig: "U+ttgunxO21ALrsAnGsjFtZjNKt54ZX9r9+mACd1eMLQnnWXfnwFgZtQfcrupSOuBBhji98Z7pusbs35cAF/DQ==",
	doctorSig:   "TUqq4so50jSS/IKQhW5KdfavE89ohIncKWPNKbvNTocoxSoyC14/71X/8nVP5oGfPMCkmuurJFUdkxN+PZlECg==",
	pubKey:      "stWdnOhyOzOUW9nhjDOtnzxlCi1BDupvuYAtqhM3HWA=",
}

func TestSessionStatementVectors_ExistingCasesStayByteIdentical(t *testing.T) {
	pin := sessionVectorStablePins
	check := func(t *testing.T, fx sessionVectorFile) {
		t.Helper()
		assert.Equal(t, pin.canonHex, fx.Valid.CanonicalJSONHex)
		assert.Equal(t, pin.taggedHex, fx.Valid.TaggedBytesHex)
		assert.Equal(t, pin.validSig, fx.Valid.Sig)
		assert.Equal(t, pin.pubKey, fx.Valid.PubKey)

		want := map[string]struct{ signed, sig string }{
			"wrong_domain_envelope":          {pin.canonHex, pin.envelopeSig},
			"wrong_domain_no_tag":            {pin.canonHex, pin.envelopeSig},
			"wrong_domain_doctor_selfverify": {pin.doctorHex, pin.doctorSig},
			"tampered_network_id":            {pin.taggedHex, pin.validSig},
			"tampered_challenge":             {pin.taggedHex, pin.validSig},
			"tampered_ts":                    {pin.taggedHex, pin.validSig},
			"tampered_slug":                  {pin.taggedHex, pin.validSig},
		}
		got := make(map[string]sessionVectorReject, len(fx.Reject))
		for _, rc := range fx.Reject {
			got[rc.Name] = rc
		}
		for name, wantCase := range want {
			rc, ok := got[name]
			require.True(t, ok, "missing %s", name)
			assert.Equal(t, wantCase.signed, rc.SignedBytesHex, name)
			assert.Equal(t, wantCase.sig, rc.Sig, name)
			assert.Equal(t, pin.pubKey, rc.PubKey, name)
		}
	}

	check(t, buildSessionVectors(t))

	raw, err := os.ReadFile(sessionVectorPath)
	require.NoError(t, err)
	var filed sessionVectorFile
	require.NoError(t, json.Unmarshal(raw, &filed))
	check(t, filed)
}
