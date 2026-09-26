package hookscripts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The reach hook fires in bursts — commit, push, merge within a minute — and
// measured 82% of its delivered bodies in a conversation as repeats, 68% of
// them within five minutes. It asks with a short dedup window, so a note it
// already delivered comes back as its title.
func TestReach_AsksWithAShortDedupWindow(t *testing.T) {
	h := newHookEnv(t, argsStub)
	out, _ := runHookScript(t, "vault-reach.sh", h.env(false), bashPayload(`git commit -m "fix: a thing"`))
	assert.Contains(t, out, "--dedup-window 10m")
}
