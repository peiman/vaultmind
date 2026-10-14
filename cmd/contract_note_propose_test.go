package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const proposeConceptConfig = `types:
  concept:
    required: [title]
    optional: [aliases, tags, sources]
    template: .vaultmind/templates/concept.md
`

const proposeConceptTemplate = "---\nid: <%=id%>\ntype: <%=type%>\ntitle: <%=title%>\ncreated: <%=created%>\n---\n"

func proposeVault(t *testing.T) string {
	t.Helper()
	isolateXDG(t)
	dir := setupNoteCreateVault(t, proposeConceptConfig, map[string]string{
		".vaultmind/templates/concept.md": proposeConceptTemplate,
	})
	writeVaultNote(t, dir, "concepts/memory.md", "concept-memory", "Memory", "accepted",
		strings.Repeat("sharedrecalltoken publishedtoken ", 8))
	rebuildVault(t, dir)
	return dir
}

func TestContract_NoteProposeJSON(t *testing.T) {
	dir := proposeVault(t)
	out, errOut, err := runRootCmd(t, "note", "propose",
		"--vault", dir,
		"--question", "How does quartz work?",
		"--finding", "Quartz rebuilds the site when a note is saved.",
		"--close-band", "not-a-real-band",
		"--source", "https://example.com/quartz",
		"--json")
	require.NoError(t, err, "stdout: %s\nstderr: %s", out.String(), errOut.String())

	var env NoteProposeEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, "note propose", env.Command)
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "new", env.Result.Mode)
	assert.NotEmpty(t, env.Result.Path)
	assert.NotEmpty(t, env.Result.ID)
	assert.Empty(t, env.Result.TargetID)
	assert.Contains(t, env.Result.NextStep, "vaultmind note accept "+env.Result.ID)
	assert.Equal(t, dir, env.Meta.VaultPath)
	assert.NotEmpty(t, env.Meta.IndexHash)
}

func TestContract_NoteAcceptJSON(t *testing.T) {
	dir := proposeVault(t)
	proposed, _, err := runRootCmd(t, "note", "propose",
		"--vault", dir,
		"--question", "How does quartz work?",
		"--finding", "Quartz rebuilds the site when a note is saved.",
		"--close-band", "not-a-real-band",
		"--json")
	require.NoError(t, err)
	var draft NoteProposeEnvelope
	require.NoError(t, json.Unmarshal(proposed.Bytes(), &draft))
	require.Equal(t, "new", draft.Result.Mode)

	out, _, err := runRootCmd(t, "note", "accept", draft.Result.ID, "--vault", dir, "--json")
	require.NoError(t, err)

	var env NoteAcceptEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	assert.Equal(t, "v1", env.SchemaVersion)
	assert.Equal(t, "note accept", env.Command)
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "new", env.Result.Mode)
	assert.Equal(t, draft.Result.ID, env.Result.ID)
	assert.Equal(t, draft.Result.Path, env.Result.Path)
	assert.Equal(t, "accepted", env.Result.Status)
	assert.Empty(t, env.Result.TargetID)
}

func TestPropose_WrittenNoteValidatesAndAskOmitsDrafts(t *testing.T) {
	dir := proposeVault(t)
	out, _, err := runRootCmd(t, "note", "propose",
		"--vault", dir,
		"--question", "Where does sharedrecalltoken live?",
		"--finding", "draftonlytokenzz answers sharedrecalltoken",
		"--json")
	require.NoError(t, err)

	var env NoteProposeEnvelope
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.NotEmpty(t, env.Result.ID)
	require.Contains(t, []string{"new", "patch"}, env.Result.Mode)

	db, err := index.Open(filepath.Join(dir, ".vaultmind", "index.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	row, err := db.QueryNoteByID(env.Result.ID)
	require.NoError(t, err)
	require.NotNil(t, row, "the draft must be indexed, or an ask that omits it proves nothing")
	assert.Equal(t, index.StatusDraft, row.Status)

	cfg, err := vault.LoadConfig(dir)
	require.NoError(t, err)
	reg := schema.NewRegistryWithAliases(cfg.Types, cfg.Schema.Aliases)
	report, err := query.Validate(db, reg)
	require.NoError(t, err)
	var schemaErrors []string
	for _, issue := range report.Issues {
		if issue.ID == env.Result.ID && issue.Severity == "error" {
			schemaErrors = append(schemaErrors, issue.Message)
		}
	}
	assert.Empty(t, schemaErrors)

	askOut, _, err := runRootCmd(t, "ask", "sharedrecalltoken", "--vault", dir, "--json", "--pointers-only")
	require.NoError(t, err)
	var asked AskEnvelope
	require.NoError(t, json.Unmarshal(askOut.Bytes(), &asked))
	ids := make([]string, 0, len(asked.Result.TopHits))
	for _, hit := range asked.Result.TopHits {
		ids = append(ids, hit.ID)
		assert.NotEqual(t, env.Result.ID, hit.ID, "a default ask must leave the draft out")
	}
	assert.Contains(t, ids, "concept-memory")
}
