package envelope

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
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

	// FieldFromClass, FieldKind, FieldVouchesFor, FieldGateRef are the OPTIONAL
	// human-principal signed fields (S3). Each is OMITTED when absent (absent !=
	// null in JCS), exactly like room/to_agent — never emitted as a null value.
	FieldFromClass  = "from_class"
	FieldKind       = "kind"
	FieldVouchesFor = "vouches_for"
	FieldGateRef    = "gate_ref"

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

// Human-principal enum values (SSOT). from_class and kind, when present, must be
// EXACTLY one of these — any other value is a typed wrap-side reject.
const (
	// FromClassAgent | FromClassBridge | FromClassHuman are the valid from_class
	// values. FromClassAgent is also the registry's default effective class
	// (registry.ClassAgent).
	FromClassAgent  = "agent"
	FromClassBridge = "bridge"
	FromClassHuman  = "human"

	// KindChat | KindApproval are the valid kind values. An absent kind is treated
	// as chat at verify (no effect), but a PRESENT kind must be one of these.
	KindChat     = "chat"
	KindApproval = "approval"
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

	// ErrFieldNotUTF8 is returned when a present human-principal string field
	// (from_class, kind, vouches_for, gate_ref) is not valid UTF-8.
	ErrFieldNotUTF8 = "envelope: signed string field is not valid UTF-8"
	// ErrFieldNotNFC is returned when a present human-principal string field is
	// not Unicode NFC (no silent normalize — same discipline as body).
	ErrFieldNotNFC = "envelope: signed string field must be Unicode NFC-normalized"
	// ErrFromClassInvalid is returned when a present from_class is not one of
	// agent|bridge|human.
	ErrFromClassInvalid = "envelope: from_class must be agent, bridge, or human"
	// ErrKindInvalid is returned when a present kind is not one of chat|approval.
	ErrKindInvalid = "envelope: kind must be chat or approval"
	// ErrBridgeNeedsVouch is returned when from_class=bridge but vouches_for is
	// absent (a bridge MUST name whom it vouches for).
	ErrBridgeNeedsVouch = "envelope: from_class=bridge requires vouches_for"
	// ErrVouchNeedsBridge is returned when vouches_for is present but from_class
	// is not bridge (only a bridge may vouch).
	ErrVouchNeedsBridge = "envelope: vouches_for requires from_class=bridge"
	// ErrApprovalNeedsGateRef is returned when kind=approval but gate_ref is
	// absent (an approval MUST reference the gate it approves).
	ErrApprovalNeedsGateRef = "envelope: kind=approval requires gate_ref"
	// ErrGateRefNeedsApproval is returned when gate_ref is present but kind is not
	// approval (gate_ref is only meaningful on an approval).
	ErrGateRefNeedsApproval = "envelope: gate_ref requires kind=approval"

	// ErrVerifyClassMismatch is returned by VerifyEnvelope when an envelope's
	// PRESENT from_class claim does not equal the resolved binding's effective
	// class. The signature authenticates the claim; only the registry binding
	// AUTHORIZES it (authenticated != authorized — fail closed).
	ErrVerifyClassMismatch = "envelope: from_class claim does not match the registry binding class"
	// ErrVerifyVouchNotAllowed is returned by VerifyEnvelope when a bridge's
	// vouches_for is not in the binding's VouchAllowlist (empty allowlist ⇒ every
	// vouch rejected).
	ErrVerifyVouchNotAllowed = "envelope: vouches_for is not in the binding's vouch allowlist"

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
	// AlgVersion, KeyEpoch, Seq, TS are int64 (NOT int) ON PURPOSE: the signed
	// numerics are JCS/IEEE-754 bounded to [0, 2^53], and that gate must be the
	// single, platform-independent authority. With a plain int these would parse
	// differently on a 32-bit build (a valid >2^31 value would hit a JSON parse
	// error instead of the typed range gate), breaking Go<->Rust parity — the same
	// class of bug as the slice-3 epoch precision hole.
	AlgVersion int64
	Body       string
	FromAgent  string
	KeyEpoch   int64
	Nonce      string
	// Room and ToAgent: exactly one non-nil. The signed subset emits whichever is
	// present and OMITS the other.
	Room    *string
	ToAgent *string
	Seq     int64
	TS      int64
	// FromClass, Kind, VouchesFor, GateRef are the OPTIONAL human-principal signed
	// fields. Each is a POINTER so absent (nil) is OMITTED from the signed bytes
	// (never emitted as null), exactly like Room/ToAgent. When present they enter
	// the JCS map by lexicographic key name and are gated by checkGates.
	FromClass  *string
	Kind       *string
	VouchesFor *string
	GateRef    *string
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
	KeyEpoch int64
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
	// Optional human-principal fields: emit each present one, OMIT the absent
	// (absent != null in JCS — same load-bearing discipline as room/to_agent).
	if fields.FromClass != nil {
		subset[FieldFromClass] = *fields.FromClass
	}
	if fields.Kind != nil {
		subset[FieldKind] = *fields.Kind
	}
	if fields.VouchesFor != nil {
		subset[FieldVouchesFor] = *fields.VouchesFor
	}
	if fields.GateRef != nil {
		subset[FieldGateRef] = *fields.GateRef
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
	if !intInRange(f.AlgVersion, 0) || !intInRange(f.Seq, 0) || !intInRange(f.TS, 0) {
		return fmt.Errorf("%s", ErrIntRange)
	}
	if !intInRange(f.KeyEpoch, MinKeyEpoch) {
		return fmt.Errorf("%s", ErrKeyEpochRange)
	}
	if err := checkBodyNonceRouting(f); err != nil {
		return err
	}
	return checkHumanPrincipalGates(f)
}

// checkHumanPrincipalGates enforces the STRUCTURAL wrap-side gates for the four
// optional human-principal fields so a malformed human envelope cannot
// canonicalize. It mirrors workhorse's Rust wrap-side gates: NFC discipline on
// the new strings, enum validity for from_class/kind, and the two biconditional
// relationships (bridge⇔vouches_for, approval⇔gate_ref).
func checkHumanPrincipalGates(f Fields) error {
	// UTF-8 + NFC on every PRESENT new string (no silent normalize, same rule as
	// body).
	for _, s := range []*string{f.FromClass, f.Kind, f.VouchesFor, f.GateRef} {
		if s != nil {
			if err := checkSignedString(*s); err != nil {
				return err
			}
		}
	}
	// from_class / kind enums when present.
	if f.FromClass != nil && !validFromClass(*f.FromClass) {
		return fmt.Errorf("%s", ErrFromClassInvalid)
	}
	if f.Kind != nil && !validKind(*f.Kind) {
		return fmt.Errorf("%s", ErrKindInvalid)
	}
	// bridge ⇔ vouches_for: bridge REQUIRES a vouch, and a vouch REQUIRES bridge.
	isBridge := f.FromClass != nil && *f.FromClass == FromClassBridge
	if isBridge && f.VouchesFor == nil {
		return fmt.Errorf("%s", ErrBridgeNeedsVouch)
	}
	if f.VouchesFor != nil && !isBridge {
		return fmt.Errorf("%s", ErrVouchNeedsBridge)
	}
	// approval ⇔ gate_ref: approval REQUIRES a gate_ref, and a gate_ref REQUIRES
	// approval.
	isApproval := f.Kind != nil && *f.Kind == KindApproval
	if isApproval && f.GateRef == nil {
		return fmt.Errorf("%s", ErrApprovalNeedsGateRef)
	}
	if f.GateRef != nil && !isApproval {
		return fmt.Errorf("%s", ErrGateRefNeedsApproval)
	}
	return nil
}

// checkSignedString enforces the shared UTF-8 + NFC discipline on a present
// human-principal string field.
func checkSignedString(s string) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("%s", ErrFieldNotUTF8)
	}
	if !norm.NFC.IsNormalString(s) {
		return fmt.Errorf("%s", ErrFieldNotNFC)
	}
	return nil
}

