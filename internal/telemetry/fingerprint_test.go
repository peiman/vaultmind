package telemetry_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// EnsureFingerprint generates a fingerprint on first call. The
// generated value is a 32-char hex string and is persisted at
// .vaultmind/fingerprint.txt so subsequent calls return the same
// value — the federated telemetry pipeline depends on this stability
// to group records by vault across uploads.
func TestEnsureFingerprint_GeneratesAndPersists(t *testing.T) {
	v := t.TempDir()

	first, err := telemetry.EnsureFingerprint(v)
	require.NoError(t, err)
	assert.Len(t, first, 32, "fingerprint must be 32 hex chars")

	// File on disk
	body, err := os.ReadFile(filepath.Join(v, telemetry.FingerprintFile))
	require.NoError(t, err)
	assert.Contains(t, string(body), first)

	// Subsequent call returns the same fingerprint
	second, err := telemetry.EnsureFingerprint(v)
	require.NoError(t, err)
	assert.Equal(t, first, second, "fingerprint must be stable across calls")
}

// Two vaults must get distinct fingerprints — that's the entire
// point of per-vault grouping. If they collided silently, the
// federated aggregator would conflate users' data.
func TestEnsureFingerprint_DistinctPerVault(t *testing.T) {
	a := t.TempDir()
	b := t.TempDir()

	fpA, err := telemetry.EnsureFingerprint(a)
	require.NoError(t, err)
	fpB, err := telemetry.EnsureFingerprint(b)
	require.NoError(t, err)

	assert.NotEqual(t, fpA, fpB, "distinct vaults must have distinct fingerprints")
}

// A corrupt fingerprint file (truncated, garbage, mixed case) must be
// regenerated — silently propagating garbage to the telemetry pipeline
// would taint the dataset.
func TestEnsureFingerprint_RegeneratesOnCorruption(t *testing.T) {
	v := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(v, ".vaultmind"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(v, telemetry.FingerprintFile), []byte("not-a-valid-hex-string\n"), 0o600))

	fp, err := telemetry.EnsureFingerprint(v)
	require.NoError(t, err)
	assert.Len(t, fp, 32)
	assert.NotEqual(t, "not-a-valid-hex-string", fp)
	// And the file got rewritten with the new value
	body, err := os.ReadFile(filepath.Join(v, telemetry.FingerprintFile))
	require.NoError(t, err)
	assert.Contains(t, string(body), fp)
}

// Whitespace around the fingerprint (e.g. trailing newline that the
// writer produces) must be tolerated on read — otherwise a freshly
// written fingerprint fails its own validation on the next call.
func TestEnsureFingerprint_TolerantOfTrailingWhitespace(t *testing.T) {
	v := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(v, ".vaultmind"), 0o750))

	canonical := "0123456789abcdef0123456789abcdef"
	// Write with extra newlines and trailing spaces
	require.NoError(t, os.WriteFile(filepath.Join(v, telemetry.FingerprintFile), []byte("\n  "+canonical+"  \n\n"), 0o600))

	got, err := telemetry.EnsureFingerprint(v)
	require.NoError(t, err)
	assert.Equal(t, canonical, got)
}

// Various invalid hex shapes must be rejected (forcing regen) — uppercase
// hex, special chars, partial hex. Short circuits caught by length check
// are tested elsewhere; this exercises the per-character validation.
func TestEnsureFingerprint_RejectsInvalidHexShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"uppercase", "0123456789ABCDEF0123456789abcdef"},
		{"non-hex char", "0123456789xbcdef0123456789abcdef"},
		{"all zeroes ascii but with G", "g123456789abcdef0123456789abcdef"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(v, ".vaultmind"), 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(v, telemetry.FingerprintFile), []byte(tc.body), 0o600))
			fp, err := telemetry.EnsureFingerprint(v)
			require.NoError(t, err)
			assert.NotEqual(t, tc.body, fp, "invalid fingerprint must be regenerated, not preserved")
			assert.Len(t, fp, 32)
		})
	}
}

// EnsureFingerprint surfaces an error rather than silently regenerating
// when the on-disk file can't be read for a reason other than
// not-existing (e.g. permission denied). On macOS we simulate this by
// pointing the vault at a path where .vaultmind/fingerprint.txt is a
// directory, not a file — os.ReadFile returns a non-NotExist error.
func TestEnsureFingerprint_ReadErrorSurfaced(t *testing.T) {
	v := t.TempDir()
	// Create the fingerprint path AS A DIRECTORY — reading it fails
	// with EISDIR (or similar), which is not os.IsNotExist.
	require.NoError(t, os.MkdirAll(filepath.Join(v, telemetry.FingerprintFile), 0o750))

	_, err := telemetry.EnsureFingerprint(v)
	require.Error(t, err)
}
