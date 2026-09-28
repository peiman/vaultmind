package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/peiman/vaultmind/internal/cmdutil"
)

// Hashing the index reads the whole file — 18% of CPU on a three-vault ask.
// Only a JSON envelope reports the hash, so text output must never pay for it,
// and JSON output must still carry it.
func TestIndexHash_OnlyJSONOutputPaysForIt(t *testing.T) {
	vault := buildIndexedTestVault(t)
	commands := [][]string{
		{"doctor"},
		{"frontmatter", "validate"},
		{"dataview", "lint"},
		{"dataview", "render", "concepts/alpha.md"},
		{"memory", "links", "concept-alpha"},
	}
	for _, c := range commands {
		args := append(append([]string{}, c...), "--vault", vault)

		before := cmdutil.IndexHashesComputed()
		_, _, err := runRootCmd(t, args...)
		require.NoError(t, err, "%v", c)
		assert.Equal(t, before, cmdutil.IndexHashesComputed(), "%v: text output must not hash the index", c)

		before = cmdutil.IndexHashesComputed()
		_, _, err = runRootCmd(t, append(args, "--json")...)
		require.NoError(t, err, "%v --json", c)
		assert.Equal(t, before+1, cmdutil.IndexHashesComputed(), "%v --json: the envelope reports the hash", c)
	}
}
