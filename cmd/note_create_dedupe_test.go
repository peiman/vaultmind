package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Before a note is added, the vault is asked whether a note by that name is
// already there: a duplicate splits what the vault knows about one thing in
// two, and the next reader finds half. The check warns and never blocks.

func TestNoteCreate_WarnsWhenANoteByThatNameExists(t *testing.T) {
	vault := buildIndexedTestVault(t)

	_, errOut, err := runRootCmd(t, "note", "create", "concepts/alpha-again.md",
		"--type", "concept", "--field", "title=Alpha Concept", "--vault", vault)
	require.NoError(t, err, "a warning, not a refusal")
	assert.Contains(t, errOut.String(), "concepts/alpha.md", "names the note that already exists")
	assert.Contains(t, errOut.String(), "extend it")
}

func TestNoteCreate_TheDuplicateWarningIsInTheJSON(t *testing.T) {
	vault := buildIndexedTestVault(t)

	out, _, err := runRootCmd(t, "note", "create", "concepts/alpha-again.md",
		"--type", "concept", "--field", "title=Alpha Concept", "--vault", vault, "--json")
	require.NoError(t, err)
	var env struct {
		Result struct {
			Warnings []string `json:"warnings"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env), out.String())
	require.NotEmpty(t, env.Result.Warnings)
	assert.Contains(t, strings.Join(env.Result.Warnings, "\n"), "concepts/alpha.md")
}

func TestNoteCreate_ANewNameHasNoDuplicateWarning(t *testing.T) {
	vault := buildIndexedTestVault(t)

	_, errOut, err := runRootCmd(t, "note", "create", "concepts/delta.md",
		"--type", "concept", "--field", "title=Delta Concept", "--vault", vault)
	require.NoError(t, err)
	assert.NotContains(t, errOut.String(), "already exists")
}

// Only a name counts: a title that looks like a path or equals an id is not a
// duplicate name.
func TestNoteCreate_OnlyANameIsADuplicate(t *testing.T) {
	vault := buildIndexedTestVault(t)

	for _, title := range []string{"concepts/alpha", "concepts/alpha.md", "concept-alpha"} {
		_, errOut, err := runRootCmd(t, "note", "create", "concepts/"+strings.NewReplacer("/", "-", ".", "-").Replace(title)+"-x.md",
			"--type", "concept", "--field", "title="+title, "--vault", vault)
		require.NoError(t, err)
		assert.NotContains(t, errOut.String(), "already exists", "title %q is not another note's name", title)
	}
}

// When several notes share the name, all of them are named.
func TestNoteCreate_AllNotesSharingTheNameAreNamed(t *testing.T) {
	vault := buildIndexedTestVault(t)
	_, _, err := runRootCmd(t, "note", "create", "concepts/alpha-two.md",
		"--type", "concept", "--field", "title=Alpha Concept", "--vault", vault)
	require.NoError(t, err)

	_, errOut, err := runRootCmd(t, "note", "create", "concepts/alpha-three.md",
		"--type", "concept", "--field", "title=Alpha Concept", "--vault", vault)
	require.NoError(t, err)
	assert.Contains(t, errOut.String(), "concepts/alpha.md")
	assert.Contains(t, errOut.String(), "concepts/alpha-two.md")
}
