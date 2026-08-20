package vault_test

import (
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every case in TestResolveInside uses an ABSOLUTE root. That is the dimension
// the bug lived in: with a relative root the containment test is a string-prefix
// comparison between two relative paths, and filepath.Join collapses the "./"
// that the prefix depends on.
//
//	ResolveInside(".", ".vaultmind/index.db")
//	  cleanVault = "."
//	  cleanAbs   = ".vaultmind/index.db"        <- Join dropped the "./"
//	  HasPrefix(".vaultmind/index.db", "./")    <- false
//	  => ErrEscapesVault
//
// A path plainly inside the vault was refused as an escape. Live effect: every
// command failed when the vault was named as `.`, while OMITTING the flag —
// which means the same directory — worked, because that path is resolved to
// absolute before it ever reaches here.
//
//	$ cd <vault> && vaultmind doctor --vault .
//	Error: index db_path: ".vaultmind/index.db": path escapes vault
//
// It also contradicted the function's own doc comment, which promises "the
// cleaned absolute path" — with a relative root it returned a relative one, and
// every caller names the result absPath or dbPath.
func TestResolveInside_RelativeRoot(t *testing.T) {
	t.Run("dot as the root resolves a path inside it", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)

		got, err := vault.ResolveInside(".", filepath.Join(".vaultmind", "index.db"))
		require.NoError(t, err, "a path inside the vault must not read as an escape")
		assert.Equal(t, filepath.Join(dir, ".vaultmind", "index.db"), got)
	})

	t.Run("dot-slash is the same thing", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)

		got, err := vault.ResolveInside("./", filepath.Join("notes", "a.md"))
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, "notes", "a.md"), got)
	})

	t.Run("a relative subdirectory root works", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)

		got, err := vault.ResolveInside("vaultmind-identity", "arcs/a.md")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, "vaultmind-identity", "arcs", "a.md"), got)
	})

	// The half that matters more: loosening the relative case must not loosen
	// containment. These are the same escapes the absolute-root tests cover,
	// re-run through a relative root.
	t.Run("traversal out of a relative root is still refused", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)

		for _, rel := range []string{
			filepath.Join("..", "..", "etc", "passwd"),
			filepath.Join("..", "sibling", "secret.md"),
			"../outside.md",
		} {
			_, err := vault.ResolveInside(".", rel)
			require.ErrorIs(t, err, vault.ErrEscapesVault,
				"%q escapes a relative root and must be refused", rel)
		}
	})

	t.Run("a prefix-sharing sibling is still outside a relative root", func(t *testing.T) {
		t.Chdir(t.TempDir())

		_, e := vault.ResolveInside("mine", filepath.Join("..", "mine-backup", "secret.md"))
		require.ErrorIs(t, e, vault.ErrEscapesVault,
			"mine-backup must not pass a prefix check against mine")
	})

	t.Run("the root itself, named relatively, is inside", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)

		got, e := vault.ResolveInside(".", ".")
		require.NoError(t, e)
		assert.Equal(t, dir, got)
	})

	// The contract the doc comment already promised.
	t.Run("the result is always absolute", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)

		got, e := vault.ResolveInside(".", "notes/a.md")
		require.NoError(t, e)
		assert.True(t, filepath.IsAbs(got), "ResolveInside documents an absolute path; got %q", got)
	})
}
