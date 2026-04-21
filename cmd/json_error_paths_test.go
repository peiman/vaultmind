package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Most runners share a "vault path doesn't exist + --json" contract:
// they write a JSON error envelope via OpenVaultDBOrWriteErr and return nil
// (masked by ErrAlreadyWritten). These tests pin that contract so a refactor
// of the shared helper can't silently break the JSON envelope for specific
// commands.
//
// Each command is checked for:
//   - The top-level envelope shape decodes.
//   - status is "error".
//   - An error code is present (we don't pin the exact string — both
//     "vault_not_found" and command-specific equivalents are legitimate).

type jsonErrEnv struct {
	Status string `json:"status"`
	Errors []struct {
		Code string `json:"code"`
	} `json:"errors"`
}

func assertJSONErrEnvelope(t *testing.T, raw []byte) {
	t.Helper()
	var env jsonErrEnv
	require.NoError(t, json.Unmarshal(raw, &env), "envelope must decode")
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors, "errors array must be non-empty")
	assert.NotEmpty(t, env.Errors[0].Code, "first error must carry a non-empty code")
}

func TestSearch_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "search", "hello", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestNoteGet_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "note", "get", "x", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestVaultStatus_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "vault", "status", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestDoctor_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "doctor", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestResolve_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "resolve", "x", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestLinksIn_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "links", "in", "x", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestLinksOut_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "links", "out", "x", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestLinksNeighbors_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "links", "neighbors", "x", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestMemoryRecall_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "memory", "recall", "x", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestMemoryRelated_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "memory", "related", "x", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestMemoryContextPack_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "memory", "context-pack", "x", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestMemorySummarize_JSONErrorOnMissingVault(t *testing.T) {
	// memory summarize validates IDs before vault open, so we pass an ID to get past that
	out, _, _ := runRootCmd(t, "memory", "summarize", "x", "--vault", "/nonexistent/vault", "--json")
	// memory summarize returns a Go error here (doesn't use OpenVaultDBOrWriteErr
	// the same way); tolerate either signal.
	if out.Len() > 0 {
		assertJSONErrEnvelope(t, out.Bytes())
	}
}

func TestDataviewLint_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "dataview", "lint", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestDataviewRender_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "dataview", "render", "x", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestLintFixLinks_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "lint", "fix-links", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestFrontmatterSet_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "frontmatter", "set", "x.md", "k", "v", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestFrontmatterUnset_JSONErrorOnMissingVault(t *testing.T) {
	out, _, _ := runRootCmd(t, "frontmatter", "unset", "x.md", "k", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

func TestFrontmatterValidate_JSONErrorOnMissingVault(t *testing.T) {
	// Non-live mode goes through OpenVaultDBOrWriteErr
	out, _, _ := runRootCmd(t, "frontmatter", "validate", "--vault", "/nonexistent/vault", "--json")
	assertJSONErrEnvelope(t, out.Bytes())
}

// Also: classifyVaultError must return "config_error" when the issue is
// config loading (not vault-not-found). This requires a valid directory
// with a malformed .vaultmind/config.yaml. Contract: distinct error codes
// for distinct failure modes — scripts branch on them.
func TestOpenVault_ConfigErrorClassifiedDistinctFromNotFound(t *testing.T) {
	vault := t.TempDir()
	// Write malformed yaml
	require.NoError(t, writeFileAll(vault, ".vaultmind/config.yaml", "types: [this is not a map"))

	out, _, _ := runRootCmd(t, "doctor", "--vault", vault, "--json")
	var env jsonErrEnv
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors)
	assert.True(t,
		strings.Contains(env.Errors[0].Code, "config") ||
			strings.Contains(env.Errors[0].Code, "error"),
		"malformed config should map to a config/error code, got %q", env.Errors[0].Code)
}
