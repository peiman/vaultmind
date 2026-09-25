package hookscripts_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A project can hold an identity vault AND knowledge vaults (the author's own:
// identity + a desk; an adopter: persona + project knowledge). The hook is
// given all of them in VAULTMIND_VAULTS but looked for writes only in the
// primary, so a write to any other vault brought up nothing. It now finds the
// vault being written, asks the question that fits THAT vault, and searches
// that vault alone.

// argsStub echoes every argument, so a test can see the question AND the vault.
const argsStub = "#!/bin/bash\nif [ \"$1\" = ask ]; then echo \"  0.42  q  $*\"; fi\nexit 0\n"

func mixedEnv(t *testing.T) (hookEnv, []string, string, string) {
	t.Helper()
	h := newHookEnv(t, argsStub)
	identity := filepath.Join(h.projectDir, "vaultmind-identity")
	knowledge := filepath.Join(h.projectDir, "project-kb")
	require.NoError(t, os.MkdirAll(filepath.Join(identity, "arcs"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(knowledge, "concepts"), 0o750))
	env := append(h.env(false),
		"VAULTMIND_VAULT="+identity,
		"VAULTMIND_VAULTS="+identity+","+knowledge)
	return h, env, identity, knowledge
}

func TestReachHook_AWriteToTheKnowledgeVaultOfAMixedProjectAsksThatVault(t *testing.T) {
	_, env, _, knowledge := mixedEnv(t)
	note := filepath.Join(knowledge, "concepts", "ledger-store.md")

	out, _ := runHookScript(t, "vault-reach.sh", env, filePayload("Write", note))
	assert.Contains(t, out, "ledger store", "the knowledge question, with the topic")
	assert.NotContains(t, out, identityQuery)
	assert.Contains(t, out, "--vault "+knowledge, "searched in the vault being written")
	assert.NotContains(t, out, "--vaults", "not across every vault")
}

func TestReachHook_AWriteToTheIdentityVaultOfAMixedProjectKeepsArcDiscipline(t *testing.T) {
	_, env, identity, _ := mixedEnv(t)
	out, _ := runHookScript(t, "vault-reach.sh", env, filePayload("Edit", filepath.Join(identity, "arcs", "x.md")))
	assert.Contains(t, out, identityQuery)
	assert.Contains(t, out, "--vault "+identity)
}

func TestReachHook_AShellWriteToTheKnowledgeVaultOfAMixedProjectIsSeen(t *testing.T) {
	_, env, _, knowledge := mixedEnv(t)
	out, _ := runHookScript(t, "vault-reach.sh", env, bashPayload("echo x > "+knowledge+"/concepts/x.md"))
	assert.Contains(t, out, "knowledge base")
	assert.Contains(t, out, "--vault "+knowledge)
}

// Commit and push still search every vault: they are not about one vault.
func TestReachHook_ACommitInAMixedProjectStillSearchesEveryVault(t *testing.T) {
	_, env, _, _ := mixedEnv(t)
	out, _ := runHookScript(t, "vault-reach.sh", env, bashPayload("git commit -m x"))
	assert.Contains(t, out, "--vaults")
}

// Desk and journal files are named date-first; the date is not the topic.
func TestReachHook_ALeadingDateIsNotPartOfTheTopic(t *testing.T) {
	_, env, _, knowledge := mixedEnv(t)
	note := filepath.Join(knowledge, "concepts", "2026-09-25-deferring-to-tomorrow.md")
	out, _ := runHookScript(t, "vault-reach.sh", env, filePayload("Write", note))
	assert.Contains(t, out, "writing about deferring to tomorrow:")
}

// Vaults are matched by path, not by a name found anywhere in the command:
// "kb" is not "kb-archive", and two vaults can share a directory name.
func TestReachHook_OverlappingVaultNamesTargetTheRightVault(t *testing.T) {
	h := newHookEnv(t, argsStub)
	kb := filepath.Join(h.projectDir, "kb")
	archive := filepath.Join(h.projectDir, "kb-archive")
	a := filepath.Join(h.projectDir, "a", "notes")
	b := filepath.Join(h.projectDir, "b", "notes")
	for _, d := range []string{kb, archive, a, b} {
		require.NoError(t, os.MkdirAll(d, 0o750))
	}
	env := append(h.env(false), "VAULTMIND_VAULT="+kb, "VAULTMIND_VAULTS="+kb+","+archive+","+a+","+b)

	out, _ := runHookScript(t, "vault-reach.sh", env, bashPayload("echo x > kb-archive/n.md"))
	assert.Contains(t, out, "--vault "+archive)

	out, _ = runHookScript(t, "vault-reach.sh", env, bashPayload("echo x > "+b+"/n.md"))
	assert.Contains(t, out, "--vault "+b)
}

// cp and mv write their LAST argument: copying into an identity vault is a
// write to it, whatever vault the source came from.
func TestReachHook_CopyingIntoAVaultTargetsTheDestination(t *testing.T) {
	_, env, identity, knowledge := mixedEnv(t)
	out, _ := runHookScript(t, "vault-reach.sh", env,
		bashPayload("cp "+knowledge+"/concepts/x.md "+identity+"/arcs/"))
	assert.Contains(t, out, identityQuery)
	assert.Contains(t, out, "--vault "+identity)
}

// The query must reach the vault being written even if the primary vault is
// not there.
func TestReachHook_AWriteToAVaultStillFiresWhenThePrimaryIsMissing(t *testing.T) {
	h := newHookEnv(t, argsStub)
	knowledge := filepath.Join(h.projectDir, "kb")
	require.NoError(t, os.MkdirAll(knowledge, 0o750))
	env := append(h.env(false), "VAULTMIND_VAULT="+filepath.Join(h.projectDir, "gone"), "VAULTMIND_VAULTS="+knowledge)
	out, _ := runHookScript(t, "vault-reach.sh", env, filePayload("Write", filepath.Join(knowledge, "x.md")))
	assert.Contains(t, out, "--vault "+knowledge)
}

// Relative paths resolve against the directory the command actually runs in,
// which the harness sends as "cwd" — not the project root.
func TestReachHook_RelativePathsResolveAgainstTheCommandsDirectory(t *testing.T) {
	_, env, identity, _ := mixedEnv(t)
	payload := `{"tool_name":"Bash","cwd":"` + identity + `","tool_input":{"command":"echo x > arcs/x.md"}}`
	out, _ := runHookScript(t, "vault-reach.sh", env, payload)
	assert.Contains(t, out, identityQuery)
}

// A vault nested inside another belongs to the innermost one.
func TestReachHook_ANestedVaultOwnsItsOwnWrites(t *testing.T) {
	h := newHookEnv(t, argsStub)
	outer := filepath.Join(h.projectDir, "outer")
	inner := filepath.Join(outer, "inner")
	require.NoError(t, os.MkdirAll(inner, 0o750))
	for _, order := range [][2]string{{outer, inner}, {inner, outer}} {
		env := append(h.env(false), "VAULTMIND_VAULT="+order[0], "VAULTMIND_VAULTS="+order[0]+","+order[1])
		out, _ := runHookScript(t, "vault-reach.sh", env, bashPayload("echo x > "+inner+"/n.md"))
		assert.Contains(t, out, "--vault "+inner, "whichever order the vaults are listed in")
	}
}

// Copying OUT of a vault writes elsewhere: only the destination counts.
func TestReachHook_CopyingOutOfAVaultIsNotAVaultWrite(t *testing.T) {
	_, env, _, knowledge := mixedEnv(t)
	out, _ := runHookScript(t, "vault-reach.sh", env, bashPayload("cp "+knowledge+"/concepts/x.md /tmp/"))
	assert.Empty(t, out)
}

// Edit and Write pick the innermost vault too, as shell writes do.
func TestReachHook_AWriteIntoANestedVaultTargetsTheInnermost(t *testing.T) {
	h := newHookEnv(t, argsStub)
	outer := filepath.Join(h.projectDir, "outer")
	inner := filepath.Join(outer, "inner")
	require.NoError(t, os.MkdirAll(inner, 0o750))
	env := append(h.env(false), "VAULTMIND_VAULT="+outer, "VAULTMIND_VAULTS="+outer+","+inner)
	out, _ := runHookScript(t, "vault-reach.sh", env, filePayload("Write", filepath.Join(inner, "n.md")))
	assert.Contains(t, out, "--vault "+inner)
}

// Redirects are not arguments: "cp x kb/ 2>/dev/null" copies into kb. And tee
// writes to every file it is given, not only the last.
func TestReachHook_RedirectsAndTeeTargetsDoNotHideAWrite(t *testing.T) {
	_, env, _, knowledge := mixedEnv(t)
	for _, cmd := range []string{
		"cp /tmp/x.md " + knowledge + "/ 2>/dev/null",
		"echo x | tee -a " + knowledge + "/a.md >/dev/null",
		"tee " + knowledge + "/a.md /tmp/o",
		"cp /tmp/x.md " + knowledge + "/ 2>&1",
	} {
		out, _ := runHookScript(t, "vault-reach.sh", env, bashPayload(cmd))
		assert.Containsf(t, out, "--vault "+knowledge, "a write into the vault: %s", cmd)
	}
}
