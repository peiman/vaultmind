package identitycli

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

// Session-signing error strings (SSOT). Gate errors from identity are returned
// as-is. These cover the keyless path around them.
const (
	// ErrSessionNilSigner is returned when SignSession is called without a signer.
	ErrSessionNilSigner = "identitycli: signer client is nil"
	// ErrSessionBadPubKey is returned when the resolved binding pubkey is not a
	// usable ed25519 key. The signer is not called.
	ErrSessionBadPubKey = "identitycli: resolved binding pubkey is not a usable ed25519 key"
	// ErrSessionKeyMismatch is returned when the signer produced a signature that
	// does not verify under the resolved binding. Nothing is written.
	ErrSessionKeyMismatch = "identitycli: signature does not verify under the resolved binding"
	// ErrSessionNoSlug is returned when this project has no agent slug.
	ErrSessionNoSlug = "identitycli: no agent slug for this project"
	// ErrSessionNoPin is returned when no network root is pinned.
	ErrSessionNoPin = "identitycli: no pinned network root"
	// ErrSessionNoRegistry is returned when agents.yaml names no registry file.
	ErrSessionNoRegistry = "identitycli: no registry file for this project"
	// ErrSessionRegistry is returned when the registry cannot be read or verified.
	ErrSessionRegistry = "identitycli: registry"
	// ErrSessionNotEnrolled is returned when the slug has no live binding.
	ErrSessionNotEnrolled = "identitycli: agent is not enrolled"
)

// OwnBinding is the signer's own registry binding. SignSession stamps Slug and
// KeyEpoch into the statement and requires the signature to verify under
// PubKey. It has no field a caller can set to sign as someone else: the
// command resolves this binding and does not accept a slug or key epoch flag.
type OwnBinding struct {
	Slug     string
	KeyEpoch int64
	PubKey   ed25519.PublicKey
}

// sessionSignResult is the one JSON line SignSession prints.
type sessionSignResult struct {
	Statement identity.SessionStatement `json:"statement"`
	Sig       string                    `json:"sig"`
	PubKey    string                    `json:"pubkey"`
}

// SignSession builds a hub-session statement for binding, signs the
// domain-tagged canonical bytes through the KEYLESS SignerClient, and writes
// one JSON line: {"statement","sig","pubkey"}. ts is now.Unix(). challenge and
// networkID are the values the hub issued; they are stored as given.
//
// Slug and key_epoch come only from binding. It FAILS CLOSED: a bad pubkey, a
// gate failure, a signer error, or a signature that does not verify under
// binding.PubKey returns an error and writes nothing.
func SignSession(out io.Writer, client SignerClient, binding OwnBinding, challenge, networkID string, now time.Time) error {
	if client == nil {
		return fmt.Errorf("%s", ErrSessionNilSigner)
	}
	if _, err := registry.NewPublicKey(binding.PubKey); err != nil {
		return fmt.Errorf("%s", ErrSessionBadPubKey)
	}
	st := identity.SessionStatement{
		Challenge: challenge,
		KeyEpoch:  binding.KeyEpoch,
		NetworkID: networkID,
		Purpose:   identity.SessionPurpose,
		Slug:      binding.Slug,
		TS:        now.Unix(),
	}
	tagged, err := identity.SessionTaggedBytes(st)
	if err != nil {
		return err
	}
	sig, err := client.Sign(tagged.Bytes())
	if err != nil {
		return fmt.Errorf("signing session via signer: %w", err)
	}
	ok, err := identity.VerifySessionStatement(binding.PubKey, st, sig)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s", ErrSessionKeyMismatch)
	}
	outJSON, err := json.Marshal(sessionSignResult{
		Statement: st,
		Sig:       base64.StdEncoding.EncodeToString(sig),
		PubKey:    base64.StdEncoding.EncodeToString(binding.PubKey),
	})
	if err != nil {
		return fmt.Errorf("marshaling sign-session result: %w", err)
	}
	_, werr := fmt.Fprintf(out, "%s\n", outJSON)
	return werr
}
