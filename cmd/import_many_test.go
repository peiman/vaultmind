package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImport_ManyIndexFailureIsReturnedWithoutPrintingItInline(t *testing.T) {
	var out bytes.Buffer
	outcomes := []importOutcome{{Arg: "docs", Err: errors.New("index failed"), Res: &importResult{
		Vault: "vault", Source: "docs", Result: &importdocs.Result{}, IndexError: "index failed",
	}}}
	require.NoError(t, writeImportManyText(&out, outcomes))
	assert.Contains(t, out.String(), "Imported docs into vault")
	assert.NotContains(t, out.String(), "index failed")
	assert.EqualError(t, importManyError(outcomes), "import: docs: index failed")
}

// Two sources in one call import both, in order, each with the report a
// single import of that source would print.
func TestImport_TwoSourcesInOneCallImportBoth(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	extra := filepath.Join(repo, "EXTRA.md")
	require.NoError(t, os.WriteFile(extra, []byte("# Extra\n\nRoot note.\n"), 0o600))

	docs := filepath.Join(repo, "docs")
	out, _, err := runRootCmd(t, "import", docs, extra, "--vault", vault)
	require.NoError(t, err)
	text := out.String()
	docsAt := assertContainsAt(t, text, "Imported demo-repo:docs into")
	extraAt := assertContainsAt(t, text, "Imported demo-repo:EXTRA.md into")
	assert.Less(t, docsAt, extraAt, "sources import in the order given")
	assert.Contains(t, text, "2 added")
	assert.Contains(t, text, "1 added")

	_, _, err = runRootCmd(t, "note", "get", "imported-demo-repo-docs-alpha", "--vault", vault, "--json")
	require.NoError(t, err, "the folder imported")
	_, _, err = runRootCmd(t, "note", "get", "imported-demo-repo-extra", "--vault", vault, "--json")
	require.NoError(t, err, "the file imported")
}

// A missing source is named, the sources before it stay imported, and the
// command exits non-zero.
func TestImport_MissingSecondSourceIsNamedAndTheFirstStillImports(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	missing := filepath.Join(repo, "no-such.md")

	out, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), missing, "--vault", vault)
	require.Error(t, err)
	_, statErr := os.Stat(missing)
	require.Error(t, statErr)
	assert.EqualError(t, err, fmt.Sprintf("import: %s: %v", missing, statErr))
	assert.Contains(t, out.String(), "Imported demo-repo:docs into")
	assert.Contains(t, out.String(), "2 added")
	assert.NotContains(t, out.String(), "no-such.md", "the returned error is printed by main, once")

	_, _, err = runRootCmd(t, "note", "get", "imported-demo-repo-docs-alpha", "--vault", vault, "--json")
	require.NoError(t, err, "the first source still imported")
}

// --json lists each source. A failure keeps the successes in the envelope
// and exits non-zero.
func TestImport_JSONReportsEachSource(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	extra := filepath.Join(repo, "EXTRA.md")
	require.NoError(t, os.WriteFile(extra, []byte("# Extra\n\nRoot note.\n"), 0o600))
	docs := filepath.Join(repo, "docs")

	out, _, err := runRootCmd(t, "import", docs, extra, "--vault", vault, "--json")
	require.NoError(t, err)
	env := decodeImportMany(t, out.Bytes())
	assert.Equal(t, "ok", env.Status)
	require.Len(t, env.Result.Sources, 2)
	assert.Equal(t, docs, env.Result.Sources[0].Arg)
	assert.Equal(t, 2, env.Result.Sources[0].Result.Counts["added"])
	assert.Equal(t, extra, env.Result.Sources[1].Arg)
	assert.Equal(t, 1, env.Result.Sources[1].Result.Counts["added"])
	assert.Empty(t, env.Result.Sources[0].Error)
	assert.Empty(t, env.Errors)
}

func TestImport_JSONNamesAFailedSourceAndKeepsTheFirst(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	docs := filepath.Join(repo, "docs")
	missing := filepath.Join(repo, "no-such.md")

	out, _, err := runRootCmd(t, "import", docs, missing, "--vault", vault, "--json")
	require.Error(t, err)
	env := decodeImportMany(t, out.Bytes())
	assert.Equal(t, "error", env.Status)
	require.Len(t, env.Result.Sources, 2)
	assert.Equal(t, 2, env.Result.Sources[0].Result.Counts["added"])
	assert.Contains(t, env.Result.Sources[1].Error, "no-such.md")
	require.NotEmpty(t, env.Errors)
	assert.Contains(t, env.Errors[0].Message, "no-such.md")

	_, _, err = runRootCmd(t, "note", "get", "imported-demo-repo-docs-alpha", "--vault", vault, "--json")
	require.NoError(t, err, "the first source still imported")
}

