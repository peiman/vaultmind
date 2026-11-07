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
// The messages tests assert against are EXPORTED so callers/tests reference the
// single definition (no duplicated literals) when distinguishing which rule
// fired; purely-internal messages stay unexported.
const (
	errSchemaInvalidJSON = "identity: schema: invalid JSON"
	errSchemaInvalidUTF8 = "identity: schema: string value is not valid UTF-8"
	errSchemaTooDeep     = "identity: schema: nesting exceeds maximum depth"

	// ErrSchemaFloatNumber is the message for a non-integer (float) number.
	ErrSchemaFloatNumber = "identity: schema: non-integer number not allowed"
	// ErrSchemaIntTooLarge is the message for an integer whose magnitude exceeds 2^53.
	ErrSchemaIntTooLarge = "identity: schema: integer exceeds 2^53"
	// ErrSchemaNonASCIIKey is the message for a non-ASCII object key.
	ErrSchemaNonASCIIKey = "identity: schema: object key must be ASCII"
	// ErrSchemaDuplicateKey is the message for a duplicate object key.
	ErrSchemaDuplicateKey = "identity: schema: duplicate object key"
	// ErrSchemaTrailingInput is the message for trailing data after the JSON value.
	ErrSchemaTrailingInput = "identity: schema: trailing data after JSON value"
	// ErrSchemaNotNFC is the message for a string (value or key) that is not Unicode NFC.
	ErrSchemaNotNFC = "identity: schema: string must be Unicode NFC-normalized"
	// ErrSchemaNotObject is the message for a top-level document that is not a
	// JSON object. Contract-B entries are objects; a bare scalar/array is rejected.
	ErrSchemaNotObject = "identity: schema: top-level value must be a JSON object"
)

// ValidateSchema is the Contract-B validation gate. Contract-B entries are JSON
// OBJECTS, so it first rejects any top-level value that is not an object, then
// walks the entire document and rejects anything that would break
// cross-language, JCS-safe signing:
//
//   - a top-level value that is not a JSON object (bare scalar or array),
//   - non-integer numbers (floats such as 1.0 or 1e3),
//   - integers strictly greater than 2^53,
//   - non-ASCII object keys (this rule also subsumes NFC for keys, since every
//     non-NFC codepoint is non-ASCII),
//   - string values that are not valid UTF-8,
//   - string values that are not Unicode NFC-normalized,
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

	// Contract-B entries are JSON OBJECTS. Reject a bare top-level scalar
	// (null/true/42/"…") or array before walking: such a document is valid JSON
	// but not a conformant entry. Read the first token and require '{'.
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("%s: %w", errSchemaInvalidJSON, err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("%s", ErrSchemaNotObject)
	}
	// The opening '{' is consumed; walk the object body at depth 1.
	if err := walkObject(dec, 1); err != nil {
		return err
	}

	// Reject trailing data (e.g. two concatenated objects).
	if dec.More() {
		return fmt.Errorf("%s", ErrSchemaTrailingInput)
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
			return fmt.Errorf("%s: %q", ErrSchemaNotNFC, t)
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
		// Keys are ASCII-only. This rule subsumes the NFC requirement for keys:
		// every non-NFC codepoint is non-ASCII, so it is rejected here before
		// any NFC check could run. A separate isNFC(key) check would therefore
		// be unreachable dead code (per .claude/rules/unused-vars.md, confirmed
		// dead by the schema's own ASCII-key rule, not blindly deleted). The
		// string-VALUE NFC check in walkValue remains — it IS reachable.
		if !isASCII(key) {
			return fmt.Errorf("%s: %q", ErrSchemaNonASCIIKey, key)
		}
		if _, dup := seen[key]; dup {
			return fmt.Errorf("%s: %q", ErrSchemaDuplicateKey, key)
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
		return fmt.Errorf("%s: %s", ErrSchemaFloatNumber, s)
	}
	iv, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		// Out of int64 range — definitely larger than 2^53.
		return fmt.Errorf("%s: %s", ErrSchemaIntTooLarge, s)
	}
	if absInt64(iv) > maxSafeInteger {
		return fmt.Errorf("%s: %s", ErrSchemaIntTooLarge, s)
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

// registrySchemaMaxDepth matches chat-core validate_registry_schema (MAX_DEPTH).
// A registry is a few levels deep; the cap is the Rust limit, not ValidateSchema's 32.
const registrySchemaMaxDepth = 16

// ValidateRegistrySchema is the sign-path gate for a Contract B registry document.
// It is the part of ValidateSchema that validate_registry_schema also enforces,
// plus duplicate object keys:
//
//   - the top-level value is a JSON object
//   - object keys are ASCII-only
//   - every string value is Unicode NFC (checked, not rewritten)
//   - duplicate object keys are rejected
//   - nesting deeper than 16 is rejected
//   - the document is valid UTF-8 and has no trailing data
//
// Numbers, booleans, and null are allowed. Epoch and key_epoch range checks stay
// with the sign path's existing gate. ValidateSchema's blanket ±2^53 cap and its
// rejection of integral tokens such as 1.0 and 1e3 are not applied: Rust accepts
// any other i64 or u64, and there is no envelope key allowlist to reuse.
//
// Duplicate keys are intentional even though the Rust verifier merges them.
// encoding/json keeps the last value, so an unsigned sign-registry input must
// fail before decode. Do not use this on VerifyAndLoad: a signed body Rust
// accepts after that merge must still load.
func ValidateRegistrySchema(jsonBytes []byte) error {
	if !utf8.Valid(jsonBytes) {
		return fmt.Errorf("%s", errSchemaInvalidUTF8)
	}
	dec := json.NewDecoder(bytes.NewReader(jsonBytes))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("%s: %w", errSchemaInvalidJSON, err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return fmt.Errorf("%s", ErrSchemaNotObject)
	}
	if err := walkRegistryObject(dec, 1); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("%s", ErrSchemaTrailingInput)
	}
	return nil
}

