package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/importdocs"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/navigate"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
)

// importEnvelope names the command in its JSON envelope.
const importEnvelope = "import"

// importResult is the report plus where it went.
type importResult struct {
	Vault  string `json:"vault"`
	Source string `json:"source"`
	*importdocs.Result
	Counts map[importdocs.Action]int `json:"counts"`
	// IndexError is set when the notes were written but re-indexing failed.
	IndexError string `json:"index_error,omitempty"`
	// IndexWarnings names imported notes the index did not take: a duplicate
	// id, a parse failure. Written is not the same as findable.
	IndexWarnings []string `json:"index_warnings,omitempty"`
}

// importDocs imports a folder, or one http(s) page, into the vault, then
// re-indexes and embeds what changed. A scheme other than http or https is
// refused before the vault is opened and before any network call.
func importDocs(cmd *cobra.Command, arg string) (*importResult, error) {
	if err := unsupportedImportURL(arg); err != nil {
		return nil, importErr(cmd, err)
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppImportVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, importEnvelope)
	if err != nil {
		return nil, err
	}
	cfg := vdb.Config
	vdb.Close()

	opts := importdocs.Options{
		DryRun: getConfigValueWithFlags[bool](cmd, "dry-run", config.KeyAppImportDryRun),
		Prune:  getConfigValueWithFlags[bool](cmd, "prune", config.KeyAppImportPrune),
		Force:  getConfigValueWithFlags[bool](cmd, "force", config.KeyAppImportForce),
		// The vault's own exclude list: a note it hides is never written.
		Excludes: cfg.Vault.Exclude,
	}
	if isHTTPURL(arg) {
		rep, err := importdocs.ImportURL(importContext(cmd), arg, vaultPath, opts, importdocs.HTTPFetcher())
		if err != nil {
			return nil, importErr(cmd, err)
		}
		return finishImport(cmd, vaultPath, arg, cfg, opts, rep)
	}
	if info, serr := os.Stat(arg); serr == nil && info.Mode().IsRegular() {
		return importOneFile(cmd, arg, vaultPath, cfg, opts)
	}
	src, err := importSource(arg)
	if err != nil {
		return nil, importErr(cmd, err)
	}
	rep, err := importdocs.Import(src, vaultPath, opts)
	if err != nil {
		return nil, importErr(cmd, err)
	}
	label := src.Repo
	if src.Prefix != "" {
		label += ":" + src.Prefix
	}
	return finishImport(cmd, vaultPath, label, cfg, opts, rep)
}

// importOneFile imports one .md or .pdf with the repository and prefix of
// its folder, as importing that folder would name it, and nothing else.
func importOneFile(cmd *cobra.Command, file, vaultPath string, cfg *vault.Config, opts importdocs.Options) (*importResult, error) {
	src, err := importSource(filepath.Dir(file))
	if err != nil {
		return nil, importErr(cmd, err)
	}
	name := filepath.Base(file)
	rep, err := importdocs.ImportFile(src, name, vaultPath, opts)
	if err != nil {
		return nil, importErr(cmd, err)
	}
	return finishImport(cmd, vaultPath, src.Repo+":"+path.Join(src.Prefix, name), cfg, opts, rep)
}

// isHTTPURL reports an argument the URL import handles. The check is
// case-sensitive: HTTP:// is not fetched.
func isHTTPURL(arg string) bool {
	return strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://")
}

// unsupportedImportURL refuses any scheme other than http and https before
// a folder import could treat the argument as a path.
func unsupportedImportURL(arg string) error {
	if isHTTPURL(arg) || !strings.Contains(arg, "://") {
		return nil
	}
	return errors.New("only http and https URLs can be imported")
}

func importContext(cmd *cobra.Command) context.Context {
	if cmd != nil {
		if ctx := cmd.Context(); ctx != nil {
			return ctx
		}
	}
	return context.Background()
}

// finishImport attaches the report to the vault and re-indexes what changed.
func finishImport(cmd *cobra.Command, vaultPath, label string, cfg *vault.Config, opts importdocs.Options, rep *importdocs.Result) (*importResult, error) {
	res := &importResult{Vault: vaultPath, Source: label, Result: rep, Counts: countActions(rep)}
	if changed := rep.Changed(); len(changed) > 0 && !opts.DryRun {
		dbPath := filepath.Join(vaultPath, cfg.Index.DBPath)
		ir, err := index.NewIndexer(vaultPath, dbPath, cfg).Incremental()
		if err != nil {
			res.IndexError = err.Error()
		} else {
			res.IndexWarnings = indexWarnings(ir, changed)
			embedAfterWriteUpTo(cmd, vaultPath, cfg, embedOnWriteLimit+len(changed))
			sayIfNeverEmbedded(cmd, vaultPath, dbPath)
		}
	}
	return res, nil
}

