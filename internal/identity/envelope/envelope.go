package envelope

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/peiman/vaultmind/internal/identity/registry"
)

// Signed-subset field names (SSOT). These are the EXACT keys that enter the
// JCS-canonical signed bytes. JCS sorts keys, so the order they appear here is
// for readability only — the canonical order is Canonicalize's output.
const (
	FieldAlgVersion = "alg_version"
	FieldBody       = "body"
	FieldFromAgent  = "from_agent"
	FieldKeyEpoch   = "key_epoch"
	FieldNonce      = "nonce"
	FieldRoom       = "room"
	FieldSeq        = "seq"
	FieldToAgent    = "to_agent"
	FieldTS         = "ts"

	// FieldSig and FieldFromPubKey are TRANSPORT field names (NOT in the signed
	// subset). They are the keys of the {sig, from_pubkey, key_epoch} result the
	// caller stamps into the wire envelope after signing.
	FieldSig        = "sig"
	FieldFromPubKey = "from_pubkey"
)

const (
	// AlgVersion is the pinned signing algorithm version. CanonicalizeEnvelope
	// rejects any other value (anti-downgrade).
	AlgVersion = 1

	// MaxSafeInt is the inclusive upper bound for the signed integer fields
	// (alg_version, key_epoch, seq, ts). It is 2^53 — the largest integer JCS
	// (which renders numbers as IEEE-754 doubles) round-trips without precision
	// loss. Mirrors registry.MaxSafeEpoch / identity's maxSafeInteger.
	MaxSafeInt = int64(1) << 53
	// MinKeyEpoch is the inclusive lower bound for key_epoch (same anti-rollback
	// floor as the registry: a zero/negative epoch is rejected).
	MinKeyEpoch = 1
)

// Gate reject messages (SSOT — referenced from the gate path and asserted by
// tests/callers, never inlined).
const (
	// ErrAlgVersion is returned when alg_version != AlgVersion (anti-downgrade).
	ErrAlgVersion = "envelope: alg_version must be 1 (anti-downgrade)"
	// ErrIntRange is returned when alg_version, seq, or ts is outside [0, 2^53].
	ErrIntRange = "envelope: integer field out of range [0, 2^53]"
	// ErrKeyEpochRange is returned when key_epoch is outside [1, 2^53].
	ErrKeyEpochRange = "envelope: key_epoch out of range [1, 2^53]"
	// ErrBodyUTF8 is returned when body is not valid UTF-8.
	ErrBodyUTF8 = "envelope: body is not valid UTF-8"
	// ErrBodyNotNFC is returned when body is not Unicode NFC (no silent normalize).
	ErrBodyNotNFC = "envelope: body must be Unicode NFC-normalized"
	// ErrNonceASCII is returned when nonce is not ASCII.
	ErrNonceASCII = "envelope: nonce must be ASCII"
	// ErrNonceEmpty is returned when nonce is empty.
	ErrNonceEmpty = "envelope: nonce must not be empty"
	// ErrFromAgentEmpty is returned when from_agent is empty.
	ErrFromAgentEmpty = "envelope: from_agent must not be empty"
	// ErrRoutingExactlyOne is returned when NOT exactly one of room|to_agent is
	// present (both set, or neither set). Absent != null is load-bearing in JCS.
	ErrRoutingExactlyOne = "envelope: exactly one of room or to_agent must be present"

	// errMarshalSigned wraps a marshal failure of the signed-subset object.
	errMarshalSigned = "envelope: marshal signed subset"
	// errSigBadLen is returned by VerifyEnvelope when the decoded signature is not
	// ed25519.SignatureSize bytes.
	errSigBadLen = "envelope: signature must be ed25519.SignatureSize bytes"
)

// Fields is the message-envelope input. Room and ToAgent are POINTERS so the
// caller can express absent-vs-empty: EXACTLY ONE must be non-nil (the other nil
// is OMITTED from the signed bytes, never emitted as null). The transport-only
// fields (id, sig, from_pubkey, receive_ts, ioguard_verdict, origin_daemon) are
// deliberately NOT modeled here — they are excluded from the signed subset.
type Fields struct {
	AlgVersion int
	Body       string
	FromAgent  string
	KeyEpoch   int
	Nonce      string
	// Room and ToAgent: exactly one non-nil. The signed subset emits whichever is
	// present and OMITS the other.
	Room    *string
	ToAgent *string
	Seq     int
	TS      int64
}

// SignResult is what SignEnvelope returns: the base64 fields the caller stamps
// into the wire envelope. from_pubkey is a convenience hint (DERIVED, NOT part of
// the signed bytes); the verifier resolves the real key from the registry.
type SignResult struct {
	// Sig is base64-std of the ed25519 signature over the canonical signed bytes.
	Sig string
	// FromPubKey is base64-std of the signer's ed25519 public key (hint only).
	FromPubKey string
	// KeyEpoch echoes the signed key_epoch the caller stamps alongside sig.
	KeyEpoch int
}

// SignerClient is the minimal KEYLESS seam SignEnvelope needs: hand it canonical
// bytes, get a signature back. The real implementation is *signer.Client; the
// CLI never opens the key file.
type SignerClient interface {
	Sign(canonicalBytes []byte) ([]byte, error)
}

