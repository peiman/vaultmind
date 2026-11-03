package keeperctl

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/peiman/vaultmind/internal/identity"
)

// Signed field names are frozen SSOT constants. Optional fields are omitted.
const (
	FieldAlgVersion = "alg_version"
	FieldKind       = "kind"
	FieldKeeper     = "keeper"
	FieldAction     = "action"
	FieldFrom       = "from"
	FieldKeyEpoch   = "key_epoch"
	FieldIssuedAt   = "issued_at"
	FieldNonce      = "nonce"
	FieldBody       = "body"
	FieldGrantID    = "grant_id"
	FieldGrantee    = "grantee"
	FieldKeepers    = "keepers"
	FieldScopes     = "scopes"
	FieldExpiresAt  = "expires_at"
)

const (
	AlgVersion        = 1
	MaxSafeInt        = int64(1) << 53
	MaxBodyBytes      = 65536
	KindControl       = "keeper.control"
	KindGrant         = "keeper.grant"
	KindGrantRevoke   = "keeper.grant_revoke"
	ActionMessage     = "message"
	ActionRebrief     = "rebrief"
	ActionPause       = "pause"
	ActionDrain       = "drain"
	ActionResume      = "resume"
	ActionRestart     = "restart"
	ActionStop        = "stop"
	ActionReadStatus  = "read_status"
	ActionReadContext = "read_context"
	ScopeContext      = "context"
	ScopeStatus       = "status"
)

// Gate is the frozen schema rejection vocabulary.
type Gate string

const (
	GateAlgVersion           Gate = "alg_version"
	GateKind                 Gate = "kind"
	GateIntegerRequired      Gate = "integer_required"
	GateIntegerRange         Gate = "integer_range"
	GateKeyEpochRange        Gate = "key_epoch_range"
	GateNotNFC               Gate = "not_nfc"
	GateInvalidUTF8          Gate = "invalid_utf8"
	GateNotASCII             Gate = "not_ascii"
	GateFieldLength          Gate = "field_length"
	GateNonceLength          Gate = "nonce_length"
	GateUnknownAction        Gate = "unknown_action"
	GateBodyRequired         Gate = "body_required"
	GateBodyForbidden        Gate = "body_forbidden"
	GateBodyTooLarge         Gate = "body_too_large"
	GateGrantIDForbidden     Gate = "grant_id_forbidden"
	GateArrayNotSortedUnique Gate = "array_not_sorted_unique"
	GateArrayEmpty           Gate = "array_empty"
	GateUnknownScope         Gate = "unknown_scope"
	GateExpiryOrder          Gate = "expiry_order"
	GateUnknownField         Gate = "unknown_field"
	GateNullField            Gate = "null_field"
	GateMissingField         Gate = "missing_field"
	GateWrongType            Gate = "wrong_type"
	GateMalformedJSON        Gate = "malformed_json"
)

// GateError identifies the machine-readable gate and its diagnostic context.
type GateError struct {
	Gate   Gate
	Detail string
}

func (e *GateError) Error() string     { return fmt.Sprintf("keeperctl: %s: %s", e.Gate, e.Detail) }
func fail(g Gate, detail string) error { return &GateError{Gate: g, Detail: detail} }

// ControlRequest is the entire signed request, without transport signature.
// Pointers preserve absent versus present-empty fields.
type ControlRequest struct {
	AlgVersion int64   `json:"alg_version"`
	Kind       string  `json:"kind"`
	Keeper     string  `json:"keeper"`
	Action     string  `json:"action"`
	From       string  `json:"from"`
	KeyEpoch   int64   `json:"key_epoch"`
	IssuedAt   int64   `json:"issued_at"`
	Nonce      string  `json:"nonce"`
	Body       *string `json:"body,omitempty"`
	GrantID    *string `json:"grant_id,omitempty"`
}

// Grant delegates only the named read scopes for the named keepers.
type Grant struct {
	AlgVersion int64    `json:"alg_version"`
	Kind       string   `json:"kind"`
	GrantID    string   `json:"grant_id"`
	From       string   `json:"from"`
	KeyEpoch   int64    `json:"key_epoch"`
	Grantee    string   `json:"grantee"`
	Keepers    []string `json:"keepers"`
	Scopes     []string `json:"scopes"`
	IssuedAt   int64    `json:"issued_at"`
	ExpiresAt  int64    `json:"expires_at"`
}

