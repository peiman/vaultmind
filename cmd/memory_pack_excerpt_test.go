package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `memory pack` gained --excerpt late in a long session, and nothing tested it.
// That is exactly where a wiring bug hides: the first attempt registered the
// option under app.memorycontextpack.* — a DIFFERENT command's namespace — and
// only the compiler caught it. A flag that parses but never reaches
// ContextPackConfig would compile, run, and silently do nothing.
//
// So this asserts the flag CHANGES THE OUTPUT, not merely that it exists.
func TestMemoryPackExcerpt_ChangesTheOutput(t *testing.T) {
	vault := indexedBaselineVault(t)

	capped, _, err := runRootCmd(t, "memory", "pack", "c-spreading",
		"--vault", vault, "--budget", "400", "--max-items", "3", "--excerpt", "40")
	require.NoError(t, err)

	uncapped, _, err := runRootCmd(t, "memory", "pack", "c-spreading",
		"--vault", vault, "--budget", "400", "--max-items", "3")
	require.NoError(t, err)

	require.NotEmpty(t, uncapped.String(), "precondition: the pack produced output")
	assert.NotEqual(t, uncapped.String(), capped.String(),
		"--excerpt produced byte-identical output, so it is parsed and then ignored — "+
			"exactly the shape the wrong-namespace bug would have had")
}

// The flag must be reachable from this command. A registry entry under the wrong
// prefix registers a flag the command cannot read.
func TestMemoryPackExcerpt_FlagIsRegisteredOnTheCommand(t *testing.T) {
	out, _, err := runRootCmd(t, "memory", "pack", "--help")
	require.NoError(t, err)

	help := out.String()
	require.Contains(t, help, "--excerpt", "--excerpt is not registered on `memory pack`")
	assert.True(t, strings.Contains(strings.ToLower(help), "principle"),
		"the help should say WHICH passage it prefers, so a reader knows what a cap buys")
}
