package hookscripts_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Knowledge vaults run the reach hook too (the knowledge profile includes it),
// and they are not identity vaults. Writing a note to a project's knowledge
// base must not bring up "arc discipline … never silently rewrite identity";
// what helps there is what the vault already says about the topic being
// written, and its conventions. focalc (the first knowledge-vault adopter)
// also configures its vault as a RELATIVE path, ./vaultmind-vault, which the
// absolute file_path of an Edit/Write never matched.

func knowledgeEnv(t *testing.T) (hookEnv, []string) {
	t.Helper()
	h := newHookEnv(t, echoStub)
	require.NoError(t, os.MkdirAll(filepath.Join(h.projectDir, "vaultmind-vault", "concepts"), 0o750))
	return h, append(h.env(false), "VAULTMIND_VAULT=./vaultmind-vault")
}

func TestReachHook_WritingToARelativeKnowledgeVaultFiresWithItsTopic(t *testing.T) {
	h, env := knowledgeEnv(t)
	note := filepath.Join(h.projectDir, "vaultmind-vault", "concepts", "identifiers-and-keys.md")

	out, _ := runHookScript(t, "vault-reach.sh", env, filePayload("Write", note))
	assert.Contains(t, out, "identifiers and keys", "the query names the topic being written")
	assert.Contains(t, out, "already", "it asks what the vault already says")
	assert.NotContains(t, out, identityQuery, "a knowledge vault is not an identity vault")
}

func TestReachHook_AShellWriteToAKnowledgeVaultUsesItsConventions(t *testing.T) {
	_, env := knowledgeEnv(t)
	out, _ := runHookScript(t, "vault-reach.sh", env, bashPayload("echo x > vaultmind-vault/concepts/x.md"))
	assert.Contains(t, out, "knowledge base")
	assert.NotContains(t, out, identityQuery)
}

func TestReachHook_ReadingAKnowledgeVaultIsSilent(t *testing.T) {
	_, env := knowledgeEnv(t)
	out, _ := runHookScript(t, "vault-reach.sh", env, bashPayload("cat vaultmind-vault/concepts/x.md"))
	assert.Empty(t, out)
}