func walkRegistryValue(dec *json.Decoder, depth int) error {
	if depth > registrySchemaMaxDepth {
		return fmt.Errorf("%s", errSchemaTooDeep)
	}
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("%s: %w", errSchemaInvalidJSON, err)
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			return walkRegistryObject(dec, depth+1)
		case '[':
			return walkRegistryArray(dec, depth+1)
		default:
			return fmt.Errorf("%s: unexpected %q", errSchemaInvalidJSON, t)
		}
	case string:
		if !isNFC(t) {
			return fmt.Errorf("%s: %q", ErrSchemaNotNFC, t)
		}
		return nil
	case json.Number, bool, nil:
		return nil
	default:
		return fmt.Errorf("%s: unexpected token %T", errSchemaInvalidJSON, tok)
	}
}

func walkRegistryObject(dec *json.Decoder, depth int) error {
	if depth > registrySchemaMaxDepth {
		return fmt.Errorf("%s", errSchemaTooDeep)
	}
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
			return fmt.Errorf("%s: %q", ErrSchemaNonASCIIKey, key)
		}
		if _, dup := seen[key]; dup {
			return fmt.Errorf("%s: %q", ErrSchemaDuplicateKey, key)
		}
		seen[key] = struct{}{}
		if err := walkRegistryValue(dec, depth); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("%s: %w", errSchemaInvalidJSON, err)
	}
	return nil
}

// ValidateDocumentNFC rejects a JSON document that contains a string value
// which is not Unicode NFC. encoding/json merges a duplicated key, the same
// way serde_json::Value does, and this function does not reject duplicates.
// The sign path uses ValidateRegistrySchema for that. Verify uses this so a
// signed body Rust accepts after the merge still loads, while a non-NFC string
// anywhere in the authenticated document is refused.
func ValidateDocumentNFC(jsonBytes []byte) error {
	var v any
	if err := json.Unmarshal(jsonBytes, &v); err != nil {
		return fmt.Errorf("%s: %w", errSchemaInvalidJSON, err)
	}
	return walkDocumentNFC(v, 0)
}

func walkDocumentNFC(v any, depth int) error {
	if depth > registrySchemaMaxDepth {
		return fmt.Errorf("%s", errSchemaTooDeep)
	}
	switch t := v.(type) {
	case map[string]any:
		for _, child := range t {
			if err := walkDocumentNFC(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range t {
			if err := walkDocumentNFC(child, depth+1); err != nil {
				return err
			}
		}
	case string:
		if !isNFC(t) {
			return fmt.Errorf("%s: %q", ErrSchemaNotNFC, t)
		}
	}
	return nil
}

func walkRegistryArray(dec *json.Decoder, depth int) error {
	if depth > registrySchemaMaxDepth {
		return fmt.Errorf("%s", errSchemaTooDeep)
	}
	for dec.More() {
		if err := walkRegistryValue(dec, depth); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("%s: %w", errSchemaInvalidJSON, err)
	}
	return nil
}
