package hookscripts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reach hook's identity trigger exists for one moment: writing to the
// identity vault, where arc discipline applies. It fired instead on every
// shell command that merely named the vault — reads included — and never on
// Edit or Write, the tools arcs are actually written with. Measured 2026-09-25:
// a pilot agent reported "it fired the same five results before every command
// … while I was only reading", and the same block preceded every read in the
// session that wrote three arcs through Write without it firing once.

// echoStub answers `ask` with the query it was asked, so a test can see which
// reminder the hook chose.
const echoStub = "#!/bin/bash\nif [ \"$1\" = ask ]; then echo \"  0.42  q  $2\"; fi\nexit 0\n"

const identityQuery = "writing to my identity vault"

// identityEnv is a hook environment whose vault is an identity vault — it has
// arcs/, which `vaultmind init` creates and a knowledge vault does not.
func identityEnv(t *testing.T) hookEnv {
	t.Helper()
	h := newHookEnv(t, echoStub)
	require.NoError(t, os.MkdirAll(filepath.Join(h.projectDir, "vaultmind-identity", "arcs"), 0o750))
	return h
}

func bashPayload(cmd string) string {
	b, _ := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": cmd}})
	return string(b)
}

func filePayload(tool, path string) string {
	b, _ := json.Marshal(map[string]any{"tool_name": tool, "tool_input": map[string]any{"file_path": path}})
	return string(b)
}

func TestReachHook_ReadingTheIdentityVaultIsSilent(t *testing.T) {
	h := identityEnv(t)
	for _, cmd := range []string{
		"vaultmind note get arc-x --vault vaultmind-identity 2>&1 | head -5",
		"cat vaultmind-identity/arcs/x.md",
		"grep -rn push vaultmind-identity/arcs",
		"ls vaultmind-identity/episodes > /dev/null",
		"vaultmind index --vault vaultmind-identity --embed",
		// A search word that happens to be a mutation verb is still a search.
		"vaultmind search fix --vault vaultmind-identity",
		"vaultmind ask set --vault vaultmind-identity",
		"cd vaultmind-identity && cat arcs/x.md",
	} {
		out, _ := runHookScript(t, "vault-reach.sh", h.env(false), bashPayload(cmd))
		assert.Emptyf(t, out, "a read must not fire the identity-write reminder: %s", cmd)
	}
}

func TestReachHook_WritingToTheIdentityVaultFromTheShellFires(t *testing.T) {
	h := identityEnv(t)
	for _, cmd := range []string{
		"echo x > vaultmind-identity/arcs/x.md",
		"cat a >> vaultmind-identity/arcs/x.md",
		"sed -i '' 's/a/b/' vaultmind-identity/arcs/x.md",
		"git add vaultmind-identity/arcs/x.md",
		"rm vaultmind-identity/arcs/x.md",
		"mv draft.md vaultmind-identity/arcs/x.md",
		"cp draft.md vaultmind-identity/arcs/x.md",
		"printf x | tee vaultmind-identity/arcs/x.md",
		"vaultmind frontmatter set arc-x title Y --vault vaultmind-identity",
		// Writes that no longer name the vault in the writing segment (review, 2026-09-25).
		"cd vaultmind-identity && rm arcs/x.md",
		"cd /abs/vaultmind-identity && git add arcs/x.md",
		"(cd vaultmind-identity && echo x > arcs/x.md)",
		"git -C vaultmind-identity add .",
		// Prefixed and wrapped commands.
		"sudo rm vaultmind-identity/x.md",
		"FOO=1 rm vaultmind-identity/x.md",
		"env FOO=1 rm vaultmind-identity/x.md",
		"command rm vaultmind-identity/x.md",
		// The CLI's own note-changing commands.
		"vaultmind note create arcs/x.md --type arc --vault vaultmind-identity",
		"vaultmind apply plan.json --vault vaultmind-identity",
		"vaultmind dataview render arc-x --vault vaultmind-identity",
		"vaultmind doctor heal --vault vaultmind-identity",
		"vaultmind arc review --vault vaultmind-identity --mark-reviewed=episode-x",
		"/usr/local/bin/vaultmind frontmatter unset arc-x tags --vault vaultmind-identity",
	} {
		out, _ := runHookScript(t, "vault-reach.sh", h.env(false), bashPayload(cmd))
		assert.Containsf(t, out, identityQuery, "a write must fire the identity-write reminder: %s", cmd)
	}
}

func TestReachHook_EditOrWriteInsideTheIdentityVaultFires(t *testing.T) {
	h := identityEnv(t)
	inside := filepath.Join(h.projectDir, "vaultmind-identity", "arcs", "x.md")
	outside := filepath.Join(h.projectDir, "docs", "x.md")
	for _, tool := range []string{"Write", "Edit", "MultiEdit"} {
		out, _ := runHookScript(t, "vault-reach.sh", h.env(false), filePayload(tool, inside))
		assert.Containsf(t, out, identityQuery, "%s into the identity vault must fire", tool)

		out, _ = runHookScript(t, "vault-reach.sh", h.env(false), filePayload(tool, outside))
		assert.Emptyf(t, out, "%s outside the identity vault must not fire", tool)
	}
}

// A sibling directory that shares the vault's name as a prefix is not the vault.
func TestReachHook_ASiblingWithTheVaultsNameAsPrefixIsNotTheVault(t *testing.T) {
	h := identityEnv(t)
	sibling := filepath.Join(h.projectDir, "vaultmind-identity-backup", "x.md")
	out, _ := runHookScript(t, "vault-reach.sh", h.env(false), filePayload("Write", sibling))
	assert.Empty(t, out)
}

// A vault path set with a trailing slash left the vault's name empty, and an
// empty name matched every command: every rm anywhere became an identity write.
func TestReachHook_ATrailingSlashOnTheVaultPathChangesNothing(t *testing.T) {
	h := identityEnv(t)
	vault := filepath.Join(h.projectDir, "vaultmind-identity")
	env := append(h.env(false), "VAULTMIND_VAULT="+vault+"/")

	out, _ := runHookScript(t, "vault-reach.sh", env, bashPayload("rm /tmp/unrelated.txt"))
	assert.Empty(t, out, "a write outside the vault is not an identity write")

	out, _ = runHookScript(t, "vault-reach.sh", env, filePayload("Write", filepath.Join(vault, "arcs", "x.md")))
	assert.Contains(t, out, identityQuery, "a Write inside the vault still fires")
}

// Inside the vault, a redirect to an absolute path elsewhere writes elsewhere.
func TestReachHook_ARedirectOutOfTheVaultIsNotAVaultWrite(t *testing.T) {
	h := identityEnv(t)
	out, _ := runHookScript(t, "vault-reach.sh", h.env(false), bashPayload("cd vaultmind-identity && cat arcs/x.md > /tmp/out.md"))
	assert.Empty(t, out)
	out, _ = runHookScript(t, "vault-reach.sh", h.env(false), bashPayload("cd vaultmind-identity && echo x > arcs/x.md"))
	assert.Contains(t, out, identityQuery, "a relative redirect inside the vault still is one")
}