// GrantRevoke permanently revokes a previously installed grant identifier.
type GrantRevoke struct {
	AlgVersion int64  `json:"alg_version"`
	Kind       string `json:"kind"`
	GrantID    string `json:"grant_id"`
	From       string `json:"from"`
	KeyEpoch   int64  `json:"key_epoch"`
	IssuedAt   int64  `json:"issued_at"`
	Nonce      string `json:"nonce"`
}

// SignerClient is the same keyless signing seam used by envelope.SignerClient.
type SignerClient interface {
	Sign(canonical []byte) ([]byte, error)
}

func signedBase(version int64, kind, from string, epoch, issued int64) (map[string]any, error) {
	if version != AlgVersion {
		return nil, fail(GateAlgVersion, FieldAlgVersion)
	}
	if epoch < 1 || epoch > MaxSafeInt {
		return nil, fail(GateKeyEpochRange, FieldKeyEpoch)
	}
	if issued < 0 || issued > MaxSafeInt {
		return nil, fail(GateIntegerRange, FieldIssuedAt)
	}
	if err := checkID(FieldFrom, from); err != nil {
		return nil, err
	}
	if err := checkString(FieldKind, kind); err != nil {
		return nil, err
	}
	return map[string]any{FieldAlgVersion: version, FieldKind: kind, FieldFrom: from,
		FieldKeyEpoch: epoch, FieldIssuedAt: issued}, nil
}

func checkString(field, s string) error {
	if !utf8.ValidString(s) {
		return fail(GateInvalidUTF8, field)
	}
	if !norm.NFC.IsNormalString(s) {
		return fail(GateNotNFC, field)
	}
	return nil
}

func checkASCII(field, s string) error {
	if err := checkString(field, s); err != nil {
		return err
	}
	for i := range len(s) {
		if s[i] < 0x20 || s[i] > 0x7e {
			return fail(GateNotASCII, field)
		}
	}
	return nil
}

func checkID(field, s string) error {
	if err := checkASCII(field, s); err != nil {
		return err
	}
	if len(s) == 0 || len(s) > 128 {
		return fail(GateFieldLength, field)
	}
	return nil
}

func checkNonce(s string) error {
	if err := checkASCII(FieldNonce, s); err != nil {
		return err
	}
	if len(s) < 16 || len(s) > 128 {
		return fail(GateNonceLength, FieldNonce)
	}
	return nil
}

func isRead(action string) bool { return action == ActionReadStatus || action == ActionReadContext }

func checkAction(r ControlRequest) error {
	if err := checkString(FieldAction, r.Action); err != nil {
		return err
	}
	switch r.Action {
	case ActionMessage, ActionRebrief:
		if r.Body == nil || *r.Body == "" {
			return fail(GateBodyRequired, FieldBody)
		}
	case ActionPause, ActionDrain, ActionResume, ActionRestart, ActionStop, ActionReadStatus, ActionReadContext:
		if r.Body != nil {
			return fail(GateBodyForbidden, FieldBody)
		}
	default:
		return fail(GateUnknownAction, FieldAction)
	}
	if r.Body != nil {
		if len(*r.Body) > MaxBodyBytes {
			return fail(GateBodyTooLarge, FieldBody)
		}
		if err := checkString(FieldBody, *r.Body); err != nil {
			return err
		}
	}
	if r.GrantID != nil {
		if !isRead(r.Action) {
			return fail(GateGrantIDForbidden, FieldGrantID)
		}
		return checkID(FieldGrantID, *r.GrantID)
	}
	return nil
}

// CanonicalizeControlRequest gates every field before delegating JCS to identity.
func CanonicalizeControlRequest(r ControlRequest) (identity.CanonicalBytes, error) {
	m, err := signedBase(r.AlgVersion, r.Kind, r.From, r.KeyEpoch, r.IssuedAt)
	if err != nil {
		return identity.CanonicalBytes{}, err
	}
	if r.Kind != KindControl {
		return identity.CanonicalBytes{}, fail(GateKind, FieldKind)
	}
	for _, err := range []error{checkID(FieldKeeper, r.Keeper), checkNonce(r.Nonce), checkAction(r)} {
		if err != nil {
			return identity.CanonicalBytes{}, err
		}
	}
	m[FieldKeeper], m[FieldAction], m[FieldNonce] = r.Keeper, r.Action, r.Nonce
	if r.Body != nil {
		m[FieldBody] = *r.Body
	}
	if r.GrantID != nil {
		m[FieldGrantID] = *r.GrantID
	}
	return canonicalMap(m)
}

