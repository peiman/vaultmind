package identitycli

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/identity/invite"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

// Invite output label constants (SSOT) so cmd/ and tests reference one
// definition for the three printed blocks plus the out-of-band confirm note.
const (
	// InviteTokenLabel prefixes the printed bootstrap token. It is an OUTPUT label,
	// not a secret; the regex secret-scanner false-positives because the value ends
	// just before the next label's opening quote.
	InviteTokenLabel = "token: " // nosemgrep: go-hardcoded-secret -- output label, not a credential
	// InviteURLLabel prefixes the printed enroll URL (token in the fragment).
	InviteURLLabel = "url: "
	// InviteFingerprintLabel prefixes the printed OOB fingerprint (the network id).
	InviteFingerprintLabel = "fingerprint (share out-of-band): "
	// InviteConfirmNote is the one-line instruction printed under the fingerprint:
	// the member MUST confirm this value with the admin over a TRUSTED channel.
	InviteConfirmNote = "The member must confirm this fingerprint with you over a trusted channel before enrolling."
)

// ErrInviteBadRootPubKey is returned by Invite when rootPubKeyB64 is not
// base64-std of a valid 32-byte ed25519 public key (bad base64 / wrong length /
// small-order).
const ErrInviteBadRootPubKey = "identitycli: root pubkey must be base64-std of an ed25519 public key"

// Invite builds a Contract-B agent-network INVITE from the network's root public
// key (base64-std) and relay base URL, then prints three clearly-labelled blocks
// — the token, the enroll URL, and the OOB fingerprint (with a confirm note) —
// to out. The invite is UNSIGNED by design: it carries the root anchor itself,
// and trust comes from the member confirming the fingerprint out of band.
//
// It FAILS CLOSED: a bad/invalid root pubkey or empty relay returns a non-nil
// error and prints nothing partial.
func Invite(out io.Writer, rootPubKeyB64, relay string) error {
	pub, err := decodeRootPubKey(rootPubKeyB64)
	if err != nil {
		return err
	}
	inv := invite.Invite{
		NetworkID:  registry.NetworkID(pub),
		Relay:      relay,
		RootPubKey: rootPubKeyB64,
	}
	token, url, err := invite.Encode(inv)
	if err != nil {
		return fmt.Errorf("encoding invite: %w", err)
	}
	return printInvite(out, token, url, invite.Fingerprint(inv))
}

// decodeRootPubKey base64-std-decodes the root pubkey and validates it via
// registry.NewPublicKey (wrong-length / small-order rejected). Both failures
// fail closed with ErrInviteBadRootPubKey.
func decodeRootPubKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("%s", ErrInviteBadRootPubKey)
	}
	pk, err := registry.NewPublicKey(raw)
	if err != nil {
		return nil, fmt.Errorf("%s", ErrInviteBadRootPubKey)
	}
	return pk.Bytes(), nil
}

// printInvite writes the three labelled blocks plus the confirm note to out. It
// is the single SSOT for the invite output layout.
func printInvite(out io.Writer, token, url, fingerprint string) error {
	_, err := fmt.Fprintf(out, "%s%s\n%s%s\n%s%s\n%s\n",
		InviteTokenLabel, token,
		InviteURLLabel, url,
		InviteFingerprintLabel, fingerprint,
		InviteConfirmNote,
	)
	return err
}
