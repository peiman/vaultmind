package vault_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveInside(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "vaults", "mine")

	t.Run("an ordinary relative path resolves under the root", func(t *testing.T) {
		got, err := vault.ResolveInside(root, filepath.Join("concepts", "act-r.md"))
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(root, "concepts", "act-r.md"), got)
	})

	t.Run("the root itself is inside", func(t *testing.T) {
		got, err := vault.ResolveInside(root, ".")
		require.NoError(t, err)
		assert.Equal(t, root, got)
	})

	t.Run("dot-dot traversal is refused", func(t *testing.T) {
		_, err := vault.ResolveInside(root, filepath.Join("..", "..", "etc", "passwd"))
		require.ErrorIs(t, err, vault.ErrEscapesVault)
		assert.Contains(t, err.Error(), "etc", "the error must name the path that was refused")
	})

	t.Run("traversal that dips out and comes back is refused on the resolved path", func(t *testing.T) {
		// Cleaning happens before the check, so this is contained and allowed —
		// pinning it because the opposite (rejecting on a substring match of
		// "..") is the tempting wrong implementation.
		got, err := vault.ResolveInside(root, filepath.Join("concepts", "..", "arcs", "a.md"))
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(root, "arcs", "a.md"), got)
	})

	t.Run("a sibling directory sharing a name prefix is outside", func(t *testing.T) {
		// /vaults/mine-backup must not pass a check written as HasPrefix on the
		// root without a separator.
		_, err := vault.ResolveInside(root, filepath.Join("..", "mine-backup", "secret.md"))
		require.ErrorIs(t, err, vault.ErrEscapesVault)
	})

	t.Run("an absolute path is re-rooted, not escaped", func(t *testing.T) {
		// Documented behaviour, not an endorsement: filepath.Join re-roots an
		// absolute second argument. It is contained, so confinement passes —
		// and it is NOT the path the operator wrote, which is why absolute
		// values are rejected where they are configured instead.
		got, err := vault.ResolveInside(root, filepath.Join(string(filepath.Separator), "etc", "passwd"))
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(root, "etc", "passwd"), got)
	})

	t.Run("the sentinel is matchable with errors.Is", func(t *testing.T) {
		_, err := vault.ResolveInside(root, "../outside.md")
		assert.True(t, errors.Is(err, vault.ErrEscapesVault),
			"callers wrap this into their own error codes and need errors.Is to work")
	})
}

func TestValidSegment(t *testing.T) {
	for _, ok := range []string{"concept", "arc-note", "section_key", "a1", "A", "9"} {
		assert.True(t, vault.ValidSegment(ok), "%q should be a legal segment", ok)
	}
	// The first two are the attack: a note's frontmatter `type` joined into a
	// template path. The rest are the near-misses a permissive check waves
	// through — a separator, a hidden-file dot, whitespace, a NUL.
	for _, bad := range []string{
		"..", "../../etc", "a/b", "a\\b", ".hidden", "with space", "semi;colon", "", "a\x00b",
	} {
		assert.False(t, vault.ValidSegment(bad), "%q must not be a legal segment", bad)
	}
}
