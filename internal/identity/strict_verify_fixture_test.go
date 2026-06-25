package identity_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixturePath is the cross-language strict-verify SSOT. It is vendored
// byte-identical into agent-chat, where the Rust verifier (ed25519-dalek
// verify_strict) runs the SAME bytes. Agreement on both sides = the trust-root
// seam is closed.
const fixturePath = "testdata/strict_verify_cross_language_vectors.json"

// fixtureSHA256 is the pinned hash — the vaultmind-oss half of the cross-repo
// sync-gate. agent-chat pins the same value (the files are byte-identical). A
// silent edit to the fixture in EITHER repo fails CI until both copies + both
// pins are re-synced. (Manifesto P9 — automated enforcement of the SSOT.)
const fixtureSHA256 = "638a746bb61ef91cf96a3207925c4d6d0b6b5e5c1de0fc372010b3e9b9a6a7f0"

type crossLangVec struct {
	Name   string `json:"name"`
	Note   string `json:"note"`
	MsgHex string `json:"msg_hex"`
	PubHex string `json:"pubkey_hex"`
	SigHex string `json:"sig_hex"`
	Expect string `json:"expect"`
}

func loadCrossLangFixture(t *testing.T) ([]byte, []crossLangVec) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(fixturePath))
	require.NoError(t, err, "read cross-language fixture")
	var fx struct {
		Vectors []crossLangVec `json:"vectors"`
	}
	require.NoError(t, json.Unmarshal(raw, &fx), "parse cross-language fixture")
	require.NotEmpty(t, fx.Vectors, "fixture must contain vectors")
	return raw, fx.Vectors
}

// TestStrictVerify_CrossLangFixture binds the Go verifier to the shared SSOT:
// VerifyCanonical must produce the listed verdict for every fixture vector. The
// Rust verifier runs these exact bytes; matching on both sides closes the seam.
func TestStrictVerify_CrossLangFixture(t *testing.T) {
	_, vecs := loadCrossLangFixture(t)
	for _, v := range vecs {
		v := v
		t.Run(v.Name, func(t *testing.T) {
			pub := ed25519.PublicKey(mustDecodeHex(t, v.PubHex))
			canonical := identity.CanonicalBytesFromTrusted(mustDecodeHex(t, v.MsgHex))
			sig := mustDecodeHex(t, v.SigHex)
			ok, err := identity.VerifyCanonical(pub, canonical, sig)
			switch v.Expect {
			case "accept":
				require.NoError(t, err, v.Note)
				assert.True(t, ok, v.Note)
			case "reject":
				assert.False(t, ok, v.Note)
				require.Error(t, err, "a strict reject must be a STRUCTURAL reject (non-nil error): "+v.Note)
			default:
				t.Fatalf("vector %q has unknown expect %q", v.Name, v.Expect)
			}
		})
	}
}

// TestStrictVerify_FixturePinned is the cross-repo drift gate: the fixture's hash
// must equal the pin. If the fixture changes in either repo without re-syncing
// both copies + both pins, this fails — the SSOT cannot silently diverge.
func TestStrictVerify_FixturePinned(t *testing.T) {
	raw, _ := loadCrossLangFixture(t)
	sum := sha256.Sum256(raw)
	assert.Equal(t, fixtureSHA256, hex.EncodeToString(sum[:]),
		"cross-language fixture changed without updating the pin — re-sync agent-chat + vaultmind-oss, then update both pins")
}
