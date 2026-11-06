package identity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doctorSelfVerifyDomainTag is the doctor proof-of-possession prefix. It is
// copied from query.selfVerifyDomainTag (that package imports this one, so the
// constant cannot be shared). A session proof signed under it must not verify.
const doctorSelfVerifyDomainTag = "vaultmind-doctor-selfverify-v1:"

func testStatement(t *testing.T) SessionStatement {
	t.Helper()
	return SessionStatement{
		Challenge: challengeB64(t, 32),
		KeyEpoch:  3,
		NetworkID: "vmnet1:00112233445566778899aabbccddeeff",
		Purpose:   SessionPurpose,
		Slug:      "mira",
		TS:        1_700_000_000,
	}
}

func challengeB64(t *testing.T, n int) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, n))
}

func testKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv
}

// TestSessionStatement_RoundTripAndTaggedBytes proves a signature over the
// domain-tagged JCS bytes verifies, and that those bytes are exactly the tag
// followed by the canonical JSON.
func TestSessionStatement_RoundTripAndTaggedBytes(t *testing.T) {
	pub, priv := testKey(t)
	st := testStatement(t)

	tagged, err := SessionTaggedBytes(st)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(tagged.Bytes(), []byte(SessionDomainTag)))
	assert.Equal(t, SessionDomainTag, "vaultmind-hub-session-v1:")

	raw, err := json.Marshal(st)
	require.NoError(t, err)
	canon, err := Canonicalize(raw)
	require.NoError(t, err)
	want := append([]byte(SessionDomainTag), canon.Bytes()...)
	assert.Equal(t, want, tagged.Bytes())

	sig := ed25519.Sign(priv, tagged.Bytes())
	ok, err := VerifySessionStatement(pub, st, sig)
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestSessionStatement_WrongDomainRejected proves a correct signature over the
// envelope domain (untagged JCS), the doctor self-verify tag, or no tag at all
// is not a session proof.
func TestSessionStatement_WrongDomainRejected(t *testing.T) {
	pub, priv := testKey(t)
	st := testStatement(t)
	tagged, err := SessionTaggedBytes(st)
	require.NoError(t, err)
	canon := tagged.Bytes()[len(SessionDomainTag):]

	cases := []struct {
		name  string
		bytes []byte
	}{
		{name: "envelope domain (untagged JCS)", bytes: canon},
		{name: "no tag", bytes: append([]byte(nil), canon...)},
		{name: "doctor self-verify tag", bytes: append([]byte(doctorSelfVerifyDomainTag), canon...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig := ed25519.Sign(priv, tc.bytes)
			ok, err := VerifySessionStatement(pub, st, sig)
			require.NoError(t, err)
			assert.False(t, ok)
		})
	}
}

// TestSessionStatement_TamperedFieldRejected proves a signature over the real
// statement does not verify after network_id, challenge, ts, or slug changes.
func TestSessionStatement_TamperedFieldRejected(t *testing.T) {
	pub, priv := testKey(t)
	st := testStatement(t)
	tagged, err := SessionTaggedBytes(st)
	require.NoError(t, err)
	sig := ed25519.Sign(priv, tagged.Bytes())

	tampered := []SessionStatement{
		func() SessionStatement { s := st; s.NetworkID = "vmnet1:ffffffffffffffffffffffffffffffff"; return s }(),
		func() SessionStatement {
			s := st
			s.Challenge = challengeB64(t, 32)
			s.Challenge = challengeB64(t, 16)
			return s
		}(),
		func() SessionStatement { s := st; s.TS = st.TS + 1; return s }(),
		func() SessionStatement { s := st; s.Slug = "other"; return s }(),
	}
	for _, s := range tampered {
		ok, err := VerifySessionStatement(pub, s, sig)
		require.NoError(t, err)
		assert.False(t, ok)
	}
}

func TestSessionStatement_ChallengeBounds(t *testing.T) {
	st := testStatement(t)
	st.Challenge = challengeB64(t, 16)
	_, err := SessionTaggedBytes(st)
	require.NoError(t, err)
	st.Challenge = challengeB64(t, 64)
	_, err = SessionTaggedBytes(st)
	require.NoError(t, err)

	rejects := []string{
		"",
		"not base64!!!",
		challengeB64(t, 15),
		challengeB64(t, 65),
	}
	for _, c := range rejects {
		st.Challenge = c
		_, err := SessionTaggedBytes(st)
		require.ErrorContains(t, err, ErrSessionChallenge)
	}
}

func TestSessionStatement_NetworkIDRules(t *testing.T) {
	st := testStatement(t)
	st.NetworkID = strings.Repeat("a", 128)
	_, err := SessionTaggedBytes(st)
	require.NoError(t, err)

	st.NetworkID = ""
	_, err = SessionTaggedBytes(st)
	require.ErrorContains(t, err, ErrSessionNetworkID)

	st.NetworkID = strings.Repeat("a", 129)
	_, err = SessionTaggedBytes(st)
	require.ErrorContains(t, err, ErrSessionNetworkID)

	st.NetworkID = "e\u0301" // NFD, not NFC
	_, err = SessionTaggedBytes(st)
	require.ErrorContains(t, err, ErrSessionNetworkID)

	st.NetworkID = string([]byte{0xff})
	_, err = SessionTaggedBytes(st)
	require.ErrorContains(t, err, ErrSessionNetworkID)
}

func TestSessionStatement_PurposeSlugEpochAndTime(t *testing.T) {
	st := testStatement(t)
	st.Purpose = "login"
	_, err := SessionTaggedBytes(st)
	require.ErrorContains(t, err, ErrSessionPurpose)

	st = testStatement(t)
	st.Slug = ""
	_, err = SessionTaggedBytes(st)
	require.ErrorContains(t, err, ErrSessionSlug)

	st = testStatement(t)
	st.KeyEpoch = 0
	_, err = SessionTaggedBytes(st)
	require.ErrorContains(t, err, ErrSessionKeyEpoch)

	st.KeyEpoch = 1 << 53
	_, err = SessionTaggedBytes(st)
	require.NoError(t, err)
	st.KeyEpoch = (1 << 53) + 1
	_, err = SessionTaggedBytes(st)
	require.ErrorContains(t, err, ErrSessionKeyEpoch)

	st = testStatement(t)
	st.TS = -1
	_, err = SessionTaggedBytes(st)
	require.ErrorContains(t, err, ErrSessionTS)
	st.TS = 1 << 53
	_, err = SessionTaggedBytes(st)
	require.NoError(t, err)
	st.TS = (1 << 53) + 1
	_, err = SessionTaggedBytes(st)
	require.ErrorContains(t, err, ErrSessionTS)
}

func TestVerifySessionStatement_WrongKeyAndBadSig(t *testing.T) {
	pub, priv := testKey(t)
	other, _ := testKey(t)
	st := testStatement(t)
	tagged, err := SessionTaggedBytes(st)
	require.NoError(t, err)
	sig := ed25519.Sign(priv, tagged.Bytes())

	ok, err := VerifySessionStatement(other, st, sig)
	require.NoError(t, err)
	assert.False(t, ok)

	_, err = VerifySessionStatement(pub, st, sig[:8])
	require.Error(t, err)
}
