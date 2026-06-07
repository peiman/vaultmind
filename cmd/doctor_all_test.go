package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildIndexedVaultAt creates a minimal indexed vault rooted at dir (which must
// already exist) and returns dir. Mirrors buildIndexedTestVault but targets a
// caller-chosen path so several vaults can share one discovery root.
func buildIndexedVaultAt(t *testing.T, dir string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
    optional: [tags]
`), 0o644))
	writeTestNote(t, dir, "concepts/alpha.md", `---
id: concept-alpha
type: concept
title: Alpha Concept
---
Alpha body.
`)
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	return dir
}

// workspaceWithVaults builds a root containing two indexed vaults and returns
// the root plus the two vault paths.
func workspaceWithVaults(t *testing.T) (root, vaultA, vaultB string) {
	t.Helper()
	root = t.TempDir()
	vaultA = buildIndexedVaultAt(t, filepath.Join(root, "alpha-vault"))
	vaultB = buildIndexedVaultAt(t, filepath.Join(root, "beta-vault"))
	return root, vaultA, vaultB
}

func TestDoctorAll_HumanRollupAndPerVaultHeaders(t *testing.T) {
	root, vaultA, vaultB := workspaceWithVaults(t)
	out, _, err := runRootCmd(t, "doctor", "--all", "--root", root)
	require.NoError(t, err)
	text := out.String()

	// Combined rollup at the top.
	assert.Contains(t, text, "Discovered 2 vault(s)", "rollup names the vault count")
	assert.Contains(t, text, "Total notes:", "rollup reports total notes")
	// Per-vault headers for each discovered vault.
	assert.Contains(t, text, vaultA, "per-vault section for vault A")
	assert.Contains(t, text, vaultB, "per-vault section for vault B")
	// Each vault still renders its own doctor body.
	assert.Contains(t, text, "Notes: ", "per-vault doctor body renders")
}

func TestDoctorAll_SingleCombinedJSONEnvelope(t *testing.T) {
	root, vaultA, vaultB := workspaceWithVaults(t)
	out, _, err := runRootCmd(t, "doctor", "--all", "--root", root, "--json")
	require.NoError(t, err)

	// MUST be exactly one JSON value on stdout — not one envelope per vault.
	trimmed := strings.TrimSpace(out.String())
	dec := json.NewDecoder(strings.NewReader(trimmed))
	var env struct {
		Command string `json:"command"`
		Status  string `json:"status"`
		Result  struct {
			Rollup struct {
				VaultCount int `json:"vault_count"`
				TotalNotes int `json:"total_notes"`
			} `json:"rollup"`
			Vaults []struct {
				VaultPath string `json:"vault_path"`
			} `json:"vaults"`
		} `json:"result"`
	}
	require.NoError(t, dec.Decode(&env), "stdout must be a single decodable envelope")
	// Decoding a second value must fail with EOF — proving there is only one.
	var extra json.RawMessage
	assert.ErrorIs(t, dec.Decode(&extra), io.EOF,
		"exactly one envelope on stdout (the double-envelope bug must not return)")

	assert.Equal(t, "doctor", env.Command)
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, 2, env.Result.Rollup.VaultCount)
	assert.Equal(t, 2, env.Result.Rollup.TotalNotes, "two vaults, one note each")
	require.Len(t, env.Result.Vaults, 2)
	paths := []string{env.Result.Vaults[0].VaultPath, env.Result.Vaults[1].VaultPath}
	assert.Contains(t, paths, vaultA)
	assert.Contains(t, paths, vaultB)
}

func TestDoctorAll_ComposesWithSummary(t *testing.T) {
	root, _, _ := workspaceWithVaults(t)
	out, _, err := runRootCmd(t, "doctor", "--all", "--summary", "--root", root)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Discovered 2 vault(s)", "rollup still renders under --summary")
	assert.Contains(t, text, "Issues:", "each vault's errors/warnings rollup renders")
	// --summary suppresses verbose per-link detail lines.
	assert.NotContains(t, text, "→ [[", "--all --summary suppresses per-link detail")
}

func TestDoctorAll_DeterministicVaultOrder(t *testing.T) {
	root, vaultA, vaultB := workspaceWithVaults(t)
	out, _, err := runRootCmd(t, "doctor", "--all", "--root", root, "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			Vaults []struct {
				VaultPath string `json:"vault_path"`
			} `json:"vaults"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.Len(t, env.Result.Vaults, 2)
	// alpha-vault sorts before beta-vault.
	assert.Equal(t, vaultA, env.Result.Vaults[0].VaultPath, "vaults emitted in sorted order")
	assert.Equal(t, vaultB, env.Result.Vaults[1].VaultPath)
}

// buildVaultWithValidationError builds an indexed vault containing a note that
// declares its type but omits a required field, producing a schema-validation
// error. Used to exercise the rollup's "vaults with issues" reporting.
func buildVaultWithValidationError(t *testing.T, dir string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
    optional: [tags]
`), 0o644))
	// Missing the required `title` field -> a missing_required_field error.
	writeTestNote(t, dir, "concepts/broken.md", `---
id: concept-broken
type: concept
---
Body without a title.
`)
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	return dir
}

// A vault with validation errors must appear in the rollup's "vaults with
// issues" list — in both the human header and the JSON rollup.
func TestDoctorAll_RollupListsVaultsWithIssues(t *testing.T) {
	root := t.TempDir()
	clean := buildIndexedVaultAt(t, filepath.Join(root, "clean-vault"))
	broken := buildVaultWithValidationError(t, filepath.Join(root, "zbroken-vault"))

	// Human output names the broken vault under a "Vaults with issues" heading.
	out, _, err := runRootCmd(t, "doctor", "--all", "--root", root)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Vaults with issues:", "rollup heads the problem-vault list")
	assert.Contains(t, text, broken, "the broken vault is named in the issues list")
	assert.Contains(t, text, "Total issues: 1 errors", "rollup sums the validation error")

	// JSON rollup carries the same vaults_with_issues list and combined counts.
	jsonOut, _, err := runRootCmd(t, "doctor", "--all", "--root", root, "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			Rollup struct {
				TotalErrors      int      `json:"total_errors"`
				VaultsWithIssues []string `json:"vaults_with_issues"`
			} `json:"rollup"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &env))
	assert.Equal(t, 1, env.Result.Rollup.TotalErrors)
	assert.Equal(t, []string{broken}, env.Result.Rollup.VaultsWithIssues,
		"only the broken vault is listed; the clean one is not")
	assert.NotContains(t, env.Result.Rollup.VaultsWithIssues, clean)
}

