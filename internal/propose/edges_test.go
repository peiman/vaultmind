package propose

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A drafts dir is judged after it is cleaned, so a spelling that resolves into
// the import folder, or out of the vault, cannot get a draft written there.
func TestDraftsDir_NormalizesBeforeChecking(t *testing.T) {
	tests := map[string]string{
		"notes":                          "notes",
		"notes/sub/":                     "notes/sub",
		".":                              ".",
		"":                               defaultDraftsDir,
		"notes/../" + vault.ImportFolder: defaultDraftsDir,
		"./" + vault.ImportFolder:        defaultDraftsDir,
		vault.ImportFolder + "/x":        defaultDraftsDir,
		"../outside":                     defaultDraftsDir,
		"notes/../../outside":            defaultDraftsDir,
		"/abs/path":                      defaultDraftsDir,
	}
	for in, want := range tests {
		assert.Equal(t, want, draftsDir(in), "draftsDir(%q)", in)
	}
}

func TestSlugAndPlacement(t *testing.T) {
	assert.Equal(t, "note", slugQuestion("   !!!   "))
	assert.Equal(t, strings.Repeat("a", 80), slugQuestion(strings.Repeat("a", 90)))
	assert.Equal(t, "concept-memory.proposal-20261014-153000-"+findingHash("Quartz keeps one timeline.")+".md",
		proposalPath("memory.md", "concept-memory", "Quartz keeps one timeline.", proposeNow))
	// A different finding, or a different second, never names the same draft.
	assert.NotEqual(t, proposalPath("memory.md", "concept-memory", "First.", proposeNow),
		proposalPath("memory.md", "concept-memory", "Second.", proposeNow))
	assert.NotEqual(t, proposalPath("memory.md", "concept-memory", "First.", proposeNow),
		proposalPath("memory.md", "concept-memory", "First.", proposeNow.Add(time.Second)))
	// Re-typed whitespace is the same finding.
	assert.Equal(t, findingHash("a  b\n c"), findingHash("a b c"))
	assert.Equal(t, ".", hitDir(""))
	assert.Equal(t, ".", hitDir("memory.md"))
	assert.Equal(t, "concepts/slug-20261014-1530-2.md", newRel("concepts", "slug", 2, proposeNow))
}

func TestChooseType_FallsThroughToATypeThatCanHoldADraft(t *testing.T) {
	reg := schema.NewRegistryWithAliases(map[string]vault.TypeDef{
		"overview": {Statuses: []string{"active"}},
		"alpha":    {Statuses: []string{"active"}},
		"zeta":     {},
	}, nil)
	got, err := chooseType(reg, "not-registered")
	require.NoError(t, err)
	assert.Equal(t, "zeta", got)

	onlyOverview := schema.NewRegistryWithAliases(map[string]vault.TypeDef{
		"overview": {Statuses: []string{"active"}},
	}, nil)
	_, err = chooseType(onlyOverview, "")
	require.EqualError(t, err, "vault schema has no type for a new note")
}

func TestRun_StopsBeforeRetrieval(t *testing.T) {
	req, _, _, _ := reqOf("How does the index stay current?", "A write re-indexes the note.")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Run(ctx, Vault{}, req, nil)
	require.ErrorIs(t, err, context.Canceled)

	err = Run(context.Background(), Vault{}, req, nil)
	require.EqualError(t, err, "vault is not open")

	v := testVault(t)
	err = Run(context.Background(), v, req, nil)
	require.EqualError(t, err, "no retriever")

	err = Run(context.Background(), v, req, &stubClosest{err: errors.New("down")})
	require.ErrorContains(t, err, "finding the closest note")
	require.ErrorContains(t, err, "down")
}

func TestAccept_RefusesBeforeItReads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Accept(ctx, Vault{}, Request{ID: "concept-memory"})
	require.ErrorIs(t, err, context.Canceled)

	err = Accept(context.Background(), Vault{}, Request{})
	require.EqualError(t, err, "usage: vaultmind note accept <id>")

	var buf bytes.Buffer
	err = Accept(context.Background(), Vault{}, Request{JSON: true, Stdout: &buf})
	require.ErrorIs(t, err, envelope.ErrAlreadyWritten)
	assert.Contains(t, buf.String(), "note_missing")

	err = Accept(context.Background(), Vault{}, Request{ID: "concept-memory"})
	require.EqualError(t, err, "vault is not open")
}

