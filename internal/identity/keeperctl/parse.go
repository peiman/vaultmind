package keeperctl

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/peiman/vaultmind/internal/identity"
)

var commonFields = []string{FieldAlgVersion, FieldKind, FieldFrom, FieldKeyEpoch, FieldIssuedAt}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func schemaError(err error) error {
	s := err.Error()
	for _, pair := range []struct {
		text string
		gate Gate
	}{
		{identity.ErrSchemaNotNFC, GateNotNFC},
		{identity.ErrSchemaFloatNumber, GateIntegerRequired},
		{identity.ErrSchemaIntTooLarge, GateIntegerRange},
		{identity.ErrSchemaDuplicateKey, GateUnknownField},
		{identity.ErrSchemaNonASCIIKey, GateNotASCII},
		{identity.ErrSchemaTrailingInput, GateMalformedJSON},
		{identity.ErrSchemaNotObject, GateMalformedJSON},
	} {
		if strings.Contains(s, pair.text) {
			return fail(pair.gate, s)
		}
	}
	return fail(GateMalformedJSON, s)
}

func integerField(field string) bool {
	return field == FieldAlgVersion || field == FieldKeyEpoch || field == FieldIssuedAt || field == FieldExpiresAt
}

func checkInteger(field string, raw []byte) error {
	s := string(bytes.TrimSpace(raw))
	if len(s) == 0 {
		return fail(GateIntegerRequired, field)
	}
	for i, ch := range s {
		if (ch < '0' || ch > '9') && (i != 0 || ch != '-') {
			return fail(GateIntegerRequired, field)
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	gate := GateIntegerRange
	lowest := int64(0)
	if field == FieldKeyEpoch {
		gate, lowest = GateKeyEpochRange, 1
	}
	if err != nil || n < lowest || n > MaxSafeInt {
		return fail(gate, field)
	}
	return nil
}

func rawFields(raw []byte, required, optional []string) (map[string]json.RawMessage, error) {
	if !utf8.Valid(raw) {
		return nil, fail(GateInvalidUTF8, "JSON document")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return nil, fail(GateMalformedJSON, "one JSON object required")
	}
	if err := checkRawFields(m, required, optional); err != nil {
		return nil, err
	}
	if err := identity.ValidateSchema(raw); err != nil {
		return nil, schemaError(err)
	}
	// JCS rejects unpaired UTF-16 escape surrogates. encoding/json alone
	// would silently replace them with U+FFFD before the string gates.
	if _, err := identity.Canonicalize(raw); err != nil {
		return nil, fail(GateInvalidUTF8, err.Error())
	}
	return m, nil
}

func checkRawFields(m map[string]json.RawMessage, required, optional []string) error {
	allowed := make(map[string]bool)
	for _, key := range append(append([]string{}, required...), optional...) {
		allowed[key] = true
	}
	for _, key := range sortedKeys(m) {
		if !allowed[key] {
			return fail(GateUnknownField, key)
		}
		if err := checkRawValue(key, m[key]); err != nil {
			return err
		}
	}
	for _, key := range required {
		if _, ok := m[key]; !ok {
			return fail(GateMissingField, key)
		}
	}
	return nil
}

func checkRawValue(field string, raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fail(GateNullField, field)
	}
	if integerField(field) {
		return checkInteger(field, raw)
	}
	if field != FieldKeepers && field != FieldScopes {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return fail(GateWrongType, field+": string required")
		}
		return nil
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return fail(GateWrongType, field+": string array required")
	}
	for _, value := range values {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fail(GateNullField, field+": array element")
		}
		var s string
		if err := json.Unmarshal(value, &s); err != nil {
			return fail(GateWrongType, field+": string element required")
		}
	}
	return nil
}

func decodeForm(raw []byte, out any, extraRequired, optional []string) error {
	required := append(append([]string{}, commonFields...), extraRequired...)
	if _, err := rawFields(raw, required, optional); err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fail(GateWrongType, err.Error())
	}
	return nil
}

// ParseControlRequest rejects unknown fields, nulls, and non-integer JSON tokens.
func ParseControlRequest(raw []byte) (ControlRequest, error) {
	var r ControlRequest
	err := decodeForm(raw, &r, []string{FieldKeeper, FieldAction, FieldNonce}, []string{FieldBody, FieldGrantID})
	if err == nil {
		_, err = CanonicalizeControlRequest(r)
	}
	return r, err
}

// ParseGrant strictly parses and validates a grant, preserving array order.
func ParseGrant(raw []byte) (Grant, error) {
	var g Grant
	err := decodeForm(raw, &g, []string{FieldGrantID, FieldGrantee, FieldKeepers, FieldScopes, FieldExpiresAt}, nil)
	if err == nil {
		_, err = CanonicalizeGrant(g)
	}
	return g, err
}

// ParseGrantRevoke strictly parses and validates a signed revocation document.
func ParseGrantRevoke(raw []byte) (GrantRevoke, error) {
	var r GrantRevoke
	err := decodeForm(raw, &r, []string{FieldGrantID, FieldNonce}, nil)
	if err == nil {
		_, err = CanonicalizeGrantRevoke(r)
	}
	return r, err
}
