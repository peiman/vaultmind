package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildHealableVault creates a tempdir vault containing a note with a bare
// [[Alpha Concept]] wikilink whose title resolves to a differently-named file
// (alpha.md). FixWikilinks rewrites that to [[alpha|Alpha Concept]], so this
// fixture exercises the actual repair path (unlike buildIndexedTestVault, whose
// links are already in filename form or carry a display alias). Returns the
// vault root and the relative path of the note that holds the fixable link.
func buildHealableVault(t *testing.T) (vaultDir, linkNote string) {
	t.Helper()
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
    optional: [tags, related_ids]
`), 0o644))

	writeTestNote(t, dir, "concepts/alpha.md", `---
id: concept-alpha
type: concept
title: Alpha Concept
---
Alpha body, the link target.
`)
	// The fixable link: bare [[Alpha Concept]] (title), file is alpha.md →
	// FixWikilinks rewrites to [[alpha|Alpha Concept]].
	writeTestNote(t, dir, "concepts/gamma.md", `---
id: concept-gamma
type: concept
title: Gamma Concept
---
Gamma references [[Alpha Concept]] by title.
`)

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	idxr := index.NewIndexer(dir, dbPath, cfg)
	_, err = idxr.Rebuild()
	require.NoError(t, err, "indexer rebuild failed")

	return dir, "concepts/gamma.md"
}

// doctor heal applies ALL auto-fixable repairs doctor found. Today that set is
// exactly the wikilink fixer, so `doctor heal` rewrites the title-form wikilink
// on disk by default (no flag needed). This is the key semantic flip from the
// old `lint fix-links`, which was dry-run by default.
func TestDoctorHeal_AppliesByDefault(t *testing.T) {
	vault, note := buildHealableVault(t)
	notePath := filepath.Join(vault, note)

	before, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Contains(t, string(before), "[[Alpha Concept]]",
		"precondition: note must contain the title-form (fixable) wikilink")

	out, _, err := runRootCmd(t, "doctor", "heal", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			FilesChanged int `json:"files_changed"`
			LinksFixed   int `json:"links_fixed"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Greater(t, env.Result.LinksFixed, 0, "heal applies by default, so links must be fixed")

	after, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Contains(t, string(after), "[[alpha|Alpha Concept]]",
		"heal must rewrite the title-form link to filename|title form on disk by default")
}

// doctor heal --dry-run previews without touching disk. This is the inverse of
// the apply-by-default behavior and what the old fix-links did without --fix.
func TestDoctorHeal_DryRunPreviewsNoDiskChange(t *testing.T) {
	vault, note := buildHealableVault(t)
	notePath := filepath.Join(vault, note)

	before, err := os.ReadFile(notePath)
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "doctor", "heal", "--vault", vault, "--dry-run", "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			LinksFixed int `json:"links_fixed"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Greater(t, env.Result.LinksFixed, 0,
		"--dry-run still reports the planned fix")

	after, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after),
		"--dry-run must NOT change the file on disk even though it reports the planned fix")
}

// doctor heal human output reports the mode so operators see whether the action
// was applied. Default (no flag) is "apply".
func TestDoctorHeal_HumanOutputShowsApplyMode(t *testing.T) {
	vault, _ := buildHealableVault(t)
	out, _, err := runRootCmd(t, "doctor", "heal", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Mode: apply",
		"heal applies by default; the mode line must say apply")
}

// doctor heal --dry-run flips the mode label to dry-run.
func TestDoctorHeal_DryRunModeLabel(t *testing.T) {
	vault, _ := buildHealableVault(t)
	out, _, err := runRootCmd(t, "doctor", "heal", "--vault", vault, "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Mode: dry-run")
}

// After a successful apply that rewrote files, heal must warn that the index
// is now stale so the operator knows to re-index (M2). Human output names the
// remedy; this asserts the human path.
func TestDoctorHeal_WarnsIndexStaleAfterApply(t *testing.T) {
	vault, _ := buildHealableVault(t)
	out, _, err := runRootCmd(t, "doctor", "heal", "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "Index is now stale",
		"applying a heal that rewrote files must warn the index is stale")
	assert.Contains(t, text, "vaultmind index --vault",
		"the stale-index warning must name the re-index remedy")
}

