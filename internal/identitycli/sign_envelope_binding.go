package identitycli

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/peiman/vaultmind/internal/identity/envelope"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

// Envelope binding-check error strings (SSOT). The hub is the real gate; these
// only make a message it would refuse fail here, with a reason.
const (
	// ErrEnvelopeRegistry is returned when the configured registry file is
	// missing, unreadable, or fails to parse or verify under the pinned root.
	ErrEnvelopeRegistry = "identitycli: sign-envelope registry"
	// ErrEnvelopeNoBinding is returned when the envelope's sender has no live
	// binding in the registry. The signer is not called.
	ErrEnvelopeNoBinding = "identitycli: no live registry binding for envelope sender"
	// ErrEnvelopeEpochMismatch is returned when the envelope's key_epoch is not
	// the live binding's. The signer is not called.
	ErrEnvelopeEpochMismatch = "identitycli: envelope key_epoch does not match the registry binding"
	// ErrEnvelopeKeyMismatch is returned when the from_pubkey hint or the signer's
	// key is not the binding's key. Nothing is written.
	ErrEnvelopeKeyMismatch = "identitycli: signer key does not match the registry binding"
)

// SignEnvelopeForBinding is SignEnvelope for a machine with a verified
// registry. Before signing, the envelope's sender must have a live binding
// (from_class human resolves the humans list, everything else the agents list)
// whose key_epoch is the envelope's, and a from_pubkey hint must be the
// binding's key. After signing, the signature must verify under the binding's
// key. Any failure returns an error and writes nothing. On success the output
// is byte-identical to SignEnvelope's.
func SignEnvelopeForBinding(
	out io.Writer,
	client SignerClient,
	envelopeJSON []byte,
	fromPubKeyB64 string,
	reg registry.Registry,
	now time.Time,
) error {
	fields, fromPub, err := parseEnvelopeInput(envelopeJSON, fromPubKeyB64)
	if err != nil {
		return err
	}
	// The envelope gates run first, so a malformed envelope is reported as
	// itself, not as a binding problem.
	canonical, err := envelope.CanonicalizeEnvelope(fields)
	if err != nil {
		return fmt.Errorf("signing envelope via signer: %w", err)
	}
	binding, err := envelopeBinding(reg, fields, now)
	if err != nil {
		return err
	}
	bindPub := binding.PubKey.Bytes()
	if fromPub != nil && !bytes.Equal(fromPub, bindPub) {
		return fmt.Errorf("%s: from_pubkey %s is not %q's binding key %s",
			ErrEnvelopeKeyMismatch, keyFingerprint(fromPub), fields.FromAgent, keyFingerprint(bindPub))
	}
	res, err := envelope.SignEnvelope(client, fields, fromPub)
	if err != nil {
		return fmt.Errorf("signing envelope via signer: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(res.Sig)
	if err != nil || !ed25519.Verify(bindPub, canonical.Bytes(), sig) {
		return fmt.Errorf("%s: the signer's signature does not verify under %q's binding key %s; it was discarded",
			ErrEnvelopeKeyMismatch, fields.FromAgent, keyFingerprint(bindPub))
	}
	return writeEnvelopeResult(out, res)
}

// envelopeBinding returns the live binding for the envelope's sender, read the
// same way the verifier reads it (envelope.Principal), and refuses a key_epoch
// the binding does not have.
func envelopeBinding(reg registry.Registry, fields envelope.Fields, now time.Time) (registry.AgentBinding, error) {
	class, slug, err := envelope.Principal(fields)
	if err != nil {
		return registry.AgentBinding{}, err
	}
	resolve := registry.Resolve
	if class == registry.ClassHuman {
		resolve = registry.ResolveHuman
	}
	b, err := resolve(reg, slug, now)
	if err != nil {
		return registry.AgentBinding{}, fmt.Errorf("%s %q: %w", ErrEnvelopeNoBinding, fields.FromAgent, err)
	}
	if int64(b.KeyEpoch) != fields.KeyEpoch {
		return registry.AgentBinding{}, fmt.Errorf("%s: envelope for %q has key_epoch %d, the binding has key_epoch %d",
			ErrEnvelopeEpochMismatch, fields.FromAgent, fields.KeyEpoch, b.KeyEpoch)
	}
	return b, nil
}

// keyFingerprint is a short, stable name for an ed25519 public key in error
// messages: the first 8 bytes of its SHA-256, hex.
func keyFingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "sha256:" + hex.EncodeToString(sum[:8])
}
