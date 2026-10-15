package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// note propose writes the draft and then writes the rank keys. Each write
// embeds. note accept closes a linked gap and then re-indexes the accepted
// note. Each of those passes used to print the embed line, and to warn about
// every pending empty note, including ones the command did not write.
func TestNoteProposeAndAccept_PrintEmbedOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	emb, _ := useLoadCountEmbedder(t)
	dir := proposeVault(t)
	writeVaultNote(t, dir, "concepts/empty-foreign.md", "empty-foreign", "Empty", "accepted", "")
	writeGapWithBody(t, dir)
	rebuildVault(t, dir)
	seedMiniLMEmbeddings(t, dir, emb.vec)
	clearEmbedding(t, dir, "empty-foreign")

	out, errOut, err := runRootCmd(t, "note", "propose",
		"--vault", dir,
		"--question", "Where does the embedder session live?",
		"--finding", "The process keeps one session for ranking and for the draft.",
		"--close-band", "not-a-real-band",
		"--gap", "gap-embed-once")
	require.NoError(t, err, "stdout: %s\nstderr: %s", out.String(), errOut.String())
	assert.Equal(t, 1, strings.Count(errOut.String(), "embedded "), "propose stderr:\n%s", errOut.String())
	assert.Zero(t, strings.Count(errOut.String(), "id=empty-foreign"), "propose stderr:\n%s", errOut.String())
	assert.NotContains(t, errOut.String(), "skipping note with empty body")

	id := draftIDFromPropose(t, out.String())
	db, err := index.Open(filepath.Join(dir, ".vaultmind", "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	var blob []byte
	require.NoError(t, db.QueryRow(`SELECT embedding FROM notes WHERE id = ?`, id).Scan(&blob))
	assert.NotEmpty(t, blob, "the second embed still stores the draft vector")
	_ = db.Close()

	_, acceptErr, err := runRootCmd(t, "note", "accept", id, "--vault", dir)
	require.NoError(t, err, "stderr: %s", acceptErr.String())
	assert.Equal(t, 1, strings.Count(acceptErr.String(), "embedded "), "accept stderr:\n%s", acceptErr.String())
	assert.Zero(t, strings.Count(acceptErr.String(), "id=empty-foreign"), "accept stderr:\n%s", acceptErr.String())
	assert.LessOrEqual(t, strings.Count(acceptErr.String(), "skipping note with empty body"), 1, "accept stderr:\n%s", acceptErr.String())
}

func TestEmbedPrint_WarnsOnceForANoteThisCommandWrote(t *testing.T) {
	p := &embedPrint{written: map[string]struct{}{"notes/draft.md": {}}}
	assert.True(t, p.allow("draft", "notes/draft.md"))
	assert.False(t, p.allow("draft", "notes/draft.md"))
	assert.False(t, p.allow("other", "concepts/empty.md"))
	assert.True(t, (*embedPrint)(nil).allow("x", "y.md"))
}

func writeGapWithBody(t *testing.T, dir string) {
	t.Helper()
	body := "---\nid: gap-embed-once\ntype: gap\ntitle: Where does the embedder warn?\nstatus: open\nquestion: Where does the embedder warn?\ncreated: 2026-04-03\n---\n\nThe gap has a body so accepting it embeds.\n"
	path := filepath.Join(dir, "gaps", "embed-once.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func clearEmbedding(t *testing.T, dir, id string) {
	t.Helper()
	db, err := index.Open(filepath.Join(dir, ".vaultmind", "index.db"))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`UPDATE notes SET embedding = NULL WHERE id = ?`, id)
	require.NoError(t, err)
}

func draftIDFromPropose(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "id: ") {
			return strings.TrimPrefix(line, "id: ")
		}
	}
	t.Fatalf("propose output has no id line:\n%s", out)
	return ""
}
