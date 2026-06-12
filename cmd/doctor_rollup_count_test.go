package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/require"
)

// issuesLineRE captures the two counts the TEXT rollup prints:
// "Issues: <errors> errors, <warnings> warnings".
var issuesLineRE = regexp.MustCompile(`Issues: (\d+) errors, (\d+) warnings`)

// jsonSurfacedIssues mirrors the result.issues block of the doctor --json
// envelope — the surfaced-issue set that is the source of truth for the text
// rollup. Only the count fields that SurfacedIssueCounts reads are decoded.
type jsonSurfacedIssues struct {
	Result struct {
		Issues struct {
			DuplicateIDs              int  `json:"duplicate_ids"`
			BrokenReferences          int  `json:"broken_references"`
			MissingRequiredFields     int  `json:"missing_required_fields"`
			MalformedMarkers          int  `json:"malformed_markers"`
			UnresolvedLinks           int  `json:"unresolved_links"`
			NotesMissingIDOrType      int  `json:"notes_missing_id_or_type"`
			ObsidianIncompatibleLinks int  `json:"obsidian_incompatible_links"`
			PathPseudoIDLinks         int  `json:"path_pseudo_id_links"`
			StaleIndex                int  `json:"stale_index"`
			HookDrift                 int  `json:"hook_drift"`
			LegacyHooksJSON           bool `json:"legacy_hooks_json"`
		} `json:"issues"`
		// ValidationSummary is the explicitly-labeled raw aggregate axis (renamed
		// from the ambiguous "issues_summary" to prevent confusion with the surfaced
		// result.issues set). Decoded here to assert it carries the raw count.
		ValidationSummary struct {
			Warnings int `json:"warnings"`
		} `json:"validation_summary"`
	} `json:"result"`
}

// surfacedErrWarn classifies the --json result.issues block the same way the
// SSOT helper does, so the test asserts against an INDEPENDENTLY computed count
// rather than re-using the production helper's arithmetic.
func (j jsonSurfacedIssues) surfacedErrWarn() (errs, warns int) {
	i := j.Result.Issues
	errs = i.DuplicateIDs + i.MissingRequiredFields + i.MalformedMarkers +
		i.NotesMissingIDOrType + i.PathPseudoIDLinks
	warns = i.UnresolvedLinks + i.BrokenReferences + i.ObsidianIncompatibleLinks +
		i.StaleIndex + i.HookDrift
	if i.LegacyHooksJSON {
		warns++
	}
	return errs, warns
}

// chdirToTemp moves the test process into a fresh tempdir with no .claude/
// ancestor, so doctor's project-root hook-drift detection cannot fire and the
// surfaced-issue set is determined solely by vault content. Restores CWD on
// cleanup. Not parallel-safe — these tests must not call t.Parallel().
func chdirToTemp(t *testing.T) {
	t.Helper()
	old, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() { _ = os.Chdir(old) })
}

// buildValidationWarningVault creates a vault whose ONLY health findings are
// schema-validation warnings (unknown_type) — findings the doctor TEXT renderer
// does NOT surface as per-item lines. This is the exact shape that produced the
// reported divergence: TEXT "Issues: 0 errors, N warnings" while --json's
// surfaced result.issues block is all zero (the validation warnings live only
// in the nested result.validation_summary aggregate).
func buildValidationWarningVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))
	for i, name := range []string{"a", "b", "c"} {
		writeTestNote(t, dir, fmt.Sprintf("%s.md", name), fmt.Sprintf(
			"---\nid: note-%s\ntype: mystery-%d\ntitle: Note %s\n---\nbody\n", name, i, name))
	}

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	return dir
}

func readTextIssueCounts(t *testing.T, vault string) (errs, warns int, raw string) {
	t.Helper()
	textOut, _, err := runRootCmd(t, "doctor", "--vault", vault)
	require.NoError(t, err)
	raw = textOut.String()
	m := issuesLineRE.FindStringSubmatch(raw)
	require.NotNil(t, m, "TEXT rollup must print an Issues: line; got:\n%s", raw)
	errs, _ = strconv.Atoi(m[1])
	warns, _ = strconv.Atoi(m[2])
	return errs, warns, raw
}

