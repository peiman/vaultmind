// Package fix implements the `vaultmind frontmatter fix --backfill`
// command's logic — walking the vault and identifying domain notes that
// are missing vaultmind-owned frontmatter fields (created, vm_updated),
// then optionally writing the missing fields via the existing mutator.
//
// Per the four-tier frontmatter taxonomy in schema/registry.go,
// vaultmind-owned fields are vaultmind's responsibility. The mutator
// auto-maintains them on every operation; this command exists for
// existing-vault audits and migration scenarios where the auto-write
// contract didn't yet apply (notes pre-dating the auto-maintenance).
//
// Default mode is dry-run. Apply is opt-in. Per arc-extending-not-
// overwriting, vaultmind never silently rewrites user files; the
// human/agent must explicitly --apply to commit the changes.
//
// User-owned fields (title, status, tags, related_ids, etc.) are NEVER
// touched. The mutator's existing infrastructure (atomic writes,
// conflict detection, schema validation, vm_updated auto-bump) handles
// the actual write — this package is the discovery + iteration layer.
package fix

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/git"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/peiman/vaultmind/internal/parser"
	"github.com/peiman/vaultmind/internal/schema"
	"github.com/peiman/vaultmind/internal/vault"
)

// CreatedDateResolver computes a `created` date for a note path. Returns
// (value, source) where source describes provenance — "git", "mtime",
// or "today". Allows tests to inject deterministic values without
// touching the filesystem or git.
type CreatedDateResolver func(absPath string) (value string, source string)

// Config controls a fix run.
type Config struct {
	// VaultPath is the vault root.
	VaultPath string
	// Apply: when true, write changes via mutator. When false (default),
	// dry-run — items are populated with proposed values + diffs but no
	// file is written.
	Apply bool
	// CreatedResolver is optional; defaults to DefaultCreatedDateResolver
	// (git first-commit → file mtime → today). Tests inject deterministic
	// resolvers for stable assertions.
	CreatedResolver CreatedDateResolver
}

// Result is the JSON-serializable output of RunBackfill.
type Result struct {
	VaultPath     string `json:"vault_path"`
	FilesScanned  int    `json:"files_scanned"`
	NotesAffected int    `json:"notes_affected"`
	Items         []Item `json:"items"`
	Applied       bool   `json:"applied"`
}

// Item is one note that needs backfill.
type Item struct {
	Path           string            `json:"path"`
	ID             string            `json:"id"`
	MissingFields  []string          `json:"missing_fields"`
	ProposedValues map[string]string `json:"proposed_values"`
	Sources        map[string]string `json:"sources"`
	Diff           string            `json:"diff,omitempty"`
	Error          string            `json:"error,omitempty"`
}

// RunBackfill walks the vault, identifies domain notes missing
// vaultmind-owned fields, and (if cfg.Apply) writes the missing
// fields via the mutator. Always returns a Result describing what
// was found / what would change.
func RunBackfill(cfg Config) (*Result, error) {
	if cfg.CreatedResolver == nil {
		cfg.CreatedResolver = DefaultCreatedDateResolver
	}

	// Validate vault root exists. Per principle 1 (truth-seeking),
	// surface this clearly rather than letting downstream loaders fail
	// with surprising error chains.
	info, statErr := os.Stat(cfg.VaultPath)
	if statErr != nil || !info.IsDir() {
		return nil, fmt.Errorf("vault path %q does not exist or is not a directory", cfg.VaultPath)
	}

	// Load config and build registry inline. Per ADR-009, business
	// packages don't depend on each other (cmdutil is also business);
	// use the infrastructure layers (vault + schema) directly.
	vaultCfg, err := vault.LoadConfig(cfg.VaultPath)
	if err != nil {
		return nil, fmt.Errorf("loading vault config: %w", err)
	}
	reg := schema.NewRegistryWithAliases(vaultCfg.Types, vaultCfg.Schema.Aliases)
	checker, err := git.NewPolicyChecker(vaultCfg.Git)
	if err != nil {
		return nil, fmt.Errorf("building git policy checker: %w", err)
	}
	mutator := &mutation.Mutator{
		VaultPath: cfg.VaultPath,
		Detector:  &git.GoGitDetector{},
		Checker:   checker,
		Registry:  reg,
	}

	result := &Result{
		VaultPath: cfg.VaultPath,
		Items:     []Item{},
		Applied:   cfg.Apply,
	}

	walkErr := filepath.WalkDir(cfg.VaultPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != cfg.VaultPath && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		result.FilesScanned++

		item, found, processErr := processNote(cfg, reg, mutator, path)
		if processErr != nil {
			// Single-note errors don't abort the walk — capture and
			// continue, principle 3 (good will: surface partial failures
			// rather than silently dropping data). The single-note
			// failure is recorded in result.Items via Error field.
			result.Items = append(result.Items, Item{
				Path:  relPath(cfg.VaultPath, path),
				Error: processErr.Error(),
			})
			result.NotesAffected++
			return nil //nolint:nilerr // intentional: error captured in Item.Error; continue walking
		}
		if !found {
			return nil
		}
		result.Items = append(result.Items, item)
		result.NotesAffected++
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walking vault: %w", walkErr)
	}

	return result, nil
}

