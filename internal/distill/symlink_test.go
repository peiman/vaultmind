package distill_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/distill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The desk is read-only, so this is the read shape rather than the write one —
// but the desk feeds `arc candidates`, which is material a human then reads and
// distills into identity. A symlink pointing anywhere on disk would have put
// arbitrary file content into that report.
//
// Reported through diagnostics, the channel ScanDesk already uses for "there
// was material here and I did not use it".
func TestScanDesk_DoesNotFollowSymlinks(t *testing.T) {
	outside := t.TempDir()
	target := filepath.Join(outside, "private.md")
	require.NoError(t, os.WriteFile(target,
		[]byte("---\nid: journal-not-yours\ntype: journal\ntitle: Private\ndate: 2026-08-17\n---\nNot desk material.\n"), 0o644))

	desk := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(desk, "real.md"),
		[]byte("---\nid: journal-real\ntype: journal\ntitle: Real Entry\ndate: 2026-08-17\n---\nMine.\n"), 0o644))
	require.NoError(t, os.Symlink(target, filepath.Join(desk, "linked.md")))

	entries, diagnostics, err := distill.ScanDesk(desk)
	require.NoError(t, err)

	for _, e := range entries {
		assert.NotEqual(t, "journal-not-yours", e.ID,
			"a symlink pulled a file from outside the desk into the arc-candidate report")
	}
	require.Len(t, entries, 1)
	assert.Equal(t, "journal-real", entries[0].ID)

	var named bool
	for _, d := range diagnostics {
		if strings.Contains(d, "linked.md") && strings.Contains(d, "symlink") {
			named = true
		}
	}
	assert.True(t, named, "diagnostics must name the file that was passed over: %v", diagnostics)
}