// CanonicalizeEnvelope enforces EVERY pre-sign gate over fields and returns the
// JCS-canonical bytes of the signed subset. It is the single SSOT for both the
// sign path and the verify path, so the bytes signed are byte-identical to the
// bytes verified. It returns a typed error (never silently coerces) on any gate
// failure. from_pubkey and all transport fields are NEVER included.
func CanonicalizeEnvelope(fields Fields) (identity.CanonicalBytes, error) {
	if err := checkGates(fields); err != nil {
		return identity.CanonicalBytes{}, err
	}
	subset := map[string]any{
		FieldAlgVersion: fields.AlgVersion,
		FieldBody:       fields.Body,
		FieldFromAgent:  fields.FromAgent,
		FieldKeyEpoch:   fields.KeyEpoch,
		FieldNonce:      fields.Nonce,
		FieldSeq:        fields.Seq,
		FieldTS:         fields.TS,
	}
	// Exactly one of room|to_agent is present (checkGates already enforced this);
	// emit the present one and OMIT the other (absent != null in JCS).
	if fields.Room != nil {
		subset[FieldRoom] = *fields.Room
	} else {
		subset[FieldToAgent] = *fields.ToAgent
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
	// Integer ranges. alg_version, seq, ts in [0, 2^53]; key_epoch in [1, 2^53].
	if !intInRange(int64(f.AlgVersion), 0) || !intInRange(int64(f.Seq), 0) || !intInRange(f.TS, 0) {
		return fmt.Errorf("%s", ErrIntRange)
	}
	if !intInRange(int64(f.KeyEpoch), MinKeyEpoch) {
		return fmt.Errorf("%s", ErrKeyEpochRange)
	}
	return checkBodyNonceRouting(f)
}

// checkBodyNonceRouting enforces the body/nonce/routing gates (split out of
// checkGates to keep each function small).
func checkBodyNonceRouting(f Fields) error {
	// from_agent must be present (it is the registry resolve key).
	if f.FromAgent == "" {
		return fmt.Errorf("%s", ErrFromAgentEmpty)
	}
	// body: valid UTF-8 + NFC, never silently normalized.
	if !utf8.ValidString(f.Body) {
		return fmt.Errorf("%s", ErrBodyUTF8)
	}
	if !norm.NFC.IsNormalString(f.Body) {
		return fmt.Errorf("%s", ErrBodyNotNFC)
	}
	// nonce: non-empty ASCII.
	if f.Nonce == "" {
		return fmt.Errorf("%s", ErrNonceEmpty)
	}
	if !isASCII(f.Nonce) {
		return fmt.Errorf("%s", ErrNonceASCII)
	}
	// EXACTLY ONE of room|to_agent.
	if (f.Room == nil) == (f.ToAgent == nil) {
		return fmt.Errorf("%s", ErrRoutingExactlyOne)
	}
	return nil
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

// SignEnvelope gates+canonicalizes fields, then signs the canonical bytes via the
// KEYLESS signer client and returns the {sig, from_pubkey, key_epoch} the caller
// stamps into the wire envelope. fromPubKey is the signer's public key, used ONLY
// to populate the convenience hint — it is NOT part of the signed bytes and the
// verifier ignores it (resolving the real key from the registry). It FAILS CLOSED:
// any gate, canonicalize, or signer error returns a non-nil error.
func SignEnvelope(client SignerClient, fields Fields, fromPubKey ed25519.PublicKey) (SignResult, error) {
	canonical, err := CanonicalizeEnvelope(fields)
	if err != nil {
		return SignResult{}, err
	}
	sig, err := client.Sign(canonical.Bytes())
	if err != nil {
		return SignResult{}, fmt.Errorf("envelope: sign via signer: %w", err)
	}
	return SignResult{
		Sig:        base64.StdEncoding.EncodeToString(sig),
		FromPubKey: base64.StdEncoding.EncodeToString(fromPubKey),
		KeyEpoch:   fields.KeyEpoch,
	}, nil
}

// VerifyEnvelope re-runs every pre-sign gate over the received fields, rebuilds
// the canonical signed bytes, then delegates the binding+signature check to
// registry.VerifyMessage (resolve the live binding for (from_agent, key_epoch) ->
// validated pubkey -> ZIP-215 verify). It authenticates the SIGNATURE + the
// registry BINDING only — anti-replay (seq high-water + nonce-unseen) is the
// daemon's stateful job and is NOT performed here.
//
// It distinguishes failure kinds the same way the lower layers do: a gate
// failure, a malformed signature, or a registry default-deny (unknown / revoked /
// expired / not-yet-valid / epoch-mismatch / small-order key) returns
// (false, non-nil error); an honest signature non-match returns (false, nil); a
// valid envelope returns (true, nil). It never panics.
func VerifyEnvelope(reg registry.Registry, fields Fields, sig []byte, now time.Time) (bool, error) {
	canonical, err := CanonicalizeEnvelope(fields)
	if err != nil {
		return false, err
	}
	if len(sig) != ed25519.SignatureSize {
		return false, fmt.Errorf("%s", errSigBadLen)
	}
	return registry.VerifyMessage(reg, fields.FromAgent, fields.KeyEpoch, canonical, sig, now)
}
