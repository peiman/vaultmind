// Package identitycli holds the business logic behind the `vaultmind identity`
// commands (init, sign), keeping the cmd/ layer ultra-thin (ADR-001).
//
// The cardinal property: this package is KEYLESS for signing. SignEntry never
// opens the private-key file — it validates+canonicalizes via the slice-1
// identity core and delegates the actual ed25519 sign to a SignerClient (the
// separate signer process over its 0600 socket). Only Init touches a private
// key, and only to SEAL it via signer.SealPrivateKey; the key is never printed
// or logged.
package identitycli

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/signer"
)

// Output label constants (SSOT) so cmd/ and tests reference one definition.
const (
	// PubKeyLabel prefixes the printed public key. Only the PUBLIC key is ever
	// printed by Init.
	PubKeyLabel = "public_key (ed25519, base64): "
	// SigLabel prefixes the printed signature.
	SigLabel = "signature (ed25519, base64): "
)

// SignerClient is the minimal seam SignEntry needs: hand it canonical bytes,
// get a signature back. The real implementation is *signer.Client; tests inject
// a fake to prove the sign path never opens the key file.
type SignerClient interface {
	Sign(canonicalBytes []byte) ([]byte, error)
}

// Init mints a per-agent ed25519 keypair, SEALs the private key to keyPath
// (0600, refusing to overwrite), and writes ONLY the public key to out. The
// private key is never printed or logged.
func Init(out io.Writer, keyPath string) error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generating keypair: %w", err)
	}
	// SEAL the private key; on any error it stays only in memory, never emitted.
	if err := signer.SealPrivateKey(keyPath, priv); err != nil {
		return fmt.Errorf("sealing private key: %w", err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	if _, err := fmt.Fprintf(out, "%s%s\n", PubKeyLabel, pubB64); err != nil {
		return err
	}
	// TODO(contract-b registry slice): also derive + print the fingerprint and
	// register the pubkey with the trust-root registry. Slice 2 is mint+custody.
	return nil
}

// SignEntry runs the slice-1 validate+canonicalize path, hands the canonical
// bytes to the SignerClient, and writes the signature to out. It is provably
// KEYLESS: it has no key path and never reads the private-key file. It FAILS
// CLOSED — any schema, canonicalize, or signer error returns non-nil and prints
// nothing.
func SignEntry(out io.Writer, client SignerClient, entryJSON []byte) error {
	if err := identity.ValidateSchema(entryJSON); err != nil {
		return fmt.Errorf("entry rejected by schema: %w", err)
	}
	canonical, err := identity.Canonicalize(entryJSON)
	if err != nil {
		return fmt.Errorf("canonicalizing entry: %w", err)
	}
	sig, err := client.Sign(canonical.Bytes())
	if err != nil {
		return fmt.Errorf("signing via signer: %w", err)
	}
	sigB64 := base64.StdEncoding.EncodeToString(sig)
	_, werr := fmt.Fprintf(out, "%s%s\n", SigLabel, sigB64)
	return werr
}
