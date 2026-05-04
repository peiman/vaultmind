// Package initvault scaffolds a fresh VaultMind vault from embedded
// templates. The templates are persona-shaped — identity, principles,
// arcs, references, concepts — because that's what VaultMind is for:
// long-term memory of an AI agent collaboratively curated by the agent
// and a human partner.
package initvault

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/peiman/vaultmind/internal/schema"
)

//go:embed all:templates
var templates embed.FS

// Result is what Init returns to the caller — used by cmd/init.go to
// render the next-steps message after scaffolding succeeds.
type Result struct {
	VaultPath  string
	FilesAdded int
}

// Init scaffolds a fresh vault at vaultPath. The directory must not
// already exist — Init refuses to overwrite, because a vault is
// stateful (notes, embeddings, git history) and silently rewriting
// someone's existing vault would be the worst kind of destructive
// surprise.
//
// Each templated note has its frontmatter dates filled in with today's
// date so a fresh vault indexes cleanly without manual editing.
func Init(vaultPath string) (*Result, error) {
	cleanPath := filepath.Clean(vaultPath)
	if _, err := os.Stat(cleanPath); err == nil {
		return nil, fmt.Errorf("refuse to overwrite existing path: %s", cleanPath)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat %s: %w", cleanPath, err)
	}

	now := time.Now().UTC()
	// Both formats use the canonical SSOT constants from schema package
	// (per principle 7). vm_updated is RFC3339 second-precision UTC so
	// doctor's drift detector can parse it consistently. created stays
	// date-only — it's a humanish "when was this born" stamp, not a
	// vaultmind-processed-on tracker.
	today := now.Format(schema.CreatedDateFormat)
	vmUpdated := now.Format(schema.VMUpdatedFormat)
	count := 0

	walkErr := fs.WalkDir(templates, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "templates" {
			return nil
		}
		// Strip the "templates/" prefix to get the in-vault relative path.
		rel := strings.TrimPrefix(p, "templates/")
		dst := filepath.Join(cleanPath, rel)

		if d.IsDir() {
			return os.MkdirAll(dst, 0o750)
		}

		body, readErr := templates.ReadFile(p)
		if readErr != nil {
			return fmt.Errorf("read template %s: %w", p, readErr)
		}
		body = renderTemplate(body, today, vmUpdated)

		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return fmt.Errorf("create dir for %s: %w", dst, err)
		}
		if err := os.WriteFile(dst, body, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
		count++
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	return &Result{VaultPath: cleanPath, FilesAdded: count}, nil
}

// renderTemplate fills in date placeholders in the embedded templates.
// Frontmatter `created:` (date-only) and `vm_updated:` (RFC3339
// datetime per schema.VMUpdatedFormat) are inserted dynamically so
// every fresh vault starts with today's stamps — keeps the index
// honest instead of pinning everyone to the date the templates were
// authored.
//
// vm_updated takes the RFC3339 form (with colons) — YAML auto-quotes
// the resulting value, which is correct YAML. The format matches the
// mutator's auto-bump path so doctor's `mtime > vm_updated` parser
// can read both write sites uniformly.
func renderTemplate(body []byte, today, vmUpdated string) []byte {
	// Templates intentionally OMIT created/vm_updated fields. We inject
	// them after the leading frontmatter `---` so dates are always fresh.
	// Files without leading frontmatter (README.md, .vaultmind/config.yaml)
	// pass through unchanged.
	const fmStart = "---\n"
	if len(body) < len(fmStart) || string(body[:len(fmStart)]) != fmStart {
		return body
	}
	// vm_updated is YAML-quoted because RFC3339 contains colons that
	// YAML would otherwise parse as a mapping. Quoting here makes the
	// intent explicit and matches what SetKey would emit.
	dateLines := fmt.Sprintf("created: %s\nvm_updated: %q\n", today, vmUpdated)
	return append(append([]byte(fmStart), []byte(dateLines)...), body[len(fmStart):]...)
}
