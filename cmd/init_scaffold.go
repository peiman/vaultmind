package cmd

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/peiman/vaultmind/internal/initvault"
	"github.com/peiman/vaultmind/internal/telemetry"
	"github.com/spf13/cobra"
)

// runInitScaffold runs the scaffold-a-vault flow. Split out from
// init.go's wiring so the wiring stays under the ≤30-line cap (ADR-001).
// The flag-handling lane (--print-instructions) lives in init.go;
// this file is the create-files lane. When p.wireHooks is set, the
// Claude Code hooks are provisioned after the scaffold (init_wire.go).
func runInitScaffold(cmd *cobra.Command, path string, p initWireParams) error {
	// Validated before anything is written: a typo must not leave a half-made
	// vault behind.
	profile, err := hooks.ParseProfile(p.profile)
	if err != nil {
		return err
	}
	knowledge := profile == hooks.ProfileKnowledge
	scaffold := initvault.Init
	if knowledge {
		scaffold = initvault.InitKnowledge
	}
	res, err := scaffold(path)
	if err != nil {
		return err
	}
	if _, err := telemetry.EnsureFingerprint(res.VaultPath); err != nil {
		return fmt.Errorf("generate fingerprint: %w", err)
	}
	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(w, "✅ Vault scaffolded at %s (%d files)\n\n", res.VaultPath, res.FilesAdded)

	if p.wireHooks {
		if err := wireInitHooks(w, res.VaultPath, p); err != nil {
			return err
		}
	}

	writeInitNextSteps(w, res.VaultPath, knowledge)
	_, _ = fmt.Fprintf(w, "If this vault lives in a git repo, add to .gitignore (the index is a\nregenerable cache; the type registry is source):\n")
	_, _ = fmt.Fprintf(w, "  .vaultmind/index.db*\n")
	_, _ = fmt.Fprintf(w, "  !.vaultmind/config.yaml\n\n")
	if !p.wireHooks {
		_, _ = fmt.Fprintf(w, "To wire Claude Code now: re-run with --wire-hooks.\n")
		if abs, err := filepath.Abs(res.VaultPath); err == nil {
			_, _ = fmt.Fprintf(w, "To wire Codex: vaultmind hooks install <project-dir> --vault %s --agent codex --merge\n", shellWord(abs))
		}
		_, _ = fmt.Fprintf(w, "Or ")
		_, _ = fmt.Fprintf(w, "for agent-led setup (interview, project read, migration): vaultmind init --print-instructions\n")
	}
	return nil
}

// writeInitNextSteps says what to do with the vault just made — the first
// question and the files to replace differ between a knowledge base and an
// agent's identity.
func writeInitNextSteps(w io.Writer, vaultPath string, knowledge bool) {
	question, edit := "who am I", "Edit identity/who-am-i.md and references/current-context.md to make it yours."
	if knowledge {
		question = "<a question about this project>"
		edit = "Replace the examples in decisions/ and concepts/ with real notes, and tie each\nto the code it is about with paths: — README.md shows how."
	}
	_, _ = fmt.Fprintf(w, "Next steps:\n")
	_, _ = fmt.Fprintf(w, "  cd %s\n", vaultPath)
	_, _ = fmt.Fprintf(w, "  vaultmind index --vault .\n")
	_, _ = fmt.Fprintf(w, "  vaultmind index --embed --vault .\n")
	_, _ = fmt.Fprintf(w, "  vaultmind ask %q --vault .\n\n", question)
	_, _ = fmt.Fprintf(w, "%s\n\n", edit)
}
