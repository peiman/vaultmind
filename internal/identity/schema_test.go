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
	require.ErrorContains(t, err, identity.ErrSchemaFloatNumber,
		"a decimal float (1.0) must be rejected by the float rule")
}

func TestValidateSchema_RejectsFloatExponent(t *testing.T) {
	err := identity.ValidateSchema([]byte(`{"key_epoch":1e3}`))
	require.ErrorContains(t, err, identity.ErrSchemaFloatNumber,
		"an exponent float (1e3) must be rejected by the float rule")
}

func TestValidateSchema_RejectsNegativeFloat(t *testing.T) {
	err := identity.ValidateSchema([]byte(`{"valid_from":-1.5}`))
	require.ErrorContains(t, err, identity.ErrSchemaFloatNumber,
		"a negative float must be rejected by the float rule")
}

func TestValidateSchema_RejectsIntAbove2Pow53(t *testing.T) {
	// 2^53 + 1 = 9007199254740993, the first integer not exactly
	// representable as a float64 — Contract B forbids it.
	err := identity.ValidateSchema([]byte(`{"valid_until":9007199254740993}`))
	require.ErrorContains(t, err, identity.ErrSchemaIntTooLarge,
		"an integer above 2^53 must be rejected by the int-range rule")
}

func TestValidateSchema_AcceptsIntAt2Pow53(t *testing.T) {
	// 2^53 = 9007199254740992 is the boundary and is allowed.
	require.NoError(t, identity.ValidateSchema([]byte(`{"valid_until":9007199254740992}`)),
		"the integer exactly equal to 2^53 must be accepted")
}

func TestValidateSchema_RejectsNonASCIIObjectKey(t *testing.T) {
	err := identity.ValidateSchema([]byte(`{"naïve":1}`))
	require.ErrorContains(t, err, identity.ErrSchemaNonASCIIKey,
		"a non-ASCII object key must be rejected by the ASCII-key rule")
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
	require.ErrorContains(t, err, identity.ErrSchemaDuplicateKey,
		"duplicate object keys must be rejected by the duplicate-key rule (ambiguous, JCS-unsafe)")
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
	require.ErrorContains(t, err, identity.ErrSchemaIntTooLarge,
		"an integer beyond int64 range must be rejected by the int-range rule")
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
	require.ErrorContains(t, err, identity.ErrSchemaTrailingInput,
		"trailing data after the first JSON value must be rejected by the trailing-data rule")
}

func TestValidateSchema_RejectsBareScalarRoot(t *testing.T) {
	// Contract-B entries are JSON OBJECTS. A bare top-level scalar (or array)
	// is a valid JSON value but not a conformant Contract-B entry, so it must
	// be rejected by the top-level-kind check before the walk.
	for _, doc := range []string{`null`, `true`, `false`, `42`, `"slug"`, `[1,2]`} {
		err := identity.ValidateSchema([]byte(doc))
		require.ErrorContains(t, err, identity.ErrSchemaNotObject,
			"a bare top-level non-object (%s) must be rejected", doc)
	}
}

func TestValidateSchema_AcceptsTopLevelObject(t *testing.T) {
	require.NoError(t, identity.ValidateSchema([]byte(`{"slug":"mira"}`)),
		"a top-level object must be accepted")
}

func TestValidateSchema_RejectsUnbalancedArray(t *testing.T) {
	err := identity.ValidateSchema([]byte(`{"a":[1,2`))
	assert.Error(t, err, "an unterminated array must be rejected")
}
