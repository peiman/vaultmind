package invite

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/peiman/vaultmind/internal/identity/registry"
)

// Wire field names (SSOT). These are the EXACT snake_case keys of the invite's
// transported JSON body. They are referenced from the encode/decode paths and
// asserted by tests, never inlined.
const (
	FieldNetworkID  = "network_id"
	FieldRelay      = "relay"
	FieldRootPubKey = "root_pubkey"
)

const (
	// invitePrefix is the opaque scheme tag that prefixes every invite token. A
	// token is invitePrefix + base64url-NOPADDING(JSON(wireInvite)). The "1" is a
	// version slot so the format can evolve without a silent reparse.
	invitePrefix = "vmenroll1:"

	// relayEnrollPath is the path Encode appends to the relay base URL to build the
	// enroll URL; the token rides in the URL fragment after this path. SSOT so the
	// member-side enroll command and the docs reference one definition.
	relayEnrollPath = "/enroll"

	// fragmentSep splits an enroll URL into <base>#<token>. Decode takes everything
	// AFTER the first separator as the token (a URL fragment never contains a '#').
	fragmentSep = "#"
)

// Reject messages (SSOT — referenced from Decode/Encode and asserted by
// tests/callers, never inlined). Each failure mode is a distinct typed error so
// callers can tell a malformed token from a tampered one.
const (
	// ErrBadPrefix is returned when the token (after URL-fragment extraction) does
	// not start with invitePrefix.
	ErrBadPrefix = "invite: token must start with " + invitePrefix
	// ErrBadBase64 is returned when the token body is not valid base64url-nopad.
	ErrBadBase64 = "invite: token body is not base64url (no padding)"
	// ErrBadJSON is returned when the decoded body is not a strict JSON object
	// (bad JSON, unknown field, or trailing data — fail closed).
	ErrBadJSON = "invite: token body is not a valid invite JSON object"
	// ErrEmptyRelay is returned when relay is empty.
	ErrEmptyRelay = "invite: relay must not be empty"
	// ErrBadRootPubKey is returned when root_pubkey is not base64-std of a valid
	// 32-byte ed25519 public key (bad base64 / wrong length / small-order).
	ErrBadRootPubKey = "invite: root_pubkey must be base64-std of an ed25519 public key"
	// ErrNetworkIDMismatch is returned when network_id != NetworkID(root_pubkey).
	// This is the integrity check that binds the advertised id to the actual
	// anchor key, so a tampered or substituted network_id is rejected.
	ErrNetworkIDMismatch = "invite: network_id does not match NetworkID(root_pubkey)"

	// errMarshalWire wraps a marshal failure of the wire body (should never happen
	// for the fixed-shape struct).
	errMarshalWire = "invite: marshal wire body"
)

// Invite is a self-contained, UNSIGNED transport of a network's PUBLIC trust
// anchor plus where to reach it. It is deliberately unsigned: it CARRIES the
// root pubkey (the trust anchor itself), so authenticity comes from the
// out-of-band fingerprint comparison (Fingerprint), not from a signature.
type Invite struct {
	// NetworkID is "vmnet1:" + hex(SHA-256(root pubkey)[:16]) — registry.NetworkID
	// of RootPubKey. Decode re-derives and re-checks this binding.
	NetworkID string
	// Relay is the relay base URL, e.g. "https://chat.acme.com".
	Relay string
	// RootPubKey is base64-std (padded) of the 32-byte ed25519 ROOT public key.
	RootPubKey string
}

// wireInvite is the JSON shape an invite token carries: the snake_case body. It
// is a 1:1 mirror of Invite, separated so the exported struct stays Go-idiomatic
// while the wire form pins the cross-language field names.
type wireInvite struct {
	NetworkID  string `json:"network_id"`
	Relay      string `json:"relay"`
	RootPubKey string `json:"root_pubkey"`
}

// Encode validates inv (the SAME rules as Decode, so a token Encode produces
// always round-trips) and returns the opaque token plus the enroll URL that
// carries the token in its fragment. It FAILS CLOSED: any validation failure
// returns a non-nil error and empty strings.
func Encode(inv Invite) (token string, url string, err error) {
	if err := validate(inv); err != nil {
		return "", "", err
	}
	raw, err := json.Marshal(wireInvite(inv))
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", errMarshalWire, err)
	}
	token = invitePrefix + base64.RawURLEncoding.EncodeToString(raw)
	url = inv.Relay + relayEnrollPath + fragmentSep + token
	return token, url, nil
}

// Decode accepts EITHER a bare token (vmenroll1:…) OR an enroll URL whose
// fragment is the token (https://…/enroll#vmenroll1:…). It strips the prefix,
// base64url-nopad decodes, strictly JSON-unmarshals (unknown fields + trailing
// data rejected), and VALIDATES — including the critical network_id ⇔
// root_pubkey integrity check. It FAILS CLOSED with a typed error on any failure.
func Decode(s string) (Invite, error) {
	token := tokenFromInput(s)
	body, ok := strings.CutPrefix(token, invitePrefix)
	if !ok {
		return Invite{}, fmt.Errorf("%s", ErrBadPrefix)
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return Invite{}, fmt.Errorf("%s", ErrBadBase64)
	}
	w, err := decodeWire(raw)
	if err != nil {
		return Invite{}, err
	}
	inv := Invite(w)
	if err := validate(inv); err != nil {
		return Invite{}, err
	}
	return inv, nil
}

// Fingerprint returns the human-comparable out-of-band fingerprint, which IS the
// network_id (the vmnet1:… string). This is the value the admin reads to the
// member over a TRUSTED channel, and the value `identity enroll` will ask the
// member to confirm before it trusts the anchor.
func Fingerprint(inv Invite) string { return inv.NetworkID }

// tokenFromInput returns the bare token from either a bare token or an enroll
// URL: if the input contains a fragment separator, everything after the FIRST
// one is the token; otherwise the input is already the token.
func tokenFromInput(s string) string {
	if _, frag, ok := strings.Cut(s, fragmentSep); ok {
		return frag
	}
	return s
}

// decodeWire strictly JSON-decodes the invite body: unknown fields and trailing
// data are rejected (fail closed).
func decodeWire(raw []byte) (wireInvite, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var w wireInvite
	if err := dec.Decode(&w); err != nil {
		return wireInvite{}, fmt.Errorf("%s: %w", ErrBadJSON, err)
	}
	if dec.More() {
		return wireInvite{}, fmt.Errorf("%s: trailing data", ErrBadJSON)
	}
	return w, nil
}

// validate enforces the invite contract: non-empty relay, a base64-std 32-byte
// small-order-rejected ed25519 root key, and the network_id ⇔ root_pubkey
// integrity binding. Each failure is a distinct typed reject.
func validate(inv Invite) error {
	if inv.Relay == "" {
		return fmt.Errorf("%s", ErrEmptyRelay)
	}
	pub, err := decodeRootPubKey(inv.RootPubKey)
	if err != nil {
		return err
	}
	if registry.NetworkID(pub) != inv.NetworkID {
		return fmt.Errorf("%s", ErrNetworkIDMismatch)
	}
	return nil
}

// decodeRootPubKey base64-std-decodes the root pubkey and routes it through
// registry.NewPublicKey, so a wrong-length or small-order key is rejected. Both
// the base64 and the validation failures fail closed with ErrBadRootPubKey.
func decodeRootPubKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("%s", ErrBadRootPubKey)
	}
	pk, err := registry.NewPublicKey(raw)
	if err != nil {
		return nil, fmt.Errorf("%s", ErrBadRootPubKey)
	}
	return pk.Bytes(), nil
}
