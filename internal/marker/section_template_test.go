package marker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peiman/vaultmind/internal/marker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noteType comes from extractNoteType — the note's OWN frontmatter — and
// sectionKey from the markers in its body. In any vault you did not write
// yourself, both are attacker-controlled, and both are joined straight into
// .vaultmind/sections/{type}/{key}.md and read.
//
// The `.md` suffix bounds what can be read; it does not bound WHERE.
func TestLoadSectionTemplate_RejectsTraversalInNoteType(t *testing.T) {
	// Laid out so the traversal REACHES a real file. A test where the target
	// does not exist passes on template_not_found and proves nothing.
	parent := t.TempDir()
	vaultDir := filepath.Join(parent, "vault")
	require.NoError(t, os.MkdirAll(filepath.Join(vaultDir, ".vaultmind", "sections"), 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(parent, "secretdir"), 0o750))
	secret := "PRIVATE KEY MATERIAL sentinel-3c1f"
	require.NoError(t, os.WriteFile(filepath.Join(parent, "secretdir", "private.md"), []byte(secret), 0o600))

	// .vaultmind/sections/<type>/<key>.md with type = ../../../secretdir
	// resolves to <parent>/secretdir/private.md.
	data, err := marker.LoadSectionTemplate(vaultDir, filepath.Join("..", "..", "..", "secretdir"), "private")
	require.Error(t, err, "a note's own frontmatter must not be able to name a file outside the vault")
	assert.NotContains(t, string(data), secret, "the file must not be read at all")
}

func TestLoadSectionTemplate_RejectsTraversalInSectionKey(t *testing.T) {
	vaultDir := t.TempDir()
	_, err := marker.LoadSectionTemplate(vaultDir, "concept", filepath.Join("..", "..", "..", "etc", "passwd"))
	require.Error(t, err)
}

// A separator alone is enough to leave the sections directory, without any "..".
func TestLoadSectionTemplate_RejectsSeparatorInSegment(t *testing.T) {
	vaultDir := t.TempDir()
	_, err := marker.LoadSectionTemplate(vaultDir, "concept/nested", "key")
	require.Error(t, err)
}

// The ordinary path still works: a real template in the real place.
func TestLoadSectionTemplate_ReadsAValidTemplate(t *testing.T) {
	vaultDir := t.TempDir()
	dir := filepath.Join(vaultDir, ".vaultmind", "sections", "concept")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "summary.md"), []byte("# Summary\n"), 0o644))

	got, err := marker.LoadSectionTemplate(vaultDir, "concept", "summary")
	require.NoError(t, err)
	assert.Equal(t, "# Summary\n", string(got))
}