func TestOutput_JSONAndWarnings(t *testing.T) {
	err := fail(Request{JSON: true}, cmdAccept, "note_missing", "usage: vaultmind note accept <id>")
	require.ErrorIs(t, err, envelope.ErrAlreadyWritten)

	var buf bytes.Buffer
	err = failOp(Request{JSON: true, Stdout: &buf}, cmdPropose, "retrieval_failed", errors.New("down"))
	require.ErrorIs(t, err, envelope.ErrAlreadyWritten)
	assert.Contains(t, buf.String(), "retrieval_failed")
	assert.Contains(t, buf.String(), "down")

	err = renderPropose(Request{}, Vault{}, outcome{
		Mode: modeNew, Path: "a.md", ID: "a", Warnings: []string{"slow"},
	})
	require.NoError(t, err)

	var text, errOut bytes.Buffer
	err = renderAccept(Request{Stdout: &text, Stderr: &errOut}, Vault{}, outcome{
		Mode: modeNew, ID: "a", Status: "accepted", Warnings: []string{"slow"},
	})
	require.NoError(t, err)
	assert.Contains(t, text.String(), "accepted a (status: accepted)")
	assert.Contains(t, errOut.String(), "warning: slow")

	var jsonOut bytes.Buffer
	err = renderAccept(Request{JSON: true, Stdout: &jsonOut}, Vault{
		Path:      "/vault",
		IndexHash: func() string { return "abc" },
	}, outcome{
		Mode: modePatch, Path: "concepts/memory.md", ID: "concept-memory", TargetID: "concept-memory",
		Warnings: []string{"re-index failed: boom"},
	})
	require.NoError(t, err)
	body := jsonOut.String()
	assert.Contains(t, body, `"mode":"patch"`)
	assert.Contains(t, body, `"target_id":"concept-memory"`)
	assert.Contains(t, body, "reindex_failed")
	assert.Contains(t, body, `"index_hash":"abc"`)
}

func TestSourceURL_FileScheme(t *testing.T) {
	v := testVault(t)
	writeTarget(t, v, "concepts/memory.md", "concept-memory", "concept", "Memory", "accepted", "body")
	inside := filepath.Join(v.Path, "concepts", "memory.md")
	require.NoError(t, validateSourceURL(v.Path, "", "file://"+inside))

	bare := validateSource(v.Path, "", "/etc/passwd")
	got := validateSourceURL(v.Path, "", "file:///etc/passwd")
	require.EqualError(t, got, bare.Error())

	require.EqualError(t, validateSourceURL(v.Path, "", "file://"), `source "file://" is not a valid URL`)

	parsed, err := url.Parse("file:///C:/notes/a.md")
	require.NoError(t, err)
	assert.Equal(t, "C:/notes/a.md", fileURLPath(parsed))
	assert.Empty(t, fileURLPath(nil))
}

func TestFrontmatterHelpers(t *testing.T) {
	assert.Equal(t, []string{"a"}, stringList([]string{"a"}))
	assert.Nil(t, stringList(""))
	assert.Nil(t, stringList(3))
	assert.Equal(t, []string{"a"}, stringList([]interface{}{"a", "", 1}))
	assert.Equal(t, []string{"a", "b"}, mergeSources([]string{"a", ""}, []string{"a", "b"}))
	assert.Equal(t, "7", fmString(map[string]interface{}{"n": 7}, "n"))
}

func TestReindex_ReportsWhyItDidNotRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := reindex(ctx, Vault{}, Request{}, "a.md")
	assert.Equal(t, []string{"re-index failed: context canceled"}, cancelled)

	missing := reindex(context.Background(), Vault{}, Request{}, "a.md")
	assert.Equal(t, []string{"re-index failed: vault config is missing"}, missing)
}

func TestFreeRel_SkipsAPathThatExists(t *testing.T) {
	v := testVault(t)
	abs, err := vault.ResolveInside(v.Path, "taken.md")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(abs, []byte("x"), 0o644))

	got, err := freeRel(v, "taken.md")
	require.NoError(t, err)
	assert.Equal(t, "taken-2.md", got)
}
