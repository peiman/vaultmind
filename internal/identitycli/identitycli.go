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
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/envelope"
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

// Envelope-signing error strings (SSOT).
const (
	// ErrEnvelopeParse is returned by SignEnvelope when the envelope JSON cannot
	// be decoded (bad JSON, unknown field, or trailing data — strict, fail closed).
	ErrEnvelopeParse = "identitycli: parse envelope JSON"
	// ErrEnvelopeBadFromPubKey is returned when fromPubKeyB64 is set but not valid
	// base64 of an ed25519 public key.
	ErrEnvelopeBadFromPubKey = "identitycli: from_pubkey must be base64 of an ed25519 public key"
)

// wireEnvelope is the JSON shape SignEnvelope reads from stdin/--file: the
// SIGNED-SUBSET fields only. room/to_agent are POINTERS so absent-vs-present maps
// onto envelope.Fields' exactly-one routing rule. Transport fields (id, sig,
// from_pubkey, …) are NOT read here — they are not part of the signed subset and
// are stamped by the caller after signing.
type wireEnvelope struct {
	// alg_version/key_epoch/seq/ts decode as int64 so the envelope's [0, 2^53] gate
	// is the single authority for range (and behaves identically on 32/64-bit). A
	// plain int would JSON-parse-error a >2^31 value on a 32-bit build before the
	// gate ever ran — the parity trap the signed contract forbids.
	AlgVersion int64   `json:"alg_version"`
	Body       string  `json:"body"`
	FromAgent  string  `json:"from_agent"`
	KeyEpoch   int64   `json:"key_epoch"`
	Nonce      string  `json:"nonce"`
	Room       *string `json:"room"`
	Seq        int64   `json:"seq"`
	ToAgent    *string `json:"to_agent"`
	TS         int64   `json:"ts"`
}

// SignEnvelope reads a chat-message envelope (the signed-subset fields), enforces
// the Contract-B signing gates + canonicalizes via the envelope package, signs
// the canonical bytes through the KEYLESS SignerClient, and writes
// {sig, from_pubkey, key_epoch} as JSON to out. fromPubKeyB64 (optional) is the
// signer's public key stamped into the from_pubkey HINT — it is DERIVED, not
// signed, and the verifier ignores it. It is KEYLESS and FAILS CLOSED: any parse,
// gate, canonicalize, or signer error returns non-nil and prints nothing.
func SignEnvelope(out io.Writer, client SignerClient, envelopeJSON []byte, fromPubKeyB64 string) error {
	dec := json.NewDecoder(bytes.NewReader(envelopeJSON))
	dec.DisallowUnknownFields()
	var w wireEnvelope
	if err := dec.Decode(&w); err != nil {
		return fmt.Errorf("%s: %w", ErrEnvelopeParse, err)
	}
	if dec.More() {
		return fmt.Errorf("%s: trailing data after JSON object", ErrEnvelopeParse)
	}

	var fromPub ed25519.PublicKey
	if fromPubKeyB64 != "" {
		raw, err := base64.StdEncoding.DecodeString(fromPubKeyB64)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return fmt.Errorf("%s", ErrEnvelopeBadFromPubKey)
		}
		fromPub = raw
	}

	res, err := envelope.SignEnvelope(client, envelope.Fields{
		AlgVersion: w.AlgVersion,
		Body:       w.Body,
		FromAgent:  w.FromAgent,
		KeyEpoch:   w.KeyEpoch,
		Nonce:      w.Nonce,
		Room:       w.Room,
		ToAgent:    w.ToAgent,
		Seq:        w.Seq,
		TS:         w.TS,
	}, fromPub)
	if err != nil {
		return fmt.Errorf("signing envelope via signer: %w", err)
	}

	outJSON, err := json.Marshal(map[string]any{
		envelope.FieldSig:        res.Sig,
		envelope.FieldFromPubKey: res.FromPubKey,
		envelope.FieldKeyEpoch:   res.KeyEpoch,
	})
	if err != nil {
		return fmt.Errorf("marshaling sign-envelope result: %w", err)
	}
	_, werr := fmt.Fprintf(out, "%s\n", outJSON)
	return werr
}
