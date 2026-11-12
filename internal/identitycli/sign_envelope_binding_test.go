package identitycli

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/identity/envelope"
	"github.com/peiman/vaultmind/internal/identity/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var bindingNow = time.Unix(2_000_000, 0)

func liveBinding(t *testing.T, slug string, pub ed25519.PublicKey, epoch int) registry.AgentBinding {
	t.Helper()
	pk, err := registry.NewPublicKey(pub)
	require.NoError(t, err)
	return registry.AgentBinding{
		Slug: slug, DisplayName: slug, PubKey: pk, KeyEpoch: epoch,
		ValidFrom: 0, ValidUntil: 1 << 40,
	}
}

func newKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv
}

// The binding says mira signs at key_epoch 2; the envelope says 1. The hub
// would refuse it later, so sign-envelope refuses it now, before the signer.
func TestSignEnvelopeForBinding_EpochMismatchRefusedBeforeSigning(t *testing.T) {
	pub, priv := newKey(t)
	reg := registry.Registry{Agents: []registry.AgentBinding{liveBinding(t, "mira", pub, 2)}}
	fake := &fakeSignerClient{priv: priv}
	var out bytes.Buffer

	err := SignEnvelopeForBinding(&out, fake, []byte(testEnvelope), "", reg, bindingNow)

	require.ErrorContains(t, err, ErrEnvelopeEpochMismatch)
	assert.Contains(t, err.Error(), "mira")
	assert.Contains(t, err.Error(), "key_epoch 1")
	assert.Contains(t, err.Error(), "key_epoch 2")
	assert.Nil(t, fake.gotCanonical, "signer must not be called")
	assert.Empty(t, out.String())
}

func TestSignEnvelopeForBinding_NoLiveBindingRefused(t *testing.T) {
	pub, priv := newKey(t)
	cases := map[string]registry.AgentBinding{
		"unknown slug": liveBinding(t, "other", pub, 1),
		"expired":      func() registry.AgentBinding { b := liveBinding(t, "mira", pub, 1); b.ValidUntil = 10; return b }(),
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			reg := registry.Registry{Agents: []registry.AgentBinding{b}}
			fake := &fakeSignerClient{priv: priv}
			var out bytes.Buffer

			err := SignEnvelopeForBinding(&out, fake, []byte(testEnvelope), "", reg, bindingNow)

			require.ErrorContains(t, err, ErrEnvelopeNoBinding)
			assert.Contains(t, err.Error(), "mira")
			assert.Nil(t, fake.gotCanonical, "signer must not be called")
			assert.Empty(t, out.String())
		})
	}
}

// The signer holds a different key than the binding: the signature is
// discarded and nothing reaches stdout.
func TestSignEnvelopeForBinding_SignerKeyDiffersPrintsNothing(t *testing.T) {
	bindPub, bindPriv := newKey(t)
	otherPub, otherPriv := newKey(t)
	reg := registry.Registry{Agents: []registry.AgentBinding{liveBinding(t, "mira", bindPub, 1)}}
	bindFP := keyFingerprint(bindPub)
	otherFP := keyFingerprint(otherPub)

	t.Run("signer and hint both hold another key", func(t *testing.T) {
		var out bytes.Buffer
		hint := base64.StdEncoding.EncodeToString(otherPub)
		err := SignEnvelopeForBinding(&out, &fakeSignerClient{priv: otherPriv}, []byte(testEnvelope), hint, reg, bindingNow)
		require.ErrorContains(t, err, ErrEnvelopeKeyMismatch)
		assert.Contains(t, err.Error(), bindFP)
		assert.Contains(t, err.Error(), otherFP)
		assert.Empty(t, out.String())
	})

	t.Run("no from_pubkey hint still refuses", func(t *testing.T) {
		var out bytes.Buffer
		err := SignEnvelopeForBinding(&out, &fakeSignerClient{priv: otherPriv}, []byte(testEnvelope), "", reg, bindingNow)
		require.ErrorContains(t, err, ErrEnvelopeKeyMismatch)
		assert.Contains(t, err.Error(), bindFP)
		assert.Empty(t, out.String())
	})

	t.Run("from_pubkey hint differs from the binding", func(t *testing.T) {
		var out bytes.Buffer
		hint := base64.StdEncoding.EncodeToString(otherPub)
		err := SignEnvelopeForBinding(&out, &fakeSignerClient{priv: bindPriv}, []byte(testEnvelope), hint, reg, bindingNow)
		require.ErrorContains(t, err, ErrEnvelopeKeyMismatch)
		assert.Contains(t, err.Error(), bindFP)
		assert.Contains(t, err.Error(), otherFP)
		assert.Empty(t, out.String())
	})
}

