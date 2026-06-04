package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/peiman/vaultmind/internal/hooks"
)

// initWireParams carries the --wire-hooks flag set from runInit through the
// scaffold lane (init_scaffold.go) to the wiring step.
type initWireParams struct {
	wireHooks  bool
	local      bool
	dryRun     bool
	projectDir string
}

// wireInitHooks provisions VaultMind's Claude Code hooks for a freshly
// scaffolded vault: it installs the hook scripts into the project (CWD unless
// --project-dir overrides) and merges the wiring into the project's settings
// file, baking the new vault's ABSOLUTE path via VAULTMIND_VAULT so the hooks
// resolve it regardless of where Claude Code runs them. It reuses
// hooks.Provision — the same engine as `hooks install --merge` — so the
// install-then-wire sequence stays in one place (SSOT).
func wireInitHooks(w io.Writer, vaultPath string, p initWireParams) error {
	projectDir := p.projectDir
	if projectDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolving project dir: %w", err)
		}
		projectDir = cwd
	}
	absVault, err := filepath.Abs(vaultPath)
	if err != nil {
		return fmt.Errorf("resolving vault path: %w", err)
	}

	prov, err := hooks.Provision(hooks.InstallConfig{
		ProjectDir: projectDir,
		VaultPath:  absVault,
	}, true, p.local, p.dryRun)
	if err != nil {
		return fmt.Errorf("wiring hooks: %w", err)
	}

	if prov.Install != nil && len(prov.Install.Written) > 0 {
		_, _ = fmt.Fprintf(w, "✅ Installed %d hook script(s) into %s\n",
			len(prov.Install.Written), filepath.Join(projectDir, ".claude", "scripts"))
	}
	if prov.Merge != nil {
		writeMergeOutcome(w, prov.Merge)
		// The hooks query the vault on every prompt/session, but a freshly
		// scaffolded vault has no embeddings yet — so recall returns nothing
		// until index + embed run. Make that explicit so a user who wires and
		// immediately opens Claude Code isn't met with silent hooks.
		if !p.dryRun {
			_, _ = fmt.Fprintf(w, "\n⚠ Hooks are wired, but they surface nothing until the vault is indexed and\n")
			_, _ = fmt.Fprintf(w, "  embedded — run the index + embed steps below BEFORE starting Claude Code.\n")
		}
	}
	_, _ = fmt.Fprintln(w)
	return nil
}
