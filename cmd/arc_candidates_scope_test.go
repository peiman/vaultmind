package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A vault path that does not exist must not read as "no arc material".
//
// `arc candidates` scans directories directly rather than opening the vault DB,
// so it never noticed a bad --vault: a typo produced "Scanned 0 episodes → 0
// candidate moments", exit 0 — identical to a real vault with nothing pending.
// The two call for opposite responses (fix the path vs. go write something), so
// they must not share a representation. Same failure this session fixed in
// `ask`, found in the command being extended to read the desk.
func TestArcCandidates_NonexistentVaultFailsClosed(t *testing.T) {
	_, _, err := runRootCmd(t, "arc", "candidates", "--vault", "/does/not/exist")
	require.Error(t, err, "a missing vault must not report an empty backlog")
	assert.Contains(t, err.Error(), "/does/not/exist")
}

// A real vault with no episodes and no desk is genuinely empty — that stays a
// success, because "nothing pending" is a legitimate answer.
func TestArcCandidates_RealButEmptyVaultSucceeds(t *testing.T) {
	_, _, err := runRootCmd(t, "arc", "candidates", "--vault", t.TempDir())
	require.NoError(t, err)
}
