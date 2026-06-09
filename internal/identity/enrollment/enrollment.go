package enrollment

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

// Signed-subset field names (SSOT). These are the EXACT keys that enter the
// JCS-canonical signed bytes. JCS sorts keys, so the order they appear here is
// for readability only — the canonical order is Canonicalize's output.
const (
	FieldAlgVersion        = "alg_version"
	FieldCreated           = "created"
	FieldDisplayName       = "display_name"
	FieldKeyEpoch          = "key_epoch"
	FieldNetworkID         = "network_id"
	FieldNonce             = "nonce"
	FieldPubKey            = "pubkey"
	FieldSlug              = "slug"
	FieldTransportEndpoint = "transport_endpoint"
	FieldTransportPubKey   = "transport_pubkey"

	// FieldSig is a TRANSPORT field name (NOT in the signed subset). It is the key
	// of the {sig} result the caller stamps into the wire enrollment request after
	// signing.
	FieldSig = "sig"
)

const (
	// AlgVersion is the pinned signing algorithm version. CanonicalizeEnrollment
	// rejects any other value (anti-downgrade).
	AlgVersion = 1

	// MaxSafeInt is the inclusive upper bound for the signed integer fields
	// (alg_version, created, key_epoch). It is 2^53 — the largest integer JCS
	// (which renders numbers as IEEE-754 doubles) round-trips without precision
	// loss. Mirrors envelope.MaxSafeInt / registry.MaxSafeEpoch.
	MaxSafeInt = int64(1) << 53
	// MinKeyEpoch is the inclusive lower bound for key_epoch (same anti-rollback
	// floor as the registry: a zero/negative epoch is rejected).
	MinKeyEpoch = 1

	// transportPubKeyLen is the exact decoded length of the Curve25519 WireGuard
	// transport public key (length-checked only — it is NOT an ed25519 key, so it
	// is deliberately NOT small-order-checked).
	transportPubKeyLen = 32
)

// Gate reject messages (SSOT — referenced from the gate path and asserted by
// tests/callers, never inlined).
const (
	// ErrAlgVersion is returned when alg_version != AlgVersion (anti-downgrade).
	ErrAlgVersion = "enrollment: alg_version must be 1 (anti-downgrade)"
	// ErrIntRange is returned when alg_version or created is outside [0, 2^53].
	ErrIntRange = "enrollment: integer field out of range [0, 2^53]"
	// ErrKeyEpochRange is returned when key_epoch is outside [1, 2^53].
	ErrKeyEpochRange = "enrollment: key_epoch out of range [1, 2^53]"
	// ErrDisplayNameUTF8 is returned when display_name is not valid UTF-8.
	ErrDisplayNameUTF8 = "enrollment: display_name is not valid UTF-8"
	// ErrDisplayNameNotNFC is returned when display_name is not Unicode NFC (no
	// silent normalize).
	ErrDisplayNameNotNFC = "enrollment: display_name must be Unicode NFC-normalized"
	// ErrNonceASCII is returned when nonce is not ASCII.
	ErrNonceASCII = "enrollment: nonce must be ASCII"
	// ErrNonceEmpty is returned when nonce is empty.
	ErrNonceEmpty = "enrollment: nonce must not be empty"
	// ErrSlugEmpty is returned when slug is empty.
	ErrSlugEmpty = "enrollment: slug must not be empty"
	// ErrSlugASCII is returned when slug is not ASCII.
	ErrSlugASCII = "enrollment: slug must be ASCII"
	// ErrNetworkIDEmpty is returned when network_id is empty.
	ErrNetworkIDEmpty = "enrollment: network_id must not be empty"
	// ErrPubKey is returned when pubkey is not base64-std of a validatable
	// ed25519 public key (bad base64 / wrong length / small-order).
	ErrPubKey = "enrollment: pubkey must be base64-std of an ed25519 public key"
	// ErrTransportPubKey is returned when transport_pubkey is not base64-std of a
	// 32-byte Curve25519 key (bad base64 / wrong length).
	ErrTransportPubKey = "enrollment: transport_pubkey must be base64-std of a 32-byte Curve25519 key"

	// errMarshalSigned wraps a marshal failure of the signed-subset object.
	errMarshalSigned = "enrollment: marshal signed subset"
	// errSigBadLen is returned by VerifyEnrollment when the decoded signature is
	// not ed25519.SignatureSize bytes.
	errSigBadLen = "enrollment: signature must be ed25519.SignatureSize bytes"
)