// The stale-index warning also appears in the JSON envelope after an apply that
// changed files, via a dedicated field with a remedy string (M2).
func TestDoctorHeal_JSONReportsStaleIndexAfterApply(t *testing.T) {
	vault, _ := buildHealableVault(t)
	out, _, err := runRootCmd(t, "doctor", "heal", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			FilesChanged        int    `json:"files_changed"`
			StaleIndexAfterHeal bool   `json:"stale_index_after_heal"`
			StaleIndexRemedy    string `json:"stale_index_remedy"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.Greater(t, env.Result.FilesChanged, 0, "precondition: the apply rewrote files")
	assert.True(t, env.Result.StaleIndexAfterHeal,
		"JSON must flag the stale index after an apply that changed files")
	assert.Contains(t, env.Result.StaleIndexRemedy, "vaultmind index --vault",
		"JSON remedy must name the re-index command")
}

// --dry-run must NOT warn about a stale index: nothing was written, so the
// index is not stale (M2).
func TestDoctorHeal_DryRunDoesNotWarnStaleIndex(t *testing.T) {
	vault, _ := buildHealableVault(t)
	out, _, err := runRootCmd(t, "doctor", "heal", "--vault", vault, "--dry-run")
	require.NoError(t, err)
	assert.NotContains(t, out.String(), "Index is now stale",
		"--dry-run changed nothing on disk, so it must not warn about a stale index")
}

// A heal that changed zero files (already-clean vault) must NOT warn about a
// stale index (M2).
func TestDoctorHeal_NoWarnWhenZeroChanges(t *testing.T) {
	// buildIndexedTestVault's links are already filename/alias form, so heal
	// finds nothing to fix → zero files changed.
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "doctor", "heal", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			FilesChanged        int  `json:"files_changed"`
			StaleIndexAfterHeal bool `json:"stale_index_after_heal"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.Equal(t, 0, env.Result.FilesChanged, "precondition: nothing to fix")
	assert.False(t, env.Result.StaleIndexAfterHeal,
		"zero files changed must not flag a stale index")
	assert.NotContains(t, out.String(), "Index is now stale")
}

// doctor heal wikilinks is the surgical wikilink repair (the moved
// `lint fix-links` logic). It applies by default and shares the same fixer
// engine, so it also rewrites the title-form link on disk.
func TestDoctorHealWikilinks_AppliesByDefault(t *testing.T) {
	vault, note := buildHealableVault(t)
	notePath := filepath.Join(vault, note)

	out, _, err := runRootCmd(t, "doctor", "heal", "wikilinks", "--vault", vault, "--json")
	require.NoError(t, err)

	var env struct {
		Result struct {
			LinksFixed int `json:"links_fixed"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Greater(t, env.Result.LinksFixed, 0)

	after, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Contains(t, string(after), "[[alpha|Alpha Concept]]")
}

// doctor heal wikilinks --dry-run previews only.
func TestDoctorHealWikilinks_DryRunPreviews(t *testing.T) {
	vault, note := buildHealableVault(t)
	notePath := filepath.Join(vault, note)

	before, err := os.ReadFile(notePath)
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "doctor", "heal", "wikilinks", "--vault", vault, "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Mode: dry-run")

	after, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

// doctor fix is the cobra alias for doctor heal: it must apply by default too.
func TestDoctorFix_AliasOfHeal(t *testing.T) {
	vault, note := buildHealableVault(t)
	notePath := filepath.Join(vault, note)

	_, _, err := runRootCmd(t, "doctor", "fix", "--vault", vault)
	require.NoError(t, err)

	after, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Contains(t, string(after), "[[alpha|Alpha Concept]]",
		"doctor fix is an alias of doctor heal and must apply by default")
}

// doctor fix wikilinks must also resolve via the alias and apply.
func TestDoctorFixWikilinks_AliasResolves(t *testing.T) {
	vault, _ := buildHealableVault(t)
	out, _, err := runRootCmd(t, "doctor", "fix", "wikilinks", "--vault", vault, "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			LinksFixed int `json:"links_fixed"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Greater(t, env.Result.LinksFixed, 0)
}

// The doctor heal help surface must read "heal (fix)" so the alias is
// discoverable from the help text, per the locked taxonomy.
func TestDoctorHeal_HelpReadsHealFix(t *testing.T) {
	out, _, err := runRootCmd(t, "doctor", "heal", "--help")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "heal (fix)",
		"help should read 'heal (fix)' so the alias is visible")
}

// ---- Top-level lint removal + deprecation aliases. ----

// The top-level `lint` parent must no longer appear in the visible root
// listing: linting/fixing moved under `doctor heal`. It survives as a hidden
// parent so the deprecated `lint fix-links` alias still resolves.
func TestTopLevelLint_HiddenFromRootListing(t *testing.T) {
	for _, c := range RootCmd.Commands() {
		if c.Name() == "lint" {
			assert.True(t, c.Hidden, "top-level 'lint' must be hidden from the root listing")
			return
		}
	}
}

// The agent root-help cheat-sheet must no longer advertise `lint` as an
// infrastructure command — it was removed from the top level.
func TestRootHelp_DropsLintFromInfrastructureList(t *testing.T) {
	out, _, err := runRootCmd(t, "--help")
	require.NoError(t, err)
	assert.NotContains(t, out.String(), "lint,",
		"root help must not list the removed top-level lint command")
}

// lint fix-links is a hidden deprecated alias of `doctor heal wikilinks`: it
// prints a one-line stderr notice and still delegates to the shared fixer.
// Because the OLD command was dry-run-by-default (--fix to apply), the alias
// preserves THAT contract: no --fix means preview, so disk is untouched.
func TestDeprecated_LintFixLinks_WarnsAndPreviewsByDefault(t *testing.T) {
	vault, note := buildHealableVault(t)
	notePath := filepath.Join(vault, note)
	before, err := os.ReadFile(notePath)
	require.NoError(t, err)

	out, errOut, err := runRootCmd(t, "lint", "fix-links", "--vault", vault, "--json")
	require.NoError(t, err)
	assertOneLineDeprecation(t, errOut.String(), "doctor heal wikilinks")

	// Delegated output is still a real fix-links envelope.
	var env struct {
		Result struct {
			FilesScanned int `json:"files_scanned"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Greater(t, env.Result.FilesScanned, 0)

	// Old contract: no --fix means dry-run, so disk is untouched.
	after, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after),
		"deprecated lint fix-links keeps its dry-run-by-default contract")
}

// lint fix-links --fix still applies (old contract preserved), delegating to
// the shared fixer engine and emitting the deprecation notice.
func TestDeprecated_LintFixLinks_FixApplies(t *testing.T) {
	vault, note := buildHealableVault(t)
	notePath := filepath.Join(vault, note)

	_, errOut, err := runRootCmd(t, "lint", "fix-links", "--vault", vault, "--fix")
	require.NoError(t, err)
	assertOneLineDeprecation(t, errOut.String(), "doctor heal wikilinks")

	after, err := os.ReadFile(notePath)
	require.NoError(t, err)
	assert.Contains(t, string(after), "[[alpha|Alpha Concept]]",
		"lint fix-links --fix must still apply via the shared engine")
}

// The deprecation notice for lint fix-links must be exactly one line.
func TestDeprecated_LintFixLinks_NoticeIsSingleLine(t *testing.T) {
	vault, _ := buildHealableVault(t)
	_, errOut, err := runRootCmd(t, "lint", "fix-links", "--vault", vault)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(errOut.String(), "\n"), "\n")
	require.Len(t, lines, 1, "deprecation notice must be exactly one line: %q", errOut.String())
	assert.Contains(t, lines[0], "deprecated")
}

// dataview lint is a SEPARATE domain checker and must remain untouched by the
// top-level lint removal — it still runs and reports its files-checked count.
func TestDataviewLint_SurvivesLintRemoval(t *testing.T) {
	vault := buildIndexedTestVault(t)
	out, _, err := runRootCmd(t, "dataview", "lint", "--vault", vault, "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			FilesChecked int `json:"files_checked"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Greater(t, env.Result.FilesChecked, 0)
}