// A human principal (from_agent human:<slug>, from_class human) resolves from
// the humans list, never from agents.
func TestSignEnvelopeForBinding_HumanResolvesFromHumans(t *testing.T) {
	const human = `{"alg_version":1,"body":"hi","from_agent":"human:peiman","from_class":"human","key_epoch":3,"nonce":"n1","room":"mesh","seq":1,"ts":1}`
	pub, priv := newKey(t)

	t.Run("bound in humans signs", func(t *testing.T) {
		reg := registry.Registry{Humans: []registry.AgentBinding{liveBinding(t, "peiman", pub, 3)}}
		var got, want bytes.Buffer
		require.NoError(t, SignEnvelopeForBinding(&got, &fakeSignerClient{priv: priv}, []byte(human), "", reg, bindingNow))
		require.NoError(t, SignEnvelope(&want, &fakeSignerClient{priv: priv}, []byte(human), ""))
		assert.Equal(t, want.String(), got.String())
	})

	t.Run("bound only as an agent is refused", func(t *testing.T) {
		reg := registry.Registry{Agents: []registry.AgentBinding{liveBinding(t, "peiman", pub, 3)}}
		fake := &fakeSignerClient{priv: priv}
		var out bytes.Buffer
		err := SignEnvelopeForBinding(&out, fake, []byte(human), "", reg, bindingNow)
		require.ErrorContains(t, err, ErrEnvelopeNoBinding)
		assert.Contains(t, err.Error(), "peiman")
		assert.Nil(t, fake.gotCanonical, "signer must not be called")
		assert.Empty(t, out.String())
	})
}

// Matching epoch and key: the output is exactly what SignEnvelope prints.
func TestSignEnvelopeForBinding_HappyPathSignsAsBefore(t *testing.T) {
	pub, priv := newKey(t)
	reg := registry.Registry{Agents: []registry.AgentBinding{liveBinding(t, "mira", pub, 1)}}
	hint := base64.StdEncoding.EncodeToString(pub)

	for name, h := range map[string]string{"with hint": hint, "without hint": ""} {
		t.Run(name, func(t *testing.T) {
			var got, want bytes.Buffer
			require.NoError(t, SignEnvelopeForBinding(&got, &fakeSignerClient{priv: priv}, []byte(testEnvelope), h, reg, bindingNow))
			require.NoError(t, SignEnvelope(&want, &fakeSignerClient{priv: priv}, []byte(testEnvelope), h))
			assert.Equal(t, want.String(), got.String())
		})
	}
}

// The envelope's own gates still run first: a malformed envelope is refused
// as before, not reported as a binding problem.
func TestSignEnvelopeForBinding_EnvelopeErrorsUnchanged(t *testing.T) {
	pub, priv := newKey(t)
	reg := registry.Registry{Agents: []registry.AgentBinding{liveBinding(t, "mira", pub, 1)}}
	cases := []struct {
		name, env, wantErr string
	}{
		{"unknown field", `{"nope":1}`, ErrEnvelopeParse},
		{"gate reject", `{"alg_version":2,"body":"hello","from_agent":"mira","key_epoch":1,"nonce":"n","room":"dev","seq":1,"ts":1}`, envelope.ErrAlgVersion},
		{"human prefix without human class", `{"alg_version":1,"body":"hello","from_agent":"human:mira","key_epoch":1,"nonce":"n","room":"dev","seq":1,"ts":1}`, envelope.ErrVerifyClassMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeSignerClient{priv: priv}
			var out bytes.Buffer
			err := SignEnvelopeForBinding(&out, fake, []byte(tc.env), "", reg, bindingNow)
			require.ErrorContains(t, err, tc.wantErr)
			assert.Nil(t, fake.gotCanonical, "signer must not be called")
			assert.Empty(t, out.String())
		})
	}

	t.Run("signer error", func(t *testing.T) {
		var out bytes.Buffer
		err := SignEnvelopeForBinding(&out, &fakeSignerClient{failErr: sentinelErr("signer unreachable")}, []byte(testEnvelope), "", reg, bindingNow)
		require.ErrorContains(t, err, "signer unreachable")
		assert.Empty(t, out.String())
	})
}
