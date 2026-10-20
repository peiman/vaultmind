package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotePinned_ListsPinnedNotesByID(t *testing.T) {
	isolateMeshEnv(t)
	dir := buildPinnedVault(t, map[string]string{
		"m.md": pinnedNote("note-m", "Middle", true, "MIDDLE-BODY"),
		"a.md": pinnedNote("note-a", "Alpha", true, "ALPHA-BODY"),
		"z.md": pinnedNote("note-z", "Zebra", false, "UNPINNED-BODY"),
	})

	out, _, err := runRootCmd(t, "note", "pinned", "--vault", dir)
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "note-a")
	assert.Contains(t, text, "ALPHA-BODY")
	assert.Contains(t, text, "note-m")
	assert.Contains(t, text, "MIDDLE-BODY")
	assert.NotContains(t, text, "note-z")
	assert.NotContains(t, text, "UNPINNED-BODY")
	assert.Less(t, strings.Index(text, "note-a"), strings.Index(text, "note-m"))
}

func TestNotePinned_NamesNotesThatDoNotFit(t *testing.T) {
	isolateMeshEnv(t)
	t.Setenv("VAULTMIND_CALLER", "workhorse-ledger")
	dir := buildPinnedVault(t, map[string]string{
		"a.md": pinnedNote("note-a", "Alpha", true, exactTokenBody("ALPHA-BODY")),
		"b.md": pinnedNote("note-b", "Bravo", true, exactTokenBody("BRAVO-BODY")),
		"c.md": pinnedNote("note-c", "Charlie", true, exactTokenBody("CHARLIE-BODY")),
	})

	out, _, err := runRootCmd(t, "note", "pinned", "--vault", dir, "--budget", "10")
	require.NoError(t, err)
	text := out.String()
	assert.Contains(t, text, "ALPHA-BODY")
	assert.Contains(t, text, "note-b")
	assert.Contains(t, text, "Bravo")
	assert.Contains(t, text, "note-c")
	assert.Contains(t, text, "Charlie")
	assert.NotContains(t, text, "BRAVO-BODY")
	assert.NotContains(t, text, "CHARLIE-BODY")
	assert.Contains(t, text, "DID NOT FIT")

	jout, _, err := runRootCmd(t, "note", "pinned", "--vault", dir, "--budget", "10", "--json")
	require.NoError(t, err)
	var env pinnedEnvelope
	require.NoError(t, json.Unmarshal(jout.Bytes(), &env))
	assert.Equal(t, "note pinned", env.Command)
	require.Len(t, env.Result.Items, 1)
	assert.Equal(t, "note-a", env.Result.Items[0].ID)
	assert.Contains(t, env.Result.Items[0].Excerpt, "ALPHA-BODY")
	require.Len(t, env.Result.Omitted, 2)
	assert.Equal(t, "note-b", env.Result.Omitted[0].ID)
	assert.Equal(t, "Bravo", env.Result.Omitted[0].Title)
	assert.Equal(t, "note-c", env.Result.Omitted[1].ID)
	assert.Equal(t, "Charlie", env.Result.Omitted[1].Title)
	raw := jout.String()
	assert.NotContains(t, raw, "BRAVO-BODY")
	assert.NotContains(t, raw, "CHARLIE-BODY")

	db, err := index.Open(filepath.Join(dir, ".vaultmind", "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	var callers []string
	rows, err := db.Query(`SELECT note_id, caller FROM note_accesses ORDER BY note_id`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, caller string
		require.NoError(t, rows.Scan(&id, &caller))
		callers = append(callers, id+"="+caller)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"note-a=workhorse-ledger", "note-a=workhorse-ledger"}, callers)
}

func TestNotePinned_OneBudgetAcrossVaults(t *testing.T) {
	isolateMeshEnv(t)
	// note-z sorts before note-a by id. Vault order wins: the first vault's
	// note is kept and the second is named, even though its id is earlier.
	first := buildPinnedVault(t, map[string]string{
		"z.md": pinnedNote("note-z", "Zebra", true, exactTokenBody("ZEBRA-BODY")),
	})
	second := buildPinnedVault(t, map[string]string{
		"a.md": pinnedNote("note-a", "Alpha", true, exactTokenBody("ALPHA-BODY")),
	})

	out, _, err := runRootCmd(t, "note", "pinned", "--vaults", first+","+second, "--budget", "10")
	require.NoError(t, err)
	text := out.String()
	assert.Equal(t, 1, strings.Count(text, "PINNED —"))
	assert.Contains(t, text, "ZEBRA-BODY")
	assert.NotContains(t, text, "ALPHA-BODY")
	assert.Contains(t, text, "note-a")
	assert.Contains(t, text, "Alpha")
	assert.Contains(t, text, second, "an omitted note is named with its vault")
	assert.Less(t, strings.Index(text, "note-z"), strings.Index(text, "note-a"))

	jout, _, err := runRootCmd(t, "note", "pinned", "--vaults", first+","+second, "--budget", "10", "--json")
	require.NoError(t, err)
	var env pinnedEnvelope
	require.NoError(t, json.Unmarshal(jout.Bytes(), &env))
	require.Len(t, env.Result.Items, 1)
	assert.Equal(t, "note-z", env.Result.Items[0].ID)
	assert.Equal(t, first, env.Result.Items[0].Vault)
	require.Len(t, env.Result.Omitted, 1)
	assert.Equal(t, "note-a", env.Result.Omitted[0].ID)
	assert.Equal(t, "Alpha", env.Result.Omitted[0].Title)
	assert.Equal(t, second, env.Result.Omitted[0].Vault)
	assert.NotContains(t, jout.String(), "ALPHA-BODY")
}

func TestNotePinnedBudgetDefaultIsTheSessionBudget(t *testing.T) {
	var found bool
	for _, opt := range commands.NotePinnedOptions() {
		if opt.Key == "app.notepinned.budget" {
			found = true
			assert.Equal(t, memory.SessionPinnedBudget, opt.DefaultValue)
		}
	}
	assert.True(t, found)
}

func TestNotePinned_VaultsRejectsAPathThatIsNotAVault(t *testing.T) {
	isolateMeshEnv(t)
	good := buildPinnedVault(t, map[string]string{
		"a.md": pinnedNote("note-a", "Alpha", true, "ALPHA-BODY"),
	})

	_, _, err := runRootCmd(t, "note", "pinned", "--vaults", good+","+t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a vault")
}

func TestNotePinned_VaultsRejectsAnEmptyList(t *testing.T) {
	isolateMeshEnv(t)

	_, _, err := runRootCmd(t, "note", "pinned", "--vaults", " , ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no usable vault")
}

func TestNotePinned_VaultsAFailingVaultIsOneNamedError(t *testing.T) {
	isolateMeshEnv(t)
	good := buildPinnedVault(t, map[string]string{
		"a.md": pinnedNote("note-a", "Alpha", true, "ALPHA-BODY"),
	})
	bad := brokenVault(t)

	out, _, err := runRootCmd(t, "note", "pinned", "--vaults", good+","+bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), bad)
	assert.Empty(t, out.String(), "nothing is printed before the error")

	jout, _, _ := runRootCmd(t, "note", "pinned", "--vaults", good+","+bad, "--json")
	var env struct {
		Status string `json:"status"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(jout.Bytes(), &env), "one JSON envelope: %s", jout.String())
	assert.Equal(t, "error", env.Status)
	require.NotEmpty(t, env.Errors)
	assert.Contains(t, env.Errors[0].Message, bad)
}

func TestNotePinned_ANoteThatCannotBeLoadedNamesTheID(t *testing.T) {
	isolateMeshEnv(t)
	dir := buildPinnedVault(t, map[string]string{
		"a.md": pinnedNote("note-a", "Alpha", true, "ALPHA-BODY"),
	})
	db, err := index.Open(filepath.Join(dir, ".vaultmind", "index.db"))
	require.NoError(t, err)
	_, err = db.Exec(`DROP TABLE aliases`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, _, err = runRootCmd(t, "note", "pinned", "--vault", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `pinned: loading "note-a"`)
}

func TestNotePinned_EmptyBodyIsNamedAsNoBody(t *testing.T) {
	isolateMeshEnv(t)
	dir := buildPinnedVault(t, map[string]string{
		"a.md": pinnedNote("note-a", "Alpha", true, ""),
	})

	out, _, err := runRootCmd(t, "note", "pinned", "--vault", dir)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "(no body)")
}

func TestWritePinned_VaultlessResultHasNoTrailingVault(t *testing.T) {
	var buf bytes.Buffer
	err := writePinned(&buf, &memory.PinnedResult{
		Total:   2,
		Budget:  10,
		Items:   []memory.PinnedItem{{ID: "note-a", Title: "Alpha"}},
		Omitted: []memory.OmittedNote{{ID: "note-b", Title: "Bravo"}},
	})
	require.NoError(t, err)
	text := buf.String()
	assert.Contains(t, text, "── note-a — Alpha\n")
	assert.Contains(t, text, "(no body)")
	assert.Contains(t, text, "   note-b — Bravo\n")
	assert.NotContains(t, text, "── note-a — Alpha —")
}

func TestWritePinned_WriteError(t *testing.T) {
	err := writePinned(&failAfterNWriter{ok: 0}, &memory.PinnedResult{
		Total: 1,
		Items: []memory.PinnedItem{{ID: "note-a", Title: "Alpha", Excerpt: "ALPHA-BODY"}},
	})
	require.Error(t, err)
}

func TestTrackPinnedReads_SkipsANoteFromAVaultThatWasNotOpened(t *testing.T) {
	trackPinnedReads(&cobra.Command{}, nil, &memory.PinnedResult{
		Items: []memory.PinnedItem{{ID: "note-a", Excerpt: "ALPHA-BODY", Vault: "/missing"}},
	})
}

func TestTrackPinnedReads_AClosedVaultDoesNotFailTheLoad(t *testing.T) {
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	require.NoError(t, err)
	require.NoError(t, db.Close())
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	trackPinnedReads(cmd, []memory.PinnedVault{{DB: db, Vault: "/v"}}, &memory.PinnedResult{
		Items: []memory.PinnedItem{{ID: "note-a", Excerpt: "ALPHA-BODY", Vault: "/v"}},
	})
}

func TestTrackPinnedReads_RecordsTheSession(t *testing.T) {
	isolateMeshEnv(t)
	dir := buildPinnedVault(t, map[string]string{
		"a.md": pinnedNote("note-a", "Alpha", true, "ALPHA-BODY"),
	})
	db, err := index.Open(filepath.Join(dir, ".vaultmind", "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	exp := openTestExpDB(t)
	sid, err := exp.StartSession(dir)
	require.NoError(t, err)
	session := &experiment.Session{DB: exp, ID: sid}
	cmd := &cobra.Command{}
	cmd.SetContext(experiment.WithSession(context.Background(), session))

	trackPinnedReads(cmd, []memory.PinnedVault{{DB: db, Vault: dir}}, &memory.PinnedResult{
		Items: []memory.PinnedItem{{ID: "note-a", Excerpt: "ALPHA-BODY", Vault: dir}},
	})
	assert.Equal(t, dir, session.VaultPath)
}

func TestNotePinned_NoPinnedNotesPrintsNothing(t *testing.T) {
	isolateMeshEnv(t)
	dir := buildPinnedVault(t, map[string]string{
		"z.md": pinnedNote("note-z", "Zebra", false, "UNPINNED-BODY"),
	})

	out, _, err := runRootCmd(t, "note", "pinned", "--vault", dir)
	require.NoError(t, err)
	assert.Empty(t, out.String())

	jout, _, err := runRootCmd(t, "note", "pinned", "--vault", dir, "--json")
	require.NoError(t, err)
	assert.Empty(t, jout.String())
}

type pinnedEnvelope struct {
	Command string `json:"command"`
	Result  struct {
		Items []struct {
			ID      string `json:"id"`
			Excerpt string `json:"excerpt"`
			Vault   string `json:"vault"`
		} `json:"items"`
		Omitted []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Vault string `json:"vault"`
		} `json:"omitted"`
	} `json:"result"`
}

func buildPinnedVault(t *testing.T, notes map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".vaultmind"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".vaultmind", "config.yaml"), []byte(`
types:
  concept:
    required: [title]
    optional: [tags, related_ids]
`), 0o644))
	for rel, content := range notes {
		writeTestNote(t, dir, rel, content)
	}
	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	dbPath := filepath.Join(dir, cfg.Index.DBPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	_, err = index.NewIndexer(dir, dbPath, cfg).Rebuild()
	require.NoError(t, err)
	return dir
}

func pinnedNote(id, title string, pinned bool, body string) string {
	pin := ""
	if pinned {
		pin = "pinned: true\n"
	}
	return fmt.Sprintf("---\nid: %s\ntype: concept\ntitle: %s\n%s---\n%s", id, title, pin, body)
}

// exactTokenBody is 40 bytes, which EstimateTokens counts as 10.
func exactTokenBody(sentinel string) string {
	if len(sentinel) > 40 {
		panic("sentinel longer than 40")
	}
	return sentinel + strings.Repeat("x", 40-len(sentinel))
}
