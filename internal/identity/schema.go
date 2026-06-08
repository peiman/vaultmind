package identity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// maxSafeInteger is 2^53, the largest integer exactly representable as an
// IEEE-754 float64. Contract B forbids integers strictly greater than this
// so entries survive a round-trip through any JSON parser that uses doubles.
const maxSafeInteger = int64(1) << 53

// maxDepth bounds object/array nesting so a deeply nested document cannot
// exhaust the goroutine stack via the recursive walk. 32 levels is far beyond
// any legitimate Contract-B entry.
const maxDepth = 32

// Validation error messages (SSOT — referenced from the walk, not inlined).
const (
	errSchemaInvalidJSON   = "identity: schema: invalid JSON"
	errSchemaFloatNumber   = "identity: schema: non-integer number not allowed"
	errSchemaIntTooLarge   = "identity: schema: integer exceeds 2^53"
	errSchemaNonASCIIKey   = "identity: schema: object key must be ASCII"
	errSchemaInvalidUTF8   = "identity: schema: string value is not valid UTF-8"
	errSchemaDuplicateKey  = "identity: schema: duplicate object key"
	errSchemaTrailingInput = "identity: schema: trailing data after JSON value"
	errSchemaNotNFC        = "identity: schema: string must be Unicode NFC-normalized"
	errSchemaTooDeep       = "identity: schema: nesting exceeds maximum depth"
)

// ValidateSchema is the Contract-B validation gate. It walks the entire JSON
// document and rejects anything that would break cross-language, JCS-safe
// signing:
//
//   - non-integer numbers (floats such as 1.0 or 1e3),
//   - integers strictly greater than 2^53,
//   - non-ASCII object keys,
//   - string values (and keys) that are not valid UTF-8,
//   - string values (and keys) that are not Unicode NFC-normalized,
//   - duplicate object keys (ambiguous after canonicalization),
//   - nesting deeper than maxDepth (DoS guard).
//
// It returns nil for a conformant document.
func ValidateSchema(jsonBytes []byte) error {
	// RFC 8259 requires JSON text to be UTF-8. Go's json.Decoder silently
	// replaces invalid UTF-8 inside string tokens with U+FFFD, which would
	// let a malformed value slip past the per-value check below — so reject
	// invalid UTF-8 at the document level first.
	if !utf8.Valid(jsonBytes) {
		return fmt.Errorf("%s", errSchemaInvalidUTF8)
	}

	dec := json.NewDecoder(bytes.NewReader(jsonBytes))
	dec.UseNumber()

	if err := walkValue(dec, 0); err != nil {
		return err
	}

	// Reject trailing data (e.g. two concatenated objects).
	if dec.More() {
		return fmt.Errorf("%s", errSchemaTrailingInput)
	}
	return nil
}

// walkValue consumes exactly one JSON value from the decoder, recursing into
// objects and arrays. depth is the current nesting level; it is checked before
// descending so a deeply nested document is rejected rather than overflowing
// the stack. The decoder's token stream guarantees that, for an object, keys
// and values alternate until the closing brace.
func walkValue(dec *json.Decoder, depth int) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("%s: %w", errSchemaInvalidJSON, err)
	}

	switch t := tok.(type) {
	case json.Delim:
		if depth >= maxDepth {
			return fmt.Errorf("%s", errSchemaTooDeep)
		}
		switch t {
		case '{':
			return walkObject(dec, depth+1)
		case '[':
			return walkArray(dec, depth+1)
		default:
			// A stray '}' or ']' here means malformed JSON.
			return fmt.Errorf("%s: unexpected %q", errSchemaInvalidJSON, t)
		}
	case json.Number:
		return validateNumber(t)
	case string:
		// String values may be non-ASCII; the document-level utf8.Valid check
		// in ValidateSchema already guarantees they are valid UTF-8. They must,
		// however, be Unicode NFC-normalized so the signed bytes are stable
		// across producers that normalize differently.
		if !isNFC(t) {
			return fmt.Errorf("%s: %q", errSchemaNotNFC, t)
		}
		return nil
	case bool, nil:
		return nil
	default:
		return fmt.Errorf("%s: unexpected token %T", errSchemaInvalidJSON, tok)
	}
}

// walkObject validates an object's keys and recurses into its values. The
// opening '{' has already been consumed; depth is the level of this object.
func walkObject(dec *json.Decoder, depth int) error {
	seen := make(map[string]struct{})
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("%s: %w", errSchemaInvalidJSON, err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("%s: object key not a string", errSchemaInvalidJSON)
		}
		if !isASCII(key) {
			return fmt.Errorf("%s: %q", errSchemaNonASCIIKey, key)
		}
		if !isNFC(key) {
			return fmt.Errorf("%s: %q", errSchemaNotNFC, key)
		}
		if _, dup := seen[key]; dup {
			return fmt.Errorf("%s: %q", errSchemaDuplicateKey, key)
		}
		seen[key] = struct{}{}

		if err := walkValue(dec, depth); err != nil {
			return err
		}
	}
	// Consume the closing '}'.
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("%s: %w", errSchemaInvalidJSON, err)
	}
	return nil
}

// walkArray recurses into each element of an array. The opening '[' has
// already been consumed; depth is the level of this array.
func walkArray(dec *json.Decoder, depth int) error {
	for dec.More() {
		if err := walkValue(dec, depth); err != nil {
			return err
		}
	}
	// Consume the closing ']'.
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("%s: %w", errSchemaInvalidJSON, err)
	}
	return nil
}

// validateNumber rejects floats and integers outside the safe range. A token
// counts as a float if it contains a '.', 'e', or 'E'. Integers must fit in
// [-2^53, 2^53].
func validateNumber(n json.Number) error {
	s := n.String()
	if strings.ContainsAny(s, ".eE") {
		return fmt.Errorf("%s: %s", errSchemaFloatNumber, s)
	}
	iv, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		// Out of int64 range — definitely larger than 2^53.
		return fmt.Errorf("%s: %s", errSchemaIntTooLarge, s)
	}
	if absInt64(iv) > maxSafeInteger {
		return fmt.Errorf("%s: %s", errSchemaIntTooLarge, s)
	}
	return nil
}

// isASCII reports whether s contains only bytes < 0x80.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// isNFC reports whether s is already in Unicode Normalization Form C. Contract
// B requires NFC so the canonical, signed bytes are stable across producers
// that would otherwise emit a different normalization of the same text.
func isNFC(s string) bool {
	return norm.NFC.IsNormalString(s)
}

// absInt64 returns the absolute value of v, guarding against the math.MinInt64
// overflow case (whose magnitude exceeds 2^53 anyway).
func absInt64(v int64) int64 {
	if v == math.MinInt64 {
		return math.MaxInt64
	}
	if v < 0 {
		return -v
	}
	return v
}