// makeBogusVault creates a directory with a .vaultmind/ marker but a malformed
// config.yaml, so OpenVaultDB fails to load it. Returns the vault path. Such a
// vault must be SURFACED (named, with its reason), never silently dropped.
func makeBogusVault(t *testing.T, dir string) string {
	t.Helper()
	cfgDir := filepath.Join(dir, ".vaultmind")
	require.NoError(t, os.MkdirAll(cfgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.yaml"),
		[]byte("types: [this is not: valid: yaml"), 0o644))
	return dir
}

// A path that contains a .vaultmind/ marker but no valid config/index must be
// SURFACED, not silently swallowed: the operator must see the broken vault named
// (with its reason) in both human and JSON output, and the discovered count must
// include it. This guards the silent-failure regression vs single-vault doctor.
func TestDoctorAll_SurfacesUnopenableVault(t *testing.T) {
	root := t.TempDir()
	good := buildIndexedVaultAt(t, filepath.Join(root, "good-vault"))
	bogus := makeBogusVault(t, filepath.Join(root, "zbogus-vault"))

	// --- JSON: the broken vault appears under result.failed[], the good one
	//     under result.vaults[], and the rollup counts stay honest. ---
	out, errOut, err := runRootCmd(t, "doctor", "--all", "--root", root, "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			Rollup struct {
				VaultCount int `json:"vault_count"`
				Discovered int `json:"discovered"`
				Diagnosed  int `json:"diagnosed"`
				Failed     int `json:"failed"`
			} `json:"rollup"`
			Vaults []struct {
				VaultPath string `json:"vault_path"`
			} `json:"vaults"`
			Failed []struct {
				VaultPath string `json:"vault_path"`
				Error     string `json:"error"`
			} `json:"failed"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))

	require.Len(t, env.Result.Vaults, 1, "the openable vault is diagnosed")
	assert.Equal(t, good, env.Result.Vaults[0].VaultPath)

	require.Len(t, env.Result.Failed, 1, "the unopenable vault is surfaced, not dropped")
	assert.Equal(t, bogus, env.Result.Failed[0].VaultPath, "the broken vault is named")
	assert.NotEmpty(t, env.Result.Failed[0].Error, "the failure reason is carried")

	// Counts stay honest: discovered > diagnosed when one fails.
	assert.Equal(t, 2, env.Result.Rollup.Discovered, "discovered counts the broken vault too")
	assert.Equal(t, 1, env.Result.Rollup.Diagnosed)
	assert.Equal(t, 1, env.Result.Rollup.Failed)
	assert.Greater(t, env.Result.Rollup.Discovered, env.Result.Rollup.Diagnosed,
		"discovered exceeds diagnosed precisely because a vault failed")

	// A warning line about the broken vault must reach stderr — never invisible.
	assert.Contains(t, errOut.String(), bogus, "every failed vault warns on stderr")

	// --- Human: the broken vault is named under a clearly-visible failure
	//     section, and the header reports the honest breakdown. ---
	human, herrOut, err := runRootCmd(t, "doctor", "--all", "--root", root)
	require.NoError(t, err)
	text := human.String()
	assert.Contains(t, text, "Discovered 2 vault(s)", "header counts the broken vault")
	assert.Contains(t, text, "Vaults that failed to open: 1", "a visible failure section heads the list")
	assert.Contains(t, text, bogus, "the broken vault is named in the human failure section")
	assert.Contains(t, herrOut.String(), bogus, "every failed vault warns on stderr (human mode too)")
}

// The single combined JSON envelope contract still holds when a vault fails:
// exactly one envelope on stdout, even though a failed vault is reported.
func TestDoctorAll_FailedVaultKeepsSingleEnvelope(t *testing.T) {
	root := t.TempDir()
	buildIndexedVaultAt(t, filepath.Join(root, "good-vault"))
	makeBogusVault(t, filepath.Join(root, "zbogus-vault"))

	out, _, err := runRootCmd(t, "doctor", "--all", "--root", root, "--json")
	require.NoError(t, err)

	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(out.String())))
	var env json.RawMessage
	require.NoError(t, dec.Decode(&env), "stdout must be a single decodable envelope")
	var extra json.RawMessage
	assert.ErrorIs(t, dec.Decode(&extra), io.EOF,
		"exactly one envelope on stdout even with a failed vault (no per-vault error envelope)")
}

func TestDoctorAll_ZeroVaultsHumanMessage(t *testing.T) {
	root := t.TempDir() // empty: no vaults under it
	out, _, err := runRootCmd(t, "doctor", "--all", "--root", root)
	require.NoError(t, err, "zero vaults discovered is not an error exit")
	assert.Contains(t, out.String(), "No vaults found", "clearly states nothing was discovered")
}

func TestDoctorAll_ZeroVaultsJSON(t *testing.T) {
	root := t.TempDir()
	out, _, err := runRootCmd(t, "doctor", "--all", "--root", root, "--json")
	require.NoError(t, err)
	var env struct {
		Status string `json:"status"`
		Result struct {
			Rollup struct {
				VaultCount int `json:"vault_count"`
			} `json:"rollup"`
			Vaults []json.RawMessage `json:"vaults"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "ok", env.Status, "zero-vault is a successful (warning-free) envelope")
	assert.Equal(t, 0, env.Result.Rollup.VaultCount)
	assert.Empty(t, env.Result.Vaults, "no per-vault entries when none discovered")
}

func TestDoctorAll_MissingRootJSONError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	out, _, err := runRootCmd(t, "doctor", "--all", "--root", missing, "--json")
	require.NoError(t, err, "a discovery failure is reported in the envelope, not as a process error")
	var env struct {
		Status string `json:"status"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors)
}

func TestDoctorAll_MissingRootHumanError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	_, _, err := runRootCmd(t, "doctor", "--all", "--root", missing)
	require.Error(t, err, "a discovery failure surfaces as a non-JSON command error")
}
