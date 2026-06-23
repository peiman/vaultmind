package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `vaultmind arc guide` must emit the self-serve arc-writing discipline so an
// adopting agent can learn the whole loop with zero human (principle-ax-design:
// "make first contact self-serve"). Assert the load-bearing sections are present
// — the manual hunt (shapes a phrase-matcher misses), the 5-part bar, the
// non-negotiables, the diff test, and the propose-only framing.
func TestArcGuideCommand_EmitsDiscipline(t *testing.T) {
	buf := &bytes.Buffer{}
	arcGuideCmd.SetOut(buf)
	require.NoError(t, arcGuideCmd.RunE(arcGuideCmd, []string{}))
	out := buf.String()

	// The 5-part bar.
	for _, want := range []string{"Trigger", "Push", "Deeper sight", "Principle", "Source"} {
		assert.Contains(t, out, want)
	}
	// The manual hunt — the shapes the candidate detector cannot catch.
	for _, want := range []string{"Reversal", "Frame-break", "Cost-of-rule", "Ownership"} {
		assert.Contains(t, out, want)
	}
	// Non-negotiables + the diff test + propose-only.
	assert.Contains(t, out, "verbatim")
	assert.Contains(t, out, "DIFF TEST")
	assert.Contains(t, out, "propose-only")
}
