package keeperctl

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/identity"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr(s string) *string { return &s }

func canonicalForm(v any) (identity.CanonicalBytes, error) {
	switch f := v.(type) {
	case ControlRequest:
		return CanonicalizeControlRequest(f)
	case Grant:
		return CanonicalizeGrant(f)
	default:
		return CanonicalizeGrantRevoke(f.(GrantRevoke))
	}
}

func sampleRequest() ControlRequest {
	return ControlRequest{AlgVersion: 1, Kind: KindControl, Keeper: "keeper-a", Action: ActionPause,
		From: "owner", KeyEpoch: 1, IssuedAt: 2000000, Nonce: "0123456789abcdef"}
}

func sampleGrant() Grant {
	return Grant{AlgVersion: 1, Kind: KindGrant, GrantID: "grant-1", From: "owner", KeyEpoch: 1,
		Grantee: "reader", Keepers: []string{"keeper-a"}, Scopes: []string{ScopeStatus},
		IssuedAt: 1999900, ExpiresAt: 2001000}
}

func sampleRevoke() GrantRevoke {
	return GrantRevoke{AlgVersion: 1, Kind: KindGrantRevoke, GrantID: "grant-1", From: "owner",
		KeyEpoch: 1, IssuedAt: 2000000, Nonce: "revoke01234567890"}
}

func rawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// gateCase carries a JSON input in Input, or, when the input is deliberately
// not one valid JSON value (malformed_json cases), its exact text in InputText.
type gateCase struct {
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`
	Input     json.RawMessage `json:"input,omitempty"`
	InputText string          `json:"input_text,omitempty"`
	Gate      Gate            `json:"gate"`
}

func (c gateCase) raw() []byte {
	if c.InputText != "" {
		return []byte(c.InputText)
	}
	return c.Input
}

func newGateCase(name, kind, input string, gate Gate) gateCase {
	if json.Valid([]byte(input)) {
		return gateCase{Name: name, Kind: kind, Input: json.RawMessage(input), Gate: gate}
	}
	return gateCase{Name: name, Kind: kind, InputText: input, Gate: gate}
}

func gateCases(t *testing.T) []gateCase {
	t.Helper()
	var cases []gateCase
	add := func(name, kind string, input any, gate Gate) {
		cases = append(cases, newGateCase(name, kind, string(rawJSON(t, input)), gate))
	}
	request := func(name string, gate Gate, edit func(*ControlRequest)) {
		r := sampleRequest()
		edit(&r)
		add(name, KindControl, r, gate)
	}
	request("bad_alg_version", GateAlgVersion, func(r *ControlRequest) { r.AlgVersion = 2 })
	request("bad_kind", GateKind, func(r *ControlRequest) { r.Kind = KindGrant })
	request("body_on_pause", GateBodyForbidden, func(r *ControlRequest) { r.Body = ptr("hello") })
	request("body_on_drain", GateBodyForbidden, func(r *ControlRequest) { r.Action = ActionDrain; r.Body = ptr("hello") })
	request("missing_message_body", GateBodyRequired, func(r *ControlRequest) { r.Action = ActionMessage })
	request("empty_rebrief_body", GateBodyRequired, func(r *ControlRequest) { r.Action = ActionRebrief; r.Body = ptr("") })
	request("grant_on_pause", GateGrantIDForbidden, func(r *ControlRequest) { r.GrantID = ptr("grant-1") })
	request("unknown_action", GateUnknownAction, func(r *ControlRequest) { r.Action = "erase" })
	request("non_nfc_body", GateNotNFC, func(r *ControlRequest) { r.Action = ActionMessage; r.Body = ptr("e\u0301") })
	request("short_nonce", GateNonceLength, func(r *ControlRequest) { r.Nonce = "short" })
	request("non_ascii", GateNotASCII, func(r *ControlRequest) { r.Keeper = "kéeper" })
	request("field_length", GateFieldLength, func(r *ControlRequest) { r.Keeper = "" })
	request("key_epoch_zero", GateKeyEpochRange, func(r *ControlRequest) { r.KeyEpoch = 0 })
	request("integer_range", GateIntegerRange, func(r *ControlRequest) { r.IssuedAt = -1 })
	request("body_too_large", GateBodyTooLarge, func(r *ControlRequest) { r.Action = ActionMessage; r.Body = ptr(strings.Repeat("x", 65537)) })
	for _, tc := range []struct {
		name, old, replacement string
		gate                   Gate
	}{
		{"null_body", `"action":"pause"`, `"action":"message","body":null`, GateNullField},
		{"float_issued_at", `"issued_at":2000000`, `"issued_at":1.0`, GateIntegerRequired},
		{"fraction_issued_at", `"issued_at":2000000`, `"issued_at":1.5`, GateIntegerRequired},
		{"exponent_issued_at", `"issued_at":2000000`, `"issued_at":2e6`, GateIntegerRequired},
		{"string_issued_at", `"issued_at":2000000`, `"issued_at":"2000000"`, GateIntegerRequired},
		{"unknown_field", `"action":"pause"`, `"action":"pause","extra":true`, GateUnknownField},
		{"missing_field", `"issued_at":2000000,`, ``, GateMissingField},
		{"null_grant_id", `"action":"pause"`, `"action":"read_status","grant_id":null`, GateNullField},
		{"huge_epoch", `"key_epoch":1`, `"key_epoch":999999999999999999999999`, GateKeyEpochRange},
		{"duplicate_field", `"action":"pause"`, `"action":"pause","action":"stop"`, GateUnknownField},
		{"unpaired_surrogate", `"action":"pause"`, `"action":"message","body":"\ud800"`, GateInvalidUTF8},
		{"number_keeper", `"keeper":"keeper-a"`, `"keeper":5`, GateWrongType},
		// 2^53+1: an f64-first parser rounds it to 2^53 and would accept it.
		{"epoch_2_53_plus_1", `"key_epoch":1`, `"key_epoch":9007199254740993`, GateKeyEpochRange},
		{"issued_at_2_53_plus_1", `"issued_at":2000000`, `"issued_at":9007199254740993`, GateIntegerRange},
		{"array_document", `{`, `[{`, GateMalformedJSON},
		{"trailing_data", `"nonce":"0123456789abcdef"}`, `"nonce":"0123456789abcdef"} {}`, GateMalformedJSON},
	} {
		input := strings.Replace(string(rawJSON(t, sampleRequest())), tc.old, tc.replacement, 1)
		cases = append(cases, newGateCase(tc.name, KindControl, input, tc.gate))
	}
	grant := func(name string, gate Gate, edit func(*Grant)) {
		g := sampleGrant()
		edit(&g)
		add(name, KindGrant, g, gate)
	}
	grant("unsorted_keepers", GateArrayNotSortedUnique, func(g *Grant) { g.Keepers = []string{"z", "a"} })
	grant("duplicate_scopes", GateArrayNotSortedUnique, func(g *Grant) { g.Scopes = []string{ScopeStatus, ScopeStatus} })
	grant("empty_keepers", GateArrayEmpty, func(g *Grant) { g.Keepers = []string{} })
	grant("unknown_scope", GateUnknownScope, func(g *Grant) { g.Scopes = []string{"write"} })
	grant("expiry_order", GateExpiryOrder, func(g *Grant) { g.ExpiresAt = g.IssuedAt })
	grant("empty_grantee", GateFieldLength, func(g *Grant) { g.Grantee = "" })
	grant("non_ascii_keeper", GateNotASCII, func(g *Grant) { g.Keepers = []string{"kéeper"} })
	for _, field := range []string{FieldKeepers, FieldScopes} {
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(rawJSON(t, sampleGrant()), &fields))
		fields[field] = json.RawMessage(`[null]`)
		add("null_"+field+"_element", KindGrant, fields, GateNullField)
	}
	r := sampleRevoke()
	r.Nonce = "bad"
	add("revoke_nonce", KindGrantRevoke, r, GateNonceLength)
	return cases
}

func parseForm(kind string, data []byte) error {
	switch kind {
	case KindControl:
		_, err := ParseControlRequest(data)
		return err
	case KindGrant:
		_, err := ParseGrant(data)
		return err
	default:
		_, err := ParseGrantRevoke(data)
		return err
	}
}

func TestParseGates(t *testing.T) {
	cases := gateCases(t)
	require.NotEmpty(t, cases)
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			var gate *GateError
			require.ErrorAs(t, parseForm(tc.Kind, tc.raw()), &gate)
			assert.Equal(t, tc.Gate, gate.Gate)
		})
	}
}

func TestFrozenCanonicalBytes(t *testing.T) {
	r, g, v := sampleRequest(), sampleGrant(), sampleRevoke()
	for _, tc := range []struct {
		name string
		form any
		want string
	}{
		{"control", r, `{"action":"pause","alg_version":1,"from":"owner","issued_at":2000000,"keeper":"keeper-a","key_epoch":1,"kind":"keeper.control","nonce":"0123456789abcdef"}`},
		{"grant", g, `{"alg_version":1,"expires_at":2001000,"from":"owner","grant_id":"grant-1","grantee":"reader","issued_at":1999900,"keepers":["keeper-a"],"key_epoch":1,"kind":"keeper.grant","scopes":["status"]}`},
		{"revoke", v, `{"alg_version":1,"from":"owner","grant_id":"grant-1","issued_at":2000000,"key_epoch":1,"kind":"keeper.grant_revoke","nonce":"revoke01234567890"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := canonicalForm(tc.form)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(c.Bytes()))
		})
	}
}

