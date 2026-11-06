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

	return fx
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
		require.NoError(t, err, rc.Name)
		assert.False(t, ok, rc.Name)
	}
	assert.Equal(t, []string{
		"wrong_domain_envelope",
		"wrong_domain_no_tag",
		"wrong_domain_doctor_selfverify",
		"tampered_network_id",
		"tampered_challenge",
		"tampered_ts",
		"tampered_slug",
	}, names)
}
