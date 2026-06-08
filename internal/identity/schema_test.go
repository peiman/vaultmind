package identity_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSchema_AcceptsValidEntry(t *testing.T) {
	require.NoError(t, identity.ValidateSchema([]byte(frozenInputJSON)),
		"the frozen valid entry must pass the Contract-B gate")
}

func TestValidateSchema_RejectsFloatDecimal(t *testing.T) {
	err := identity.ValidateSchema([]byte(`{"key_epoch":1.0}`))
	assert.Error(t, err, "a decimal float (1.0) must be rejected")
}

func TestValidateSchema_RejectsFloatExponent(t *testing.T) {
	err := identity.ValidateSchema([]byte(`{"key_epoch":1e3}`))
	assert.Error(t, err, "an exponent float (1e3) must be rejected")
}

func TestValidateSchema_RejectsNegativeFloat(t *testing.T) {
	err := identity.ValidateSchema([]byte(`{"valid_from":-1.5}`))
	assert.Error(t, err, "a negative float must be rejected")
}

func TestValidateSchema_RejectsIntAbove2Pow53(t *testing.T) {
	// 2^53 + 1 = 9007199254740993, the first integer not exactly
	// representable as a float64 — Contract B forbids it.
	err := identity.ValidateSchema([]byte(`{"valid_until":9007199254740993}`))
	assert.Error(t, err, "an integer above 2^53 must be rejected")
}

func TestValidateSchema_AcceptsIntAt2Pow53(t *testing.T) {
	// 2^53 = 9007199254740992 is the boundary and is allowed.
	require.NoError(t, identity.ValidateSchema([]byte(`{"valid_until":9007199254740992}`)),
		"the integer exactly equal to 2^53 must be accepted")
}

func TestValidateSchema_RejectsNonASCIIObjectKey(t *testing.T) {
	err := identity.ValidateSchema([]byte(`{"naïve":1}`))
	assert.Error(t, err, "a non-ASCII object key must be rejected")
}

func TestValidateSchema_AcceptsNonASCIIStringValue(t *testing.T) {
	// String VALUES may contain non-ASCII (the star is UTF-8); only KEYS
	// are restricted to ASCII.
	require.NoError(t, identity.ValidateSchema([]byte(`{"display_name":"Mira ⭐"}`)),
		"non-ASCII string values must be allowed (UTF-8 required, not ASCII)")
}

func TestValidateSchema_RejectsInvalidUTF8StringValue(t *testing.T) {
	// 0xFF is never valid UTF-8.
	bad := []byte(`{"display_name":"` + "\xff" + `"}`)
	err := identity.ValidateSchema(bad)
	assert.Error(t, err, "an invalid-UTF-8 string value must be rejected")
}

func TestValidateSchema_RejectsDuplicateKeys(t *testing.T) {
	err := identity.ValidateSchema([]byte(`{"slug":"a","slug":"b"}`))
	assert.Error(t, err, "duplicate object keys must be rejected (ambiguous, JCS-unsafe)")
}

func TestValidateSchema_WalksNestedObjects(t *testing.T) {
	// A float buried in a nested object must still be caught.
	err := identity.ValidateSchema([]byte(`{"meta":{"weight":2.5}}`))
	assert.Error(t, err, "a float nested inside an object must be rejected")
}

func TestValidateSchema_WalksArrays(t *testing.T) {
	// A bad integer inside an array must be caught.
	err := identity.ValidateSchema([]byte(`{"vals":[1,9007199254740993]}`))
	assert.Error(t, err, "an out-of-range integer inside an array must be rejected")
}

func TestValidateSchema_RejectsInvalidJSON(t *testing.T) {
	err := identity.ValidateSchema([]byte(`{broken`))
	assert.Error(t, err)
}

func TestValidateSchema_AcceptsNestedASCIIAndUTF8(t *testing.T) {
	require.NoError(t, identity.ValidateSchema(
		[]byte(`{"outer":{"inner":"valid ⭐ value","n":42},"list":["x","y"]}`)),
		"a well-formed nested entry must pass")
}

func TestValidateSchema_RejectsIntegerBeyondInt64(t *testing.T) {
	// Far beyond int64 max — ParseInt fails, must still be rejected as too large.
	err := identity.ValidateSchema([]byte(`{"n":99999999999999999999}`))
	assert.Error(t, err, "an integer beyond int64 range must be rejected as too large")
}

func TestValidateSchema_RejectsNegativeIntBelowBound(t *testing.T) {
	// -(2^53 + 1) — magnitude exceeds the safe bound, must be rejected.
	err := identity.ValidateSchema([]byte(`{"n":-9007199254740993}`))
	assert.Error(t, err, "a negative integer below -2^53 must be rejected")
}

func TestValidateSchema_RejectsMinInt64(t *testing.T) {
	// math.MinInt64 parses as a valid int64 but its magnitude far exceeds
	// 2^53; the absInt64 overflow guard must classify it as too large.
	err := identity.ValidateSchema([]byte(`{"n":-9223372036854775808}`))
	assert.Error(t, err, "math.MinInt64 must be rejected as exceeding 2^53")
}

func TestValidateSchema_AcceptsNegativeIntWithinBound(t *testing.T) {
	require.NoError(t, identity.ValidateSchema([]byte(`{"n":-42}`)),
		"a small negative integer must be accepted")
}

func TestValidateSchema_AcceptsBoolAndNull(t *testing.T) {
	require.NoError(t, identity.ValidateSchema([]byte(`{"a":true,"b":false,"c":null}`)),
		"booleans and null must be accepted")
}

func TestValidateSchema_RejectsTrailingData(t *testing.T) {
	err := identity.ValidateSchema([]byte(`{"a":1}{"b":2}`))
	assert.Error(t, err, "trailing data after the first JSON value must be rejected")
}

func TestValidateSchema_RejectsTopLevelScalar_OK(t *testing.T) {
	// A bare top-level number is still a valid JSON value and must walk cleanly.
	require.NoError(t, identity.ValidateSchema([]byte(`42`)),
		"a bare in-range integer is a valid top-level value")
}

func TestValidateSchema_RejectsUnbalancedArray(t *testing.T) {
	err := identity.ValidateSchema([]byte(`{"a":[1,2`))
	assert.Error(t, err, "an unterminated array must be rejected")
}