func readJSONSurfaced(t *testing.T, vault string) jsonSurfacedIssues {
	t.Helper()
	jsonOut, _, err := runRootCmd(t, "doctor", "--vault", vault, "--json")
	require.NoError(t, err)
	var j jsonSurfacedIssues
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &j))
	return j
}

// TestDoctor_TextRollupCounts_MatchJSONSurfacedSet asserts the TEXT rollup's
// "Issues:" counts equal the --json source of truth (the surfaced result.issues
// block), for a vault whose only findings are schema-validation warnings the
// TEXT renderer never surfaces as lines.
//
// Before the fix the TEXT rollup read result.validation_summary (then named
// result.issues_summary — the validation AGGREGATE: 3 unknown_type warnings)
// while the surfaced result.issues block was all zero, so the TEXT count
// OVERSTATED warnings no text line backed — and the aggregate stayed visible
// in --json's result.validation_summary, proving the two counts came from
// different sets.
func TestDoctor_TextRollupCounts_MatchJSONSurfacedSet(t *testing.T) {
	chdirToTemp(t)
	isolateMeshEnv(t)
	vault := buildValidationWarningVault(t)

	textErrors, textWarnings, _ := readTextIssueCounts(t, vault)
	j := readJSONSurfaced(t, vault)
	jsonErrors, jsonWarnings := j.surfacedErrWarn()

	require.Equal(t, jsonErrors, textErrors,
		"TEXT error count must equal --json surfaced result.issues errors")
	require.Equal(t, jsonWarnings, textWarnings,
		"TEXT warning count must equal --json surfaced result.issues warnings")

	// Document the divergence the fix closes: the raw validation aggregate is
	// non-zero in --json under its explicitly-named key, yet the surfaced
	// counts (and now the text) are zero.
	require.Equal(t, 3, j.Result.ValidationSummary.Warnings,
		"raw validation aggregate still reports 3 warnings in --json result.validation_summary")
	require.Equal(t, 0, textWarnings,
		"text must report the surfaced count (0), not the validation aggregate (3)")
	require.Equal(t, 0, textErrors, "no surfaced errors")
}

// TestDoctor_TextRollupCounts_ReflectSurfacedItems proves the rollup tracks
// REAL surfaced issues: a stale-index drift is surfaced both as a text line and
// counted as a warning, and the text count equals the --json surfaced count.
// This guards against a fix that simply hard-codes 0/0.
func TestDoctor_TextRollupCounts_ReflectSurfacedItems(t *testing.T) {
	chdirToTemp(t)
	isolateMeshEnv(t)
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
`), 0o644))
	notePath := filepath.Join(dir, "a.md")
	writeTestNote(t, dir, "a.md", "---\nid: note-a\ntype: concept\ntitle: A\n---\noriginal body\n")

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)

	// Edit the note AFTER indexing so its content hash drifts from the stored
	// hash => a surfaced stale-index warning the text report renders as a line.
	require.NoError(t, os.WriteFile(notePath,
		[]byte("---\nid: note-a\ntype: concept\ntitle: A\n---\nEDITED body, hash now differs\n"), 0o644))

	textErrors, textWarnings, raw := readTextIssueCounts(t, dir)
	j := readJSONSurfaced(t, dir)
	jsonErrors, jsonWarnings := j.surfacedErrWarn()

	require.Equal(t, jsonErrors, textErrors, "TEXT errors must equal --json surfaced errors")
	require.Equal(t, jsonWarnings, textWarnings, "TEXT warnings must equal --json surfaced warnings")
	require.Greater(t, jsonErrors+jsonWarnings, 0,
		"this vault must surface at least one issue so the test is non-vacuous")
	require.Contains(t, raw, "Stale index:",
		"the surfaced stale-index drift must also appear as a text detail line")
}
