package registry

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

// keyBackedSigner is a RegistrySigner that signs with a held ed25519 key. It
// exists ONLY to drive SignRegistryWithSigner through the keyless seam in a test
// and prove byte-identity with the raw-key SignRegistry over the same registry.
type keyBackedSigner struct {
	priv         ed25519.PrivateKey
	gotCanonical []byte
}

func (k *keyBackedSigner) Sign(canonicalBytes []byte) ([]byte, error) {
	k.gotCanonical = append([]byte(nil), canonicalBytes...)
	return ed25519.Sign(k.priv, canonicalBytes), nil
}

// failingSigner is a RegistrySigner that always errors, to prove
// SignRegistryWithSigner FAILS CLOSED.
type failingSigner struct{ err error }

func (f *failingSigner) Sign([]byte) ([]byte, error) { return nil, f.err }

type sentinel string

func (s sentinel) Error() string { return string(s) }

// TestSignRegistryWithSignerMatchesSignRegistry proves the keyless variant is
// byte-identical to the raw-key SignRegistry for the same registry: same
// canonical bytes AND same signature (the signer wraps the same root key), only
// the signing seam differs.
func TestSignRegistryWithSignerMatchesSignRegistry(t *testing.T) {
	rootPub, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))

	want, err := SignRegistry(rootPriv, reg)
	if err != nil {
		t.Fatalf("SignRegistry: %v", err)
	}

	signer := &keyBackedSigner{priv: rootPriv}
	got, err := SignRegistryWithSigner(signer, reg)
	if err != nil {
		t.Fatalf("SignRegistryWithSigner: %v", err)
	}

	if !bytes.Equal(got.Registry, want.Registry) {
		t.Fatalf("canonical registry bytes differ:\n got=%q\nwant=%q", got.Registry, want.Registry)
	}
	if !bytes.Equal(got.RootSig, want.RootSig) {
		t.Fatalf("root signatures differ (signer-side mismatch)")
	}
	if got.RootKeyEpoch != want.RootKeyEpoch {
		t.Fatalf("root key epoch = %d, want %d", got.RootKeyEpoch, want.RootKeyEpoch)
	}
	// The signer must have received exactly the canonical registry bytes.
	if !bytes.Equal(signer.gotCanonical, want.Registry) {
		t.Fatalf("signer got %q, want canonical %q", signer.gotCanonical, want.Registry)
	}
	// The signature must verify under the root pubkey (end-to-end sanity).
	if !ed25519.Verify(rootPub, got.Registry, got.RootSig) {
		t.Fatal("SignRegistryWithSigner signature did not verify under the root key")
	}
}

// TestSignRegistryWithSignerRejectsOutOfRangeEpoch proves the SAME epoch gate as
// SignRegistry runs before the signer is ever called (fail closed, never sign an
// out-of-range epoch).
func TestSignRegistryWithSignerRejectsOutOfRangeEpoch(t *testing.T) {
	_, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(0, liveBinding(t, "mira", agentPub, 1)) // epoch 0 < 1

	signer := &keyBackedSigner{priv: rootPriv}
	if _, err := SignRegistryWithSigner(signer, reg); err == nil {
		t.Fatal("expected out-of-range epoch rejection, got nil")
	}
	if signer.gotCanonical != nil {
		t.Fatal("signer was called for an out-of-range epoch")
	}
}

// TestSignRegistryWithSignerRejectsOutOfRangeKeyEpoch covers a binding key_epoch
// out of range (the second arm of the shared gate).
func TestSignRegistryWithSignerRejectsOutOfRangeKeyEpoch(t *testing.T) {
	_, rootPriv := genKey(t)
	agentPub, _ := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 0)) // key_epoch 0 < 1

	signer := &keyBackedSigner{priv: rootPriv}
	if _, err := SignRegistryWithSigner(signer, reg); err == nil {
		t.Fatal("expected out-of-range key_epoch rejection, got nil")
	}
	if signer.gotCanonical != nil {
		t.Fatal("signer was called for an out-of-range key_epoch")
	}
}

// TestSignRegistryWithSignerFailsClosedOnSignerError proves a signer error
// surfaces and no SignedRegistry is produced.
func TestSignRegistryWithSignerFailsClosedOnSignerError(t *testing.T) {
	agentPub, _ := genKey(t)
	reg := freshRegistry(5, liveBinding(t, "mira", agentPub, 1))

	signer := &failingSigner{err: sentinel("signer unreachable")}
	got, err := SignRegistryWithSigner(signer, reg)
	if err == nil {
		t.Fatal("expected fail-closed signer error, got nil")
	}
	if len(got.Registry) != 0 || len(got.RootSig) != 0 {
		t.Fatal("fail-closed must return a zero SignedRegistry")
	}
}
