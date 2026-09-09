package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildNamedTestVault creates an indexed vault at a KNOWN directory name,
// because the directory name is the vault's display name in federated output
// and a t.TempDir() random name cannot be asserted on.
func buildNamedTestVault(t *testing.T, name string, notes map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
    optional: [tags, related_ids]
`), 0o644))
	for rel, content := range notes {
		writeTestNote(t, dir, rel, content)
	}
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	return dir
}

// --read N under federation must read the Nth hit of the ranking the agent was
// SHOWN, including when that hit lives in another vault.
//
// It did not. runAskRead re-searched the owning vault and indexed into THAT
// list, so the cross-vault menu said rank 2 was one note and --read 2
// delivered a different one, silently, from a different vault. Measured live
// against the real vaults: the menu showed `arc-the-half-i-thought-to-check`
// [identity] at 2 and --read 2 returned `journal-2026-06-04-the-re-sealing`
// [mine]. An agent that asks for what it just saw must not get something else.
func TestAskRead_UnderFederationReadsTheRankTheMenuShowed(t *testing.T) {
	alpha := buildNamedTestVault(t, "alpha-vault", map[string]string{
		"a1.md": "---\nid: a-one\ntype: concept\ntitle: quokka alpha\n---\nquokka quokka quokka strongest in alpha.\n",
		"a2.md": "---\nid: a-two\ntype: concept\ntitle: alpha runner up\n---\nquokka mentioned once. SENTINEL-ALPHA-TWO\n",
	})
	beta := buildNamedTestVault(t, "beta-vault", map[string]string{
		"b1.md": "---\nid: b-one\ntype: concept\ntitle: quokka beta\n---\nquokka quokka quokka SENTINEL-BETA-ONE\n",
	})

	menu, _, err := runRootCmd(t, "ask", "quokka", "--vaults", alpha+","+beta)
	require.NoError(t, err)
	require.Contains(t, menu.String(), "2. [beta-vault] b-one",
		"precondition: the cross-vault menu must rank the OTHER vault's note at 2")

	out, _, err := runRootCmd(t, "ask", "quokka", "--vaults", alpha+","+beta, "--read", "2")
	require.NoError(t, err)

	assert.Contains(t, out.String(), "SENTINEL-BETA-ONE",
		"--read 2 must deliver the note the menu showed at rank 2, from the vault that owns it")
	assert.NotContains(t, out.String(), "SENTINEL-ALPHA-TWO",
		"--read must not silently fall back to the owning vault's own rank 2")
}

// The federation block must survive --read. Without it the agent sees a
// single-vault view and cannot tell which vault the body came from, nor that
// two other vaults were searched and had nothing.
func TestAskRead_UnderFederationStillReportsWhatWasSearched(t *testing.T) {
	alpha := buildNamedTestVault(t, "alpha-vault", map[string]string{
		"a1.md": "---\nid: a-one\ntype: concept\ntitle: quokka alpha\n---\nquokka quokka quokka.\n",
	})
	beta := buildNamedTestVault(t, "beta-vault", map[string]string{
		"b1.md": "---\nid: b-one\ntype: concept\ntitle: unrelated wombat\n---\nwombat only.\n",
	})

	out, _, err := runRootCmd(t, "ask", "quokka", "--vaults", alpha+","+beta, "--read", "1")
	require.NoError(t, err)

	assert.Contains(t, out.String(), "federated:",
		"--read must still say how many vaults were searched and which one delivered")
}

// THE ORIGINAL TRAP, which shipping --vaults did not close.
//
// `--vault A --vault B` is a plain string flag: pflag overwrites, so A
// vanishes and the command answers confidently from B alone. That is the
// defect that motivated federated search, and after building the escape
// hatch the hole was still open — the next person to try the obvious thing
// would get the same silent wrong answer. Building the alternative is not
// the same as closing the trap.
func TestAsk_RepeatedVaultFlagIsAnErrorPointingAtVaults(t *testing.T) {
	alpha := buildNamedTestVault(t, "alpha-vault", map[string]string{
		"a1.md": "---\nid: a-one\ntype: concept\ntitle: quokka alpha\n---\nquokka.\n",
	})
	beta := buildNamedTestVault(t, "beta-vault", map[string]string{
		"b1.md": "---\nid: b-one\ntype: concept\ntitle: quokka beta\n---\nquokka.\n",
	})

	_, _, err := runRootCmd(t, "ask", "quokka", "--vault", alpha, "--vault", beta)

	require.Error(t, err, "passing --vault twice must not silently use the last one")
	assert.Contains(t, err.Error(), "--vaults", "the error must name the flag that does what they meant")
	assert.Contains(t, err.Error(), alpha, "it must name the path that would have been dropped")
	assert.Contains(t, err.Error(), beta)
}

// One --vault is the overwhelmingly common case and must be untouched.
func TestAsk_SingleVaultFlagIsUnaffectedByTheRepeatCheck(t *testing.T) {
	alpha := buildNamedTestVault(t, "alpha-vault", map[string]string{
		"a1.md": "---\nid: a-one\ntype: concept\ntitle: quokka alpha\n---\nquokka.\n",
	})

	out, _, err := runRootCmd(t, "ask", "quokka", "--vault", alpha)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "a-one")
}