// sayIfNeverEmbedded names the command that embeds a vault that has no
// embeddings at all: until it runs, the notes just imported are found by
// keyword only. A write into such a vault is left alone (embedAfterWriteUpTo);
// an import is where a vault usually starts, so it says so once.
func sayIfNeverEmbedded(cmd *cobra.Command, vaultPath, dbPath string) {
	if model, err := index.EmbeddedModel(dbPath); err != nil || model != "" {
		return
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
		"this vault has never been embedded, so search is keyword-only; embed it with vaultmind index --embed --vault %s\n", vaultPath)
}

// indexWarnings picks out the index problems that concern the notes the
// import just wrote.
func indexWarnings(ir *index.IndexResult, changed []string) []string {
	mine := make(map[string]bool, len(changed))
	for _, c := range changed {
		mine[c] = true
	}
	var out []string
	for _, d := range ir.DuplicateDetails {
		if mine[d.Skipped] {
			out = append(out, fmt.Sprintf("%s not indexed: id %s is already held by %s", d.Skipped, d.ID, d.Kept))
		}
	}
	for _, e := range ir.ErrorDetails {
		if mine[e.Path] {
			out = append(out, fmt.Sprintf("%s not indexed (%s): %s", e.Path, e.Kind, e.Error))
		}
	}
	return out
}

// importSource names the docs by the repository around dir, as `paths:`
// does. Resolving a child of dir rather than dir itself makes dir the
// repository root when it is one.
func importSource(dir string) (importdocs.Source, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return importdocs.Source{}, err
	}
	if !info.IsDir() {
		return importdocs.Source{}, fmt.Errorf("%s is not a folder", dir)
	}
	f := navigate.ResolveCodeFile(filepath.Join(dir, "x"))
	if f.Repo == "" {
		abs, _ := filepath.Abs(dir)
		return importdocs.Source{Dir: dir, Repo: filepath.Base(abs)}, nil
	}
	prefix := path.Dir(f.Rel)
	if prefix == "." {
		prefix = ""
	}
	return importdocs.Source{Dir: dir, Repo: f.Repo, Prefix: prefix}, nil
}

// importErr reports err in the JSON envelope under --json.
func importErr(cmd *cobra.Command, err error) error {
	if !getConfigValueWithFlags[bool](cmd, "json", config.KeyAppImportJson) {
		return fmt.Errorf("import: %w", err)
	}
	code := "import_failed"
	if errors.Is(err, importdocs.ErrNoMarkdown) {
		code = "no_markdown"
	}
	_ = cmdutil.WriteJSONError(cmd.OutOrStdout(), importEnvelope, code, err.Error())
	return cmdutil.ErrAlreadyWritten
}

func countActions(r *importdocs.Result) map[importdocs.Action]int {
	counts := map[importdocs.Action]int{}
	for _, e := range r.Entries {
		counts[e.Action]++
	}
	return counts
}

func writeImport(cmd *cobra.Command, res *importResult) error {
	w := cmd.OutOrStdout()
	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppImportJson) {
		return json.NewEncoder(w).Encode(envelope.OK(importEnvelope, res))
	}
	return writeImportText(w, res)
}

// importOrder is the order actions are reported in: what needs attention
// last, where the eye lands.
var importOrder = []importdocs.Action{
	importdocs.Added, importdocs.Updated, importdocs.Unchanged, importdocs.Pruned,
	importdocs.Orphaned, importdocs.Conflict, importdocs.Skipped,
}

func writeImportText(w io.Writer, res *importResult) error {
	verb := "Imported"
	if res.DryRun {
		verb = "Would import (dry run, nothing written)"
	}
	summary := fmt.Sprintf("%s %s into %s:", verb, res.Source, res.Vault)
	for _, a := range importOrder {
		if n := res.Counts[a]; n > 0 {
			summary += fmt.Sprintf(" %d %s,", n, a)
		}
	}
	lines := []string{summary[:len(summary)-1]}
	many := res.Counts[importdocs.Added]+res.Counts[importdocs.Updated]+res.Counts[importdocs.Pruned] > importListLimit
	for _, e := range res.Entries {
		if !listed(e.Action, many) {
			continue
		}
		line := fmt.Sprintf("  %-9s %s", e.Action, e.Note)
		if e.Reason != "" {
			line += " — " + e.Reason
		}
		lines = append(lines, line)
	}
	for _, warn := range res.IndexWarnings {
		lines = append(lines, "  warning   "+warn)
	}
	if res.IndexError != "" {
		lines = append(lines, "written, but not indexed ("+res.IndexError+"); run vaultmind index --vault "+res.Vault)
	}
	for _, l := range lines {
		if _, err := fmt.Fprintln(w, l); err != nil {
			return err
		}
	}
	return nil
}

// listed reports whether the text report lists an entry: always when it needs
// attention, never when nothing happened, otherwise only for a short report.
func listed(a importdocs.Action, many bool) bool {
	switch a {
	case importdocs.Orphaned, importdocs.Conflict, importdocs.Skipped:
		return true
	case importdocs.Unchanged:
		return false
	}
	return !many
}

// importListLimit is how many changes the text report lists one by one.
// Above it (a first import of a big folder), only the entries that need
// attention are listed; --json has all.
const importListLimit = 20