// Fields is the agent-enrollment-request input. TransportEndpoint is a POINTER
// so the caller can express absent-vs-empty: nil is OMITTED from the signed
// bytes (never emitted as null). The transport-only sig is deliberately NOT
// modeled here — it is excluded from the signed subset.
type Fields struct {
	// AlgVersion, Created, KeyEpoch are int64 (NOT int) ON PURPOSE: the signed
	// numerics are JCS/IEEE-754 bounded to [0, 2^53], and that gate must be the
	// single, platform-independent authority. With a plain int these would parse
	// differently on a 32-bit build (a valid >2^31 value would hit a JSON parse
	// error instead of the typed range gate), breaking Go<->Rust parity.
	AlgVersion  int64
	Created     int64
	DisplayName string
	KeyEpoch    int64
	NetworkID   string
	Nonce       string
	// PubKey is base64-std of the enrolling agent's ed25519 IDENTITY pubkey. It IS
	// in the signed subset AND is the verification key — the whole point of
	// proof-of-possession.
	PubKey string
	Slug   string
	// TransportEndpoint is OPTIONAL: nil => OMITTED from the signed bytes
	// (absent != null).
	TransportEndpoint *string
	// TransportPubKey is base64-std of a 32-byte Curve25519 WireGuard pubkey —
	// REQUIRED, signed.
	TransportPubKey string
}

// SignResult is what SignEnrollment returns: the base64 sig the caller stamps
// into the wire enrollment request alongside the (already-present) pubkey.
type SignResult struct {
	// Sig is base64-std of the ed25519 signature over the canonical signed bytes.
	Sig string
}

// SignerClient is the minimal KEYLESS seam SignEnrollment needs: hand it
// canonical bytes, get a signature back. The real implementation is
// *signer.Client; the CLI never opens the key file.
type SignerClient interface {
	Sign(canonicalBytes []byte) ([]byte, error)
}

// CanonicalizeEnrollment enforces EVERY pre-sign gate over fields and returns the
// JCS-canonical bytes of the signed subset. It is the single SSOT for both the
// sign path and the verify path, so the bytes signed are byte-identical to the
// bytes verified. It returns a typed error (never silently coerces) on any gate
// failure. The transport sig is NEVER included; transport_endpoint is emitted
// only when present.
func CanonicalizeEnrollment(fields Fields) (identity.CanonicalBytes, error) {
	if err := checkGates(fields); err != nil {
		return identity.CanonicalBytes{}, err
	}
	subset := map[string]any{
		FieldAlgVersion:      fields.AlgVersion,
		FieldCreated:         fields.Created,
		FieldDisplayName:     fields.DisplayName,
		FieldKeyEpoch:        fields.KeyEpoch,
		FieldNetworkID:       fields.NetworkID,
		FieldNonce:           fields.Nonce,
		FieldPubKey:          fields.PubKey,
		FieldSlug:            fields.Slug,
		FieldTransportPubKey: fields.TransportPubKey,
	}
	// transport_endpoint is OPTIONAL: emit only when present, OMIT when nil
	// (absent != null in JCS).
	if fields.TransportEndpoint != nil {
		subset[FieldTransportEndpoint] = *fields.TransportEndpoint
	}

	raw, err := json.Marshal(subset)
	if err != nil {
		return identity.CanonicalBytes{}, fmt.Errorf("%s: %w", errMarshalSigned, err)
	}
	return identity.Canonicalize(raw)
}

// checkGates enforces the frozen pre-sign contract. Each rule is a TYPED reject.
func checkGates(f Fields) error {
	// Anti-downgrade: alg_version pinned to AlgVersion.
	if f.AlgVersion != AlgVersion {
		return fmt.Errorf("%s", ErrAlgVersion)
	}
	// Integer ranges. alg_version, created in [0, 2^53]; key_epoch in [1, 2^53].
	if !intInRange(f.AlgVersion, 0) || !intInRange(f.Created, 0) {
		return fmt.Errorf("%s", ErrIntRange)
	}
	if !intInRange(f.KeyEpoch, MinKeyEpoch) {
		return fmt.Errorf("%s", ErrKeyEpochRange)
	}
	return checkStringGates(f)
}

