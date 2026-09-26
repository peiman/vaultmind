package hookscripts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The reach footer describes what the excerpt is without assuming a persona
// vault: the recall hook already says "a Principle section where it has one".
func TestReach_FooterDoesNotAssumeAPersonaVault(t *testing.T) {
	out := commitReach(t, `git commit -m "fix: a thing"`)
	assert.NotContains(t, out, "written as arcs")
	assert.Contains(t, out, "Principle section where it has one")
}