func checkArray(field string, values []string, scopes bool) error {
	if len(values) == 0 {
		return fail(GateArrayEmpty, field)
	}
	for i, s := range values {
		if err := checkID(field, s); err != nil {
			return err
		}
		if i > 0 && values[i-1] >= s {
			return fail(GateArrayNotSortedUnique, field)
		}
		if scopes && s != ScopeContext && s != ScopeStatus {
			return fail(GateUnknownScope, field)
		}
	}
	return nil
}

// CanonicalizeGrant rejects unsorted arrays; it never sorts or normalizes input.
func CanonicalizeGrant(g Grant) (identity.CanonicalBytes, error) {
	m, err := signedBase(g.AlgVersion, g.Kind, g.From, g.KeyEpoch, g.IssuedAt)
	if err != nil {
		return identity.CanonicalBytes{}, err
	}
	if g.Kind != KindGrant {
		return identity.CanonicalBytes{}, fail(GateKind, FieldKind)
	}
	for _, err := range []error{checkID(FieldGrantID, g.GrantID), checkID(FieldGrantee, g.Grantee),
		checkArray(FieldKeepers, g.Keepers, false), checkArray(FieldScopes, g.Scopes, true)} {
		if err != nil {
			return identity.CanonicalBytes{}, err
		}
	}
	if g.ExpiresAt < 0 || g.ExpiresAt > MaxSafeInt {
		return identity.CanonicalBytes{}, fail(GateIntegerRange, FieldExpiresAt)
	}
	if g.ExpiresAt <= g.IssuedAt {
		return identity.CanonicalBytes{}, fail(GateExpiryOrder, FieldExpiresAt)
	}
	m[FieldGrantID], m[FieldGrantee] = g.GrantID, g.Grantee
	m[FieldKeepers], m[FieldScopes], m[FieldExpiresAt] = g.Keepers, g.Scopes, g.ExpiresAt
	return canonicalMap(m)
}

// CanonicalizeGrantRevoke gates a revocation's entire signed document.
func CanonicalizeGrantRevoke(r GrantRevoke) (identity.CanonicalBytes, error) {
	m, err := signedBase(r.AlgVersion, r.Kind, r.From, r.KeyEpoch, r.IssuedAt)
	if err != nil {
		return identity.CanonicalBytes{}, err
	}
	if r.Kind != KindGrantRevoke {
		return identity.CanonicalBytes{}, fail(GateKind, FieldKind)
	}
	if err := checkID(FieldGrantID, r.GrantID); err != nil {
		return identity.CanonicalBytes{}, err
	}
	if err := checkNonce(r.Nonce); err != nil {
		return identity.CanonicalBytes{}, err
	}
	m[FieldGrantID], m[FieldNonce] = r.GrantID, r.Nonce
	return canonicalMap(m)
}

func canonicalMap(m map[string]any) (identity.CanonicalBytes, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return identity.CanonicalBytes{}, err
	}
	if err := identity.ValidateSchema(raw); err != nil {
		return identity.CanonicalBytes{}, schemaError(err)
	}
	return identity.Canonicalize(raw)
}

func sign(client SignerClient, c identity.CanonicalBytes, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("keeperctl: nil signer")
	}
	return client.Sign(c.Bytes())
}

// SignControlRequest hands only validated canonical bytes to the custody signer.
func SignControlRequest(client SignerClient, r ControlRequest) ([]byte, error) {
	c, err := CanonicalizeControlRequest(r)
	return sign(client, c, err)
}

// SignGrant signs the validated grant through the keyless custody seam.
func SignGrant(client SignerClient, g Grant) ([]byte, error) {
	c, err := CanonicalizeGrant(g)
	return sign(client, c, err)
}

// SignGrantRevoke signs the validated revocation through the keyless custody seam.
func SignGrantRevoke(client SignerClient, r GrantRevoke) ([]byte, error) {
	c, err := CanonicalizeGrantRevoke(r)
	return sign(client, c, err)
}