// validFromClass reports whether c is a valid from_class enum value.
func validFromClass(c string) bool {
	return c == FromClassAgent || c == FromClassBridge || c == FromClassHuman
}

// validKind reports whether k is a valid kind enum value.
func validKind(k string) bool { return k == KindChat || k == KindApproval }

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
// validated pubkey -> cofactorless strict verify). It authenticates the SIGNATURE + the
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
	// key_epoch is already gate-bounded to [1, 2^53] above, so the int64->int
	// narrowing at the registry boundary is lossless on this project's 64-bit
	// targets. (The registry layer's own int width is tracked separately.)
	keyEpoch := int(fields.KeyEpoch)
	ok, err := registry.VerifyMessage(reg, fields.FromAgent, keyEpoch, canonical, sig, now)
	if err != nil || !ok {
		// Preserve the contract: (false, error)=structural reject (unknown /
		// revoked / expired / not-yet-valid / epoch-mismatch / small-order / gate),
		// (false, nil)=honest signature non-match. Authority is only enforced once
		// the signature + binding are authenticated.
		return ok, err
	}
	// Signature + binding authenticated. Enforce AUTHORITY (authenticated !=
	// authorized): a signed from_class is a CLAIM, the registry binding decides.
	return enforceAuthority(reg, fields, keyEpoch, now)
}

// enforceAuthority enforces the verify-side human-principal authority gate,
// FAIL-CLOSED, AFTER VerifyMessage has authenticated the signature + binding:
//
//   - from_class ABSENT ⇒ legacy/agent path, no class check (unchanged behavior).
//   - from_class PRESENT ⇒ the resolved binding's effective class
//     (Class, ""→agent) MUST equal the claim, else ErrVerifyClassMismatch. Since
//     no binding sets Class yet, ANY bridge/human claim is rejected until the
//     registry GRANTS the class — fail closed.
//   - from_class=bridge ⇒ vouches_for (gate-guaranteed present) MUST be in the
//     binding's VouchAllowlist (empty allowlist ⇒ every vouch rejected), else
//     ErrVerifyVouchNotAllowed.
func enforceAuthority(reg registry.Registry, fields Fields, keyEpoch int, now time.Time) (bool, error) {
	if fields.FromClass == nil {
		return true, nil
	}
	// VerifyMessage just succeeded for this exact tuple, so ResolveTuple resolves
	// the SAME binding; an error here would be an internal inconsistency — fail
	// closed regardless.
	binding, err := registry.ResolveTuple(reg, fields.FromAgent, keyEpoch, now)
	if err != nil {
		return false, err
	}
	if binding.EffectiveClass() != *fields.FromClass {
		return false, fmt.Errorf("%s", ErrVerifyClassMismatch)
	}
	if *fields.FromClass == FromClassBridge {
		// vouches_for is gate-guaranteed present when from_class=bridge.
		if !slices.Contains(binding.VouchAllowlist, *fields.VouchesFor) {
			return false, fmt.Errorf("%s", ErrVerifyVouchNotAllowed)
		}
	}
	return true, nil
}
