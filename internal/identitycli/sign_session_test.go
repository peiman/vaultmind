package identitycli

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sessionChallenge(n int) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, n))
}

func TestSignSession_StampsOwnBindingAndVerifies(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	fake := &fakeSignerClient{priv: priv}
	now := time.Unix(1_700_000_000, 0)
	binding := OwnBinding{Slug: "mira", KeyEpoch: 3, PubKey: pub}
	challenge := sessionChallenge(32)
	const networkID = "vmnet1:00112233445566778899aabbccddeeff"

	var out bytes.Buffer
	err = SignSession(&out, fake, binding, challenge, networkID, now)
	require.NoError(t, err)

	tagged, err := identity.SessionTaggedBytes(identity.SessionStatement{
		Challenge: challenge,
		KeyEpoch:  binding.KeyEpoch,
		NetworkID: networkID,
		Purpose:   identity.SessionPurpose,
		Slug:      binding.Slug,
		TS:        now.Unix(),
	})
	require.NoError(t, err)
	assert.Equal(t, tagged.Bytes(), fake.gotCanonical, "signer must see the tagged bytes, not bare JSON")

	var got struct {
		Statement identity.SessionStatement `json:"statement"`
		Sig       string                    `json:"sig"`
		PubKey    string                    `json:"pubkey"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.Equal(t, "mira", got.Statement.Slug)
	assert.Equal(t, int64(3), got.Statement.KeyEpoch)
	assert.Equal(t, identity.SessionPurpose, got.Statement.Purpose)
	assert.Equal(t, challenge, got.Statement.Challenge)
	assert.Equal(t, networkID, got.Statement.NetworkID)
	assert.Equal(t, now.Unix(), got.Statement.TS)
	assert.True(t, strings.HasSuffix(out.String(), "\n"))
	assert.Equal(t, 1, strings.Count(out.String(), "\n"))

	sig, err := base64.StdEncoding.DecodeString(got.Sig)
	require.NoError(t, err)
	gotPub, err := base64.StdEncoding.DecodeString(got.PubKey)
	require.NoError(t, err)
	assert.Equal(t, []byte(pub), gotPub)

	ok, err := identity.VerifySessionStatement(pub, got.Statement, sig)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestSignSession_FailClosedWritesNothing(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	now := time.Unix(1_700_000_000, 0)
	binding := OwnBinding{Slug: "mira", KeyEpoch: 3, PubKey: pub}
	challenge := sessionChallenge(32)
	const networkID = "vmnet1:00112233445566778899aabbccddeeff"
	signerDown := errors.New("signer down")

	cases := []struct {
		name    string
		client  SignerClient
		binding OwnBinding
		chal    string
		net     string
		want    string
	}{
		{
			name:    "bad challenge does not call the signer",
			client:  &fakeSignerClient{priv: priv, failErr: signerDown},
			binding: binding,
			chal:    "not base64!!!",
			net:     networkID,
			want:    identity.ErrSessionChallenge,
		},
		{
			name:    "non-NFC network_id",
			client:  &fakeSignerClient{priv: priv, failErr: signerDown},
			binding: binding,
			chal:    challenge,
			net:     "e\u0301",
			want:    identity.ErrSessionNetworkID,
		},
		{
			name:    "empty slug",
			client:  &fakeSignerClient{priv: priv, failErr: signerDown},
			binding: OwnBinding{Slug: "", KeyEpoch: 3, PubKey: pub},
			chal:    challenge,
			net:     networkID,
			want:    identity.ErrSessionSlug,
		},
		{
			name:    "signer error",
			client:  &fakeSignerClient{priv: priv, failErr: signerDown},
			binding: binding,
			chal:    challenge,
			net:     networkID,
			want:    "signer down",
		},
		{
			name:    "signature does not match the binding key",
			client:  &fakeSignerClient{priv: otherPriv},
			binding: binding,
			chal:    challenge,
			net:     networkID,
			want:    ErrSessionKeyMismatch,
		},
		{
			name:    "unusable pubkey is not signed",
			client:  &fakeSignerClient{priv: priv, failErr: signerDown},
			binding: OwnBinding{Slug: "mira", KeyEpoch: 3, PubKey: pub[:8]},
			chal:    challenge,
			net:     networkID,
			want:    ErrSessionBadPubKey,
		},
		{
			name:    "nil signer",
			client:  nil,
			binding: binding,
			chal:    challenge,
			net:     networkID,
			want:    ErrSessionNilSigner,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := SignSession(&out, tc.client, tc.binding, tc.chal, tc.net, now)
			require.ErrorContains(t, err, tc.want)
			assert.Empty(t, out.String())
		})
	}
}
