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

// A path that contains a .vaultmind/ marker but no valid config/index must be
// skipped (not abort the run) — the other discovered vaults still report.
func TestDoctorAll_SkipsUnopenableVault(t *testing.T) {
	root := t.TempDir()
	good := buildIndexedVaultAt(t, filepath.Join(root, "good-vault"))
	// A bogus "vault": has the marker dir but a malformed config.yaml, so
	// OpenVaultDB fails to load its config. It must be skipped, not crash the run.
	bogusCfg := filepath.Join(root, "zbogus-vault", ".vaultmind")
	require.NoError(t, os.MkdirAll(bogusCfg, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bogusCfg, "config.yaml"),
		[]byte("types: [this is not: valid: yaml"), 0o644))

	out, _, err := runRootCmd(t, "doctor", "--all", "--root", root, "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			Rollup struct {
				VaultCount int `json:"vault_count"`
			} `json:"rollup"`
			Vaults []struct {
				VaultPath string `json:"vault_path"`
			} `json:"vaults"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.Len(t, env.Result.Vaults, 1, "only the openable vault is diagnosed")
	assert.Equal(t, good, env.Result.Vaults[0].VaultPath)
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
