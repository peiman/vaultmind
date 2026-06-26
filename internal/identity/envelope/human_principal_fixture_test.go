package envelope

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHumanPrincipal_CrossLangFixture binds the Go canonicalize to the shared
// cross-language SSOT. The fixture (testdata/human_principal_cross_language_vectors.json)
// is vendored byte-identical into agent-chat, where the Rust canonicalize_envelope
// asserts the same canonical bytes + sha256. A drift on EITHER side — the fixture,
// the Go output, or the Rust output — fails CI. This is the S3 dual-pin as a gate,
// same discipline as strict_verify_cross_language_vectors.json.
func TestHumanPrincipal_CrossLangFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean("testdata/human_principal_cross_language_vectors.json"))
	require.NoError(t, err, "read cross-language fixture")
	var fx struct {
		Vectors []struct {
			Name      string `json:"name"`
			Canonical string `json:"canonical"`
			SHA256    string `json:"sha256"`
		} `json:"vectors"`
	}
	require.NoError(t, json.Unmarshal(raw, &fx), "parse cross-language fixture")
	require.NotEmpty(t, fx.Vectors, "fixture must contain vectors")

	v := fx.Vectors[0]
	assert.Equal(t, "bridge_attested_chat", v.Name)
	// Fixture agrees with the Go SSOT consts...
	assert.Equal(t, frozenCanonical, v.Canonical, "fixture canonical drifted from the Go SSOT")
	assert.Equal(t, frozenSHA256, v.SHA256, "fixture sha256 drifted from the Go pin")
	// ...and with live CanonicalizeEnvelope output (Go produces the fixture bytes).
	canonical, err := CanonicalizeEnvelope(frozenFields())
	require.NoError(t, err)
	assert.Equal(t, v.Canonical, string(canonical.Bytes()), "Go canonicalize drifted from the fixture")
	sum := sha256.Sum256(canonical.Bytes())
	assert.Equal(t, v.SHA256, hex.EncodeToString(sum[:]), "Go sha256 drifted from the fixture pin")
}
