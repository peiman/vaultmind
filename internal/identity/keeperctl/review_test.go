package keeperctl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The freshness window is defined in whole epoch seconds, so a Rust verifier
// with an integer clock reaches the same verdict. A sub-second wall clock must
// not move either bound.
func TestFreshnessWindowUsesWholeSeconds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		issued int64
		reason Reason
	}{
		{"oldest_accepted", 2000000 - 300, ""},
		{"newest_accepted", 2000000 + 30, ""},
		{"one_past_oldest", 2000000 - 301, ReasonStale},
		{"one_past_newest", 2000000 + 31, ReasonFromFuture},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testPolicy(t)
			p.NotBefore = 1
			p.Now = func() time.Time { return time.Unix(2000000, 999_000_000) }
			r := sampleRequest()
			r.IssuedAt = tc.issued
			d := VerifyControl(p, r, requestSig(t, r))
			assert.Equal(t, tc.reason, d.Reason, d.Detail)
		})
	}
}

// Every identifier in a grant gets the same gate as keeper and from: printable
// ASCII, 1..128 bytes.
func TestGrantIdentifiersAreGatedLikeIDs(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Grant)
		gate Gate
	}{
		{"empty_grantee", func(g *Grant) { g.Grantee = "" }, GateFieldLength},
		{"non_ascii_grantee", func(g *Grant) { g.Grantee = "läsare" }, GateNotASCII},
		{"empty_keeper", func(g *Grant) { g.Keepers = []string{""} }, GateFieldLength},
		{"control_char_keeper", func(g *Grant) { g.Keepers = []string{"keeper\n"} }, GateNotASCII},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := sampleGrant()
			tc.edit(&g)
			_, err := CanonicalizeGrant(g)
			var gate *GateError
			require.ErrorAs(t, err, &gate)
			assert.Equal(t, tc.gate, gate.Gate)
		})
	}
}

// A value of the wrong JSON type, and a document that is not one JSON object,
// get their own gates rather than borrowing field_length or missing_field.
func TestParseNamesWrongTypeAndMalformedJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		gate Gate
	}{
		{"number_keeper", `{"alg_version":1,"kind":"keeper.control","keeper":5,"action":"pause","from":"owner","key_epoch":1,"issued_at":2000000,"nonce":"0123456789abcdef"}`, GateWrongType},
		{"string_keepers", `{"alg_version":1,"kind":"keeper.grant","grant_id":"g","from":"owner","key_epoch":1,"grantee":"reader","keepers":"keeper-a","scopes":["status"],"issued_at":1,"expires_at":2}`, GateWrongType},
		{"array_document", `[1]`, GateMalformedJSON},
		{"trailing_data", `{"alg_version":1} {}`, GateMalformedJSON},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gate *GateError
			require.ErrorAs(t, parseForm(KindControl, []byte(tc.raw)), &gate)
			if tc.name == "string_keepers" {
				require.ErrorAs(t, parseForm(KindGrant, []byte(tc.raw)), &gate)
			}
			assert.Equal(t, tc.gate, gate.Gate)
		})
	}
}

// Every GrantStore refuses a signature that cannot be ed25519, so a store
// filled outside AddGrant never holds one that later reads as owner rotation.
func TestGrantStoresRejectWrongLengthSignature(t *testing.T) {
	file, err := OpenFileGrantStore(t.TempDir()+"/grants.json", nil)
	require.NoError(t, err)
	for name, store := range map[string]GrantStore{"memory": NewMemoryGrantStore(), "file": file} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, store.Put(sampleGrant(), make([]byte, 10)))
		})
	}
}