// --watch is one source kept in step. Several sources are refused before
// anything is written.
func TestImport_WatchRefusesMoreThanOneSource(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	_, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), filepath.Join(repo, "docs", "alpha.md"),
		"--vault", vault, "--watch")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--watch")
	assert.NoDirExists(t, filepath.Join(vault, "imported"))
}

func assertContainsAt(t *testing.T, text, sub string) int {
	t.Helper()
	i := strings.Index(text, sub)
	require.GreaterOrEqual(t, i, 0, "missing %q in:\n%s", sub, text)
	return i
}

type importManyEnvelope struct {
	Status string `json:"status"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
	Result struct {
		Sources []struct {
			Arg    string `json:"arg"`
			Error  string `json:"error"`
			Result struct {
				Counts map[string]int `json:"counts"`
			} `json:"result"`
		} `json:"sources"`
	} `json:"result"`
}

func decodeImportMany(t *testing.T, raw []byte) importManyEnvelope {
	t.Helper()
	var env importManyEnvelope
	require.NoError(t, json.Unmarshal(raw, &env), string(raw))
	return env
}

// The source after a failure still runs, including a URL mixed with local docs.
func TestImport_ContinuesAfterFailedSources(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, jsonMode := range []bool{false, true} {
		t.Run(fmt.Sprint(jsonMode), func(t *testing.T) {
			vault, repo := indexedBaselineVault(t), docsRepo(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/markdown")
				_, _ = io.WriteString(w, "# Web guide\n\nRemote instructions.\n")
			}))
			t.Cleanup(srv.Close)
			missing := filepath.Join(repo, "missing.md")
			empty := t.TempDir()
			args := []string{"import", missing, filepath.Join(repo, "docs"), empty, srv.URL + "/guide.md", "--vault", vault}
			if jsonMode {
				args = append(args, "--json")
			}
			out, _, err := runRootCmd(t, args...)
			require.Error(t, err)
			if jsonMode {
				env := decodeImportMany(t, out.Bytes())
				require.Len(t, env.Result.Sources, 4)
				assert.Len(t, env.Errors, 2)
				assert.NotEmpty(t, env.Result.Sources[0].Error)
				assert.Equal(t, 2, env.Result.Sources[1].Result.Counts["added"])
				assert.NotEmpty(t, env.Result.Sources[2].Error)
				assert.Equal(t, 1, env.Result.Sources[3].Result.Counts["added"])
			} else {
				assert.Contains(t, err.Error(), missing)
				assert.Contains(t, err.Error(), empty)
				assert.Contains(t, out.String(), "Imported "+srv.URL+"/guide.md")
			}
			_, _, err = runRootCmd(t, "note", "get", "imported-demo-repo-docs-alpha", "--vault", vault, "--json")
			require.NoError(t, err)
		})
	}
}

// Freeze the pre-multi-source stdout, including whitespace and JSON field order.
func TestImport_SingleSourceOutputUnchanged(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	out, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", vault)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("Imported demo-repo:docs into %s: 2 added\n  added     imported/demo-repo/docs/alpha.md\n  added     imported/demo-repo/docs/beta.md\n", vault), out.String())

	out, _, err = runRootCmd(t, "import", filepath.Join(repo, "docs"), "--vault", vault, "--json")
	require.NoError(t, err)
	// The timestamp varies; replace only its value before comparing bytes.
	var env struct {
		Meta struct {
			Timestamp string `json:"timestamp"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &env))
	require.NotEmpty(t, env.Meta.Timestamp)
	got := strings.Replace(out.String(), env.Meta.Timestamp, "TIMESTAMP", 1)
	vaultJSON, err := json.Marshal(vault)
	require.NoError(t, err)
	want := fmt.Sprintf(`{"schema_version":"v1","command":"import","status":"ok","warnings":[],"errors":[],"result":{"vault":%s,"source":"demo-repo:docs","entries":[{"action":"unchanged","note":"imported/demo-repo/docs/alpha.md","source":"demo-repo:docs/alpha.md"},{"action":"unchanged","note":"imported/demo-repo/docs/beta.md","source":"demo-repo:docs/beta.md"}],"dry_run":false,"counts":{"unchanged":2}},"meta":{"vault_path":"","index_hash":"","timestamp":"TIMESTAMP"}}
`, vaultJSON)
	assert.Equal(t, want, got)
}

func TestImport_MultiSourceWatchJSONErrorUsesEnvelope(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	vault, repo := indexedBaselineVault(t), docsRepo(t)
	out, _, err := runRootCmd(t, "import", filepath.Join(repo, "docs"), filepath.Join(repo, "docs", "alpha.md"),
		"--vault", vault, "--watch", "--json")
	require.Error(t, err)
	env := decodeImportMany(t, out.Bytes())
	assert.Equal(t, "error", env.Status)
	require.Len(t, env.Errors, 1)
	assert.Contains(t, env.Errors[0].Message, "--watch")
	assert.NoDirExists(t, filepath.Join(vault, "imported"))
}
