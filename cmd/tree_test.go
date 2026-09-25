package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/testvault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `vaultmind tree` is the map of a vault: every folder with its note count,
// every note as "Title (id) — what it is about". An agent could only search
// before, which needs a question already formed.

func TestTree_ListsFoldersAndNotesWithTheirIDs(t *testing.T) {
	vault := testvault.IndexedFixtureVault(t)

	out, _, err := runRootCmd(t, "tree", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "concepts/ (")
	assert.Contains(t, out.String(), "ACT-R (concept-act-r)", "the id is there so the agent can open the note")
	assert.Contains(t, out.String(), "notes", "the header says how much the vault holds")
}

func TestTree_DepthOneIsAnOverview(t *testing.T) {
	vault := testvault.IndexedFixtureVault(t)

	out, _, err := runRootCmd(t, "tree", "--vault", vault, "--depth", "1")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "concepts/ (")
	assert.NotContains(t, out.String(), "ACT-R", "depth 1 shows folders and counts, not every note")
}

func TestTree_FiltersByTypeAndPath(t *testing.T) {
	vault := testvault.IndexedFixtureVault(t)

	out, _, err := runRootCmd(t, "tree", "--vault", vault, "--type", "decision")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "decisions/")
	assert.NotContains(t, out.String(), "ACT-R")

	out, _, err = runRootCmd(t, "tree", "--vault", vault, "--path", "concepts/")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "ACT-R")
	assert.NotContains(t, out.String(), "decisions/")
}

func TestTree_SaysSoWhenNothingMatches(t *testing.T) {
	vault := testvault.IndexedFixtureVault(t)

	out, _, err := runRootCmd(t, "tree", "--vault", vault, "--type", "no-such-type")
	require.NoError(t, err)
	assert.Contains(t, out.String(), "No notes", "an empty map says why instead of printing nothing")
}

func TestTree_JSONCarriesTheWholeMap(t *testing.T) {
	vault := testvault.IndexedFixtureVault(t)

	out, _, err := runRootCmd(t, "tree", "--vault", vault, "--json")
	require.NoError(t, err)
	var env struct {
		Status string `json:"status"`
		Result struct {
			Vaults []struct {
				Vault string `json:"vault"`
				Total int    `json:"total"`
				Root  struct {
					Dirs []struct {
						Path  string `json:"path"`
						Notes []struct {
							ID   string `json:"id"`
							Line string `json:"line"`
						} `json:"notes"`
					} `json:"dirs"`
				} `json:"root"`
			} `json:"vaults"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	assert.Equal(t, "ok", env.Status)
	require.Len(t, env.Result.Vaults, 1)
	assert.Positive(t, env.Result.Vaults[0].Total)
	require.NotEmpty(t, env.Result.Vaults[0].Root.Dirs)
	require.NotEmpty(t, env.Result.Vaults[0].Root.Dirs[0].Notes)
	assert.NotEmpty(t, env.Result.Vaults[0].Root.Dirs[0].Notes[0].Line)
}

func TestTree_VaultsMapsEachVault(t *testing.T) {
	a := testvault.IndexedFixtureVault(t)
	b := testvault.IndexedFixtureVault(t)

	out, _, err := runRootCmd(t, "tree", "--vaults", a+","+b, "--depth", "1")
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(out.String(), " notes"), "one header per vault")
	assert.Contains(t, out.String(), a)
	assert.Contains(t, out.String(), b)
}

// `tree --for <file>` is the map of the notes about one file: the ones whose
// `paths:` cover it.
func TestTree_ForAFileListsTheNotesThatCoverIt(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "code-repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "internal"), 0o750))
	file := filepath.Join(repo, "internal", "a.go")
	require.NoError(t, os.WriteFile(file, []byte("package internal\n"), 0o600))

	vault := testvault.IndexedFixtureVault(t)
	note := "---\nid: concept-about-a\ntype: concept\ntitle: About a.go\npaths:\n  - internal/*.go\n---\n\nWhy a.go is built this way. More.\n"
	require.NoError(t, os.WriteFile(filepath.Join(vault, "concepts", "about-a.md"), []byte(note), 0o600))
	_, _, err := runRootCmd(t, "index", "--vault", vault)
	require.NoError(t, err)

	out, _, err := runRootCmd(t, "tree", "--vault", vault, "--for", file)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "About a.go (concept-about-a) — Why a.go is built this way.")
	assert.NotContains(t, out.String(), "ACT-R", "only the notes that cover the file")
	assert.Contains(t, out.String(), "code-repo:internal/a.go", "the header names the file as paths: sees it")

	out, _, err = runRootCmd(t, "tree", "--vault", vault, "--for", filepath.Join(repo, "README.md"))
	require.NoError(t, err)
	assert.Contains(t, out.String(), "No notes cover")
}