// checkStringGates enforces the display_name / nonce / slug / network_id gates
// (split out of checkGates to keep each function small).
func checkStringGates(f Fields) error {
	// display_name: valid UTF-8 + NFC, never silently normalized.
	if !utf8.ValidString(f.DisplayName) {
		return fmt.Errorf("%s", ErrDisplayNameUTF8)
	}
	if !norm.NFC.IsNormalString(f.DisplayName) {
		return fmt.Errorf("%s", ErrDisplayNameNotNFC)
	}
	// nonce: non-empty ASCII.
	if f.Nonce == "" {
		return fmt.Errorf("%s", ErrNonceEmpty)
	}
	if !isASCII(f.Nonce) {
		return fmt.Errorf("%s", ErrNonceASCII)
	}
	// slug: non-empty ASCII.
	if f.Slug == "" {
		return fmt.Errorf("%s", ErrSlugEmpty)
	}
	if !isASCII(f.Slug) {
		return fmt.Errorf("%s", ErrSlugASCII)
	}
	// network_id: non-empty (opaque here — never recomputed).
	if f.NetworkID == "" {
		return fmt.Errorf("%s", ErrNetworkIDEmpty)
	}
	return checkKeyGates(f)
}

// checkKeyGates validates the two base64 key fields. pubkey is the identity key
// (small-order-rejected via registry.NewPublicKey); transport_pubkey is the
// Curve25519 WireGuard key (length-only checked, never small-order-checked).
func checkKeyGates(f Fields) error {
	if _, err := decodeIdentityPubKey(f.PubKey); err != nil {
		return err
	}
	return checkTransportPubKey(f.TransportPubKey)
}

// checkTransportPubKey base64-std-decodes the transport pubkey and enforces the
// exact 32-byte length. It is NOT an ed25519 key, so it is deliberately NOT
// small-order-checked.
func checkTransportPubKey(b64 string) error {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != transportPubKeyLen {
		return fmt.Errorf("%s", ErrTransportPubKey)
	}
	return nil
}

// decodeIdentityPubKey base64-std-decodes the pubkey field and routes it through
// registry.NewPublicKey, so a wrong-length or small-order key is rejected. Both
// the base64 and the validation failures fail closed with ErrPubKey.
func decodeIdentityPubKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("%s", ErrPubKey)
	}
	pk, err := registry.NewPublicKey(raw)
	if err != nil {
		return nil, fmt.Errorf("%s", ErrPubKey)
	}
	return pk.Bytes(), nil
}

// intInRange reports whether v is within [lo, MaxSafeInt].
func intInRange(v, lo int64) bool { return v >= lo && v <= MaxSafeInt }

// isASCII reports whether s contains only bytes < 0x80.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// SignEnrollment gates+canonicalizes fields, then signs the canonical bytes via
// the KEYLESS signer client and returns the {sig} the caller stamps into the
// wire enrollment request. It FAILS CLOSED: any gate, canonicalize, or signer
// error returns a non-nil error.
func SignEnrollment(client SignerClient, fields Fields) (SignResult, error) {
	canonical, err := CanonicalizeEnrollment(fields)
	if err != nil {
		return SignResult{}, err
	}
	sig, err := client.Sign(canonical.Bytes())
	if err != nil {
		return SignResult{}, fmt.Errorf("enrollment: sign via signer: %w", err)
	}
	return SignResult{Sig: base64.StdEncoding.EncodeToString(sig)}, nil
}

// VerifyEnrollment self-verifies the request = PROOF-OF-POSSESSION. It re-runs
// every pre-sign gate (CanonicalizeEnrollment), decodes the pubkey FIELD to the
// ed25519 verification key, then ZIP-215 strict-verifies sig over the canonical
// bytes under THAT key. There is NO registry lookup — the request is
// self-contained.
//
// IMPORTANT: a (true, nil) result proves the requester HOLDS the private key for
// the pubkey field. It does NOT prove the slug/identity is authorized — that is
// the admin's separate out-of-band decision to add the binding to the registry.
//
// It distinguishes failure kinds: a gate failure or a malformed signature
// returns (false, non-nil error); an honest signature non-match (wrong key,
// tampered field) returns (false, nil); a valid request returns (true, nil). It
// never panics.
func VerifyEnrollment(fields Fields, sig []byte) (bool, error) {
	canonical, err := CanonicalizeEnrollment(fields)
	if err != nil {
		return false, err
	}
	if len(sig) != ed25519.SignatureSize {
		return false, fmt.Errorf("%s", errSigBadLen)
	}
	// CanonicalizeEnrollment already validated the pubkey field, so this decode
	// cannot fail; it returns the SAME validated (small-order-rejected) key.
	pubKey, err := decodeIdentityPubKey(fields.PubKey)
	if err != nil {
		return false, err
	}
	return identity.VerifyCanonical(pubKey, canonical, sig)
}