type failingSigner struct{ called bool }

func (s *failingSigner) Sign([]byte) ([]byte, error) {
	s.called = true
	return nil, errors.New("signer offline")
}

func TestSigningGatesAndLimits(t *testing.T) {
	r := sampleRequest()
	r.Action = ActionMessage
	r.Body = ptr(strings.Repeat("x", 65536))
	_, err := CanonicalizeControlRequest(r)
	require.NoError(t, err)
	s := &failingSigner{}
	_, err = SignControlRequest(s, r)
	require.EqualError(t, err, "signer offline")
	require.True(t, s.called)
	s.called = false
	r.Body = ptr(strings.Repeat("x", 65537))
	_, err = SignControlRequest(s, r)
	require.Error(t, err)
	assert.False(t, s.called)
	r.Body = ptr(string([]byte{0xff}))
	_, err = CanonicalizeControlRequest(r)
	var gate *GateError
	require.ErrorAs(t, err, &gate)
	assert.Equal(t, GateInvalidUTF8, gate.Gate)
	bad := append([]byte(`{"body":"`), 0xff)
	bad = append(bad, []byte(`"}`)...)
	require.ErrorAs(t, parseForm(KindControl, bad), &gate)
	assert.Equal(t, GateInvalidUTF8, gate.Gate)
}

func TestParserRejectsUnpairedSurrogates(t *testing.T) {
	raw := strings.Replace(string(rawJSON(t, sampleRequest())), `"action":"pause"`, `"action":"message","body":"\ud800"`, 1)
	var gate *GateError
	require.ErrorAs(t, parseForm(KindControl, []byte(raw)), &gate)
	assert.Equal(t, GateInvalidUTF8, gate.Gate)
}

func TestGrantStringsAndIntegerCeiling(t *testing.T) {
	// grantee is a registry slug and keepers name keeper ids, so they take the
	// same ID gate as from and keeper (TestGrantIdentifiersAreGatedLikeIDs).
	g := sampleGrant()
	g.Keepers = []string{"keeper-a", "keeper-b"}
	_, err := CanonicalizeGrant(g)
	require.NoError(t, err)
	r := sampleRequest()
	r.IssuedAt, r.KeyEpoch = MaxSafeInt, MaxSafeInt
	c, err := CanonicalizeControlRequest(r)
	require.NoError(t, err)
	parsed, err := ParseControlRequest(c.Bytes())
	require.NoError(t, err)
	assert.Equal(t, r, parsed)
}
