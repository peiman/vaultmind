package cmd

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// Multi-vault was not merely unsupported — it was silently wrong. `--vault A
// --vault B` kept B and answered "nothing relevant" without ever opening A.
// These cover the parsing half: which vaults a run will actually search.

// isolateVaultsFlag stops a test's --vaults from leaking into the rest of the
// package. NewCommand binds every flag into the GLOBAL viper, so a value set
// here is still returned by viper.Get long after the test ends — which sent
// five unrelated `ask --read` tests down the federation path and made them
// fail only in a full package run, never alone.
func isolateVaultsFlag(t *testing.T) {
	t.Cleanup(func() { viper.Set(config.KeyAppAskVaults, "") })
}

func TestResolveAskVaultPaths_SingleVaultUnchanged(t *testing.T) {
	got, err := resolveAskVaultPaths("/a/identity", "")
	require.NoError(t, err)
	require.Equal(t, []string{"/a/identity"}, got, "the single-vault path must not change shape")
}

func TestResolveAskVaultPaths_CommaSeparatedFederates(t *testing.T) {
	got, err := resolveAskVaultPaths("/a/identity", "/a/identity,/a/desk,/a/research")
	require.NoError(t, err)
	require.Equal(t, []string{"/a/identity", "/a/desk", "/a/research"}, got)
}

func TestResolveAskVaultPaths_TrimsBlanksAndDeduplicates(t *testing.T) {
	got, err := resolveAskVaultPaths("/a/identity", " /a/desk , , /a/desk ,/a/research")
	require.NoError(t, err)
	require.Equal(t, []string{"/a/desk", "/a/research"}, got,
		"a repeated vault would double-count in RRF and silently outrank the others")
}

func TestResolveAskVaultPaths_OnlyBlanksIsAnError(t *testing.T) {
	_, err := resolveAskVaultPaths("/a/identity", " , ")
	require.Error(t, err, "asking to federate across nothing must not quietly fall back to one vault")
}

func TestVaultDisplayName_IsTheDirectoryName(t *testing.T) {
	require.Equal(t, "vaultmind-mine", vaultDisplayName("/Users/x/dev/vaultmind-mine"))
	require.Equal(t, "vaultmind-mine", vaultDisplayName("/Users/x/dev/vaultmind-mine/"))
}

// `--vaults X` with ONE path must search X — the same as `--vault X`.
//
// It did not. Federation only engaged at len>1, so a single-entry --vaults
// fell through to the unset --vault and died with `no vault found: "."`. Found
// while timing the federated path, not by any test: every existing case
// checked the PARSER, which was correct, while the CALLER threw the answer
// away. A parser test cannot see a caller that ignores it.
func TestAsk_SingleVaultsPathIsSearchedNotDiscarded(t *testing.T) {
	isolateVaultsFlag(t)
	vault := testvault.IndexedFixtureVault(t)
	cmd := MustNewCommand(commands.AskMetadata, runAsk)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Set("vaults", vault))

	err := runAsk(cmd, []string{"spreading activation"})

	require.NoError(t, err, "--vaults with one path must search that vault")
	require.NotContains(t, out.String(), "no vault found",
		"a single --vaults entry must not fall through to the unset --vault")
}

// A mistyped path in --vaults must be REPORTED, not silently turned into a
// vault.
//
// Found by review, reproduced live: `--vaults real,typo` created
// `typo/.vaultmind/index.db` on the way past and then reported the directory
// as "2 vaults searched ... unmeasured — no embedder, so kept in the merge".
// The single-vault rule deliberately lets a NAMED path be created, and that
// reasoning does not survive a list: with one vault an empty answer is
// obviously about the path you typed, while with three the other two answer
// and the typo disappears into a header that claims it was searched.
func TestAsk_FederationRejectsAPathThatIsNotAVault(t *testing.T) {
	isolateVaultsFlag(t)
	real := testvault.IndexedFixtureVault(t)
	typo := t.TempDir()
	cmd := MustNewCommand(commands.AskMetadata, runAsk)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Set("vaults", real+","+typo))

	err := runAsk(cmd, []string{"spreading activation"})

	require.Error(t, err, "federating across a non-vault path must fail loudly")
	require.Contains(t, err.Error(), typo, "the error must name the offending path")

	entries, readErr := os.ReadDir(typo)
	require.NoError(t, readErr)
	require.Empty(t, entries,
		"a mistyped vault path must not be promoted to a vault that every future walk-up finds")
}