// processNote inspects one .md file. Returns (item, found, error).
// found=false means the note is either non-domain or already complete —
// nothing to fix.
func processNote(cfg Config, reg *schema.Registry, mutator *mutation.Mutator, path string) (Item, bool, error) {
	// path is produced by filepath.WalkDir rooted at vault — not user input.
	content, readErr := os.ReadFile(path) // #nosec G304
	if readErr != nil {
		return Item{}, false, fmt.Errorf("reading: %w", readErr)
	}
	fm, _, parseErr := parser.ExtractFrontmatter(content)
	if parseErr != nil {
		return Item{}, false, fmt.Errorf("parsing frontmatter: %w", parseErr)
	}

	isDomain, id, _ := parser.ClassifyNote(fm)
	if !isDomain {
		return Item{}, false, nil
	}

	// Determine which vaultmind-owned fields are missing. We use
	// IsFieldPresent to honor any per-vault aliases the user may have
	// configured (principle 7: SSOT — alias-resolution lives in
	// schema, not duplicated here).
	missing := []string{}
	if !reg.IsFieldPresent(fm, "created") {
		missing = append(missing, "created")
	}
	if !reg.IsFieldPresent(fm, "vm_updated") {
		missing = append(missing, "vm_updated")
	}
	if len(missing) == 0 {
		return Item{}, false, nil
	}

	// Compute proposed values and sources.
	proposed := map[string]string{}
	sources := map[string]string{}
	for _, field := range missing {
		switch field {
		case "created":
			val, src := cfg.CreatedResolver(path)
			proposed[field] = val
			sources[field] = src
		case "vm_updated":
			proposed[field] = time.Now().UTC().Format(schema.VMUpdatedFormat)
			sources[field] = "today"
		}
	}

	item := Item{
		Path:           relPath(cfg.VaultPath, path),
		ID:             id,
		MissingFields:  missing,
		ProposedValues: proposed,
		Sources:        sources,
	}

	// Build the merge fields. The mutator's auto-bump on OpMerge will
	// also update vm_updated to today — that's exactly what we want
	// for "vm_updated missing" cases. For "vm_updated present but stale"
	// the mutator's auto-bump still applies; per the vaultmind-owned
	// contract, vm_updated represents "when vaultmind last touched
	// this," and this fix run IS that touch.
	fields := map[string]interface{}{}
	for k, v := range proposed {
		fields[k] = v
	}

	// Always run the mutator — even in dry-run — to populate the diff
	// preview. The mutator's DryRun flag controls whether anything is
	// written to disk.
	req := mutation.MutationRequest{
		Op:     mutation.OpMerge,
		Target: relPath(cfg.VaultPath, path),
		Fields: fields,
		DryRun: !cfg.Apply,
		Diff:   true,
	}
	mr, mErr := mutator.Run(req)
	if mErr != nil {
		item.Error = mErr.Error()
		return item, true, nil //nolint:nilerr // intentional: per-note mutator error captured in Item.Error so the run continues
	}
	item.Diff = mr.Diff
	return item, true, nil
}

// relPath returns path relative to vaultPath, falling back to path
// unchanged on error.
func relPath(vaultPath, path string) string {
	rel, err := filepath.Rel(vaultPath, path)
	if err != nil {
		return path
	}
	return rel
}

// DefaultCreatedDateResolver tries git first-commit, falls back to file
// mtime, falls back to today. Returns (date-string, source) where source
// is "git", "mtime", or "today". Date is YYYY-MM-DD (date-only) — created
// is a humanish "when this was born" stamp, not a sub-day-precision
// processing tracker (that's vm_updated's job).
func DefaultCreatedDateResolver(absPath string) (string, string) {
	// 1. Try git log first-commit. Use --diff-filter=A --follow to get
	//    the actual creation commit (handles renames). Format %as gives
	//    YYYY-MM-DD (author short-date).
	if val, ok := gitFirstCommitDate(absPath); ok {
		return val, "git"
	}
	// 2. File mtime fallback.
	if info, err := os.Stat(absPath); err == nil {
		return info.ModTime().UTC().Format(schema.CreatedDateFormat), "mtime"
	}
	// 3. Today's date as final fallback.
	return time.Now().UTC().Format(schema.CreatedDateFormat), "today"
}

// gitFirstCommitDate runs `git log --diff-filter=A --follow --format=%as`
// to find the creation date of the file. Returns ok=false if git is
// unavailable, the file isn't tracked, or the command fails.
func gitFirstCommitDate(absPath string) (string, bool) {
	dir := filepath.Dir(absPath)
	// Bail early if there's no .git anywhere up the tree.
	if !hasGitRoot(dir) {
		return "", false
	}
	// dir and filepath.Base(absPath) are derived from filepath.WalkDir
	// output (vault paths under the user's vault root), never from raw
	// user input. The git binary itself is the only "subprocess" — args
	// are vault-derived path components.
	cmd := exec.Command("git", "-C", dir, "log", "--diff-filter=A", "--follow", "--format=%as", "--", filepath.Base(absPath)) //nolint:gosec // G204: args are vault paths from WalkDir, not arbitrary user input
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	// Git may return multiple lines (rare with --diff-filter=A); take
	// the LAST line (oldest commit if there are multiple A entries
	// across renames).
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "" {
		return "", false
	}
	return last, true
}

// hasGitRoot walks up looking for a .git directory.
func hasGitRoot(dir string) bool {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}
