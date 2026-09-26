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
	"github.com/peiman/vaultmind/internal/vault"
)

//go:embed all:templates
var templates embed.FS

// knowledge is the knowledge-vault scaffold: a project knowledge base, not an
// agent's identity. It has no config of its own — the type registry comes from
// templates/, so both scaffolds share one.
//
//go:embed all:knowledge
var knowledge embed.FS

// Result is what Init returns to the caller — used by cmd/init.go to
// render the next-steps message after scaffolding succeeds.
type Result struct {
	VaultPath  string
	FilesAdded int
}

// WriteConfigOnly writes the type registry into an existing directory, creating
// .vaultmind/ if needed, and reports whether it wrote anything. It is a no-op
// when a config is already there.
//
// This is the path for "index a directory of markdown I already have". Init
// would also drop a README and four starter notes in, which then index AS notes
// — fine for a fresh vault, wrong for someone's existing notes folder. Without
// it, `index --vault <plain-dir>` created a .vaultmind/ holding only index.db:
// a directory that looked like a vault to the old marker check while carrying
// no registry, which is the same implicit-create the guessed-path guard exists
// to end.
func WriteConfigOnly(vaultPath string) (bool, error) {
	cfgPath := filepath.Join(vaultPath, filepath.FromSlash(vault.ConfigRelPath))
	if _, err := os.Stat(cfgPath); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat %s: %w", cfgPath, err)
	}
	body, err := templates.ReadFile("templates/" + vault.ConfigRelPath)
	if err != nil {
		return false, fmt.Errorf("read config template: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o750); err != nil {
		return false, fmt.Errorf("create %s: %w", filepath.Dir(cfgPath), err)
	}
	if err := os.WriteFile(cfgPath, body, 0o600); err != nil {
		return false, fmt.Errorf("write %s: %w", cfgPath, err)
	}
	return true, nil
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
	cleanPath, err := freshPath(vaultPath)
	if err != nil {
		return nil, err
	}
	count, err := writeTree(templates, "templates", cleanPath, today())
	if err != nil {
		return nil, err
	}
	return &Result{VaultPath: cleanPath, FilesAdded: count}, nil
}

// InitKnowledge scaffolds a project knowledge base at vaultPath: decisions/
// and concepts/ with an example each, a README on tying notes to code, and the
// shared type registry. Like Init, it refuses an existing path.
func InitKnowledge(vaultPath string) (*Result, error) {
	cleanPath, err := freshPath(vaultPath)
	if err != nil {
		return nil, err
	}
	count, err := writeTree(knowledge, "knowledge", cleanPath, today())
	if err != nil {
		return nil, err
	}
	wrote, err := WriteConfigOnly(cleanPath)
	if err != nil {
		return nil, err
	}
	if wrote {
		count++
	}
	return &Result{VaultPath: cleanPath, FilesAdded: count}, nil
}

// freshPath refuses a path that already exists: a vault is stateful, and
// silently rewriting someone's would be the worst kind of surprise.
func freshPath(vaultPath string) (string, error) {
	cleanPath := filepath.Clean(vaultPath)
	if _, err := os.Stat(cleanPath); err == nil {
		return "", fmt.Errorf("refuse to overwrite existing path: %s", cleanPath)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat %s: %w", cleanPath, err)
	}
	return cleanPath, nil
}

func today() string { return time.Now().UTC().Format(schema.CreatedDateFormat) }

// writeTree copies the embedded tree under root into dst, stamping each note's
// frontmatter with today's date, and returns the number of files written.
func writeTree(fsys embed.FS, root, dst, today string) (int, error) {
	count := 0
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		target := filepath.Join(dst, strings.TrimPrefix(p, root+"/"))
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		body, readErr := fsys.ReadFile(p)
		if readErr != nil {
			return fmt.Errorf("read template %s: %w", p, readErr)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return fmt.Errorf("create dir for %s: %w", target, err)
		}
		if err := os.WriteFile(target, renderTemplate(body, today), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		count++
		return nil
	})
	return count, err
}

// renderTemplate fills in date placeholders in the embedded templates.
// Frontmatter `created:` is inserted dynamically so every fresh vault
// starts with today's stamp — keeps the index honest instead of
// pinning everyone to the date the templates were authored.
//
// Files without leading frontmatter (README.md, .vaultmind/config.yaml)
// pass through unchanged.
func renderTemplate(body []byte, today string) []byte {
	const fmStart = "---\n"
	if len(body) < len(fmStart) || string(body[:len(fmStart)]) != fmStart {
		return body
	}
	dateLine := fmt.Sprintf("created: %s\n", today)
	return append(append([]byte(fmStart), []byte(dateLine)...), body[len(fmStart):]...)
}
