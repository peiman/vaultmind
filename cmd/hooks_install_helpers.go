package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/spf13/cobra"
)

// resolveProjectDir picks the project directory: positional arg if
// given, else CWD. Split out so cmd/hooks_install.go's wiring stays
// under the ≤30-line cap (ADR-001).
func resolveProjectDir(args []string) string {
	if len(args) == 1 && args[0] != "" {
		return args[0]
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// runHooksInstallCore is the core of `vaultmind hooks install`.
// Calls hooks.Install + emits JSON envelope or human-readable
// summary. Conflict-without-force surfaces the per-script
// remediation path explicitly. The `only` arg is a comma-separated
// list of canonical script names; empty string falls through to
// "install all" per workhorse 2026-05-07 MEDIUM.
func runHooksInstallCore(cmd *cobra.Command, projectDir string, force, jsonOut bool, only string) error {
	var onlyList []string
	if strings.TrimSpace(only) != "" {
		for _, name := range strings.Split(only, ",") {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				onlyList = append(onlyList, trimmed)
			}
		}
	}
	res, err := hooks.Install(hooks.InstallConfig{
		ProjectDir: projectDir,
		Force:      force,
		Only:       onlyList,
	})
	// res is non-nil on conflict-without-force; emit it before
	// returning the conflict error so the caller sees both written
	// AND conflicts.

	w := cmd.OutOrStdout()
	if jsonOut {
		env := envelope.OK("hooks install", res)
		if err != nil {
			env.Status = "error"
			env.Errors = append(env.Errors, envelope.Issue{
				Code:    "hooks_install_conflict",
				Message: err.Error(),
			})
		}
		_ = json.NewEncoder(w).Encode(env)
		return err
	}

	if res != nil {
		_, _ = fmt.Fprintf(w, "Project: %s\n", res.ProjectDir)
		_, _ = fmt.Fprintf(w, "Scripts dir: %s\n", res.ScriptsDir)
		if len(res.Written) > 0 {
			_, _ = fmt.Fprintf(w, "\nWritten (%d):\n", len(res.Written))
			for _, name := range res.Written {
				_, _ = fmt.Fprintf(w, "  ✓ %s\n", name)
			}
		}
		if len(res.Skipped) > 0 {
			_, _ = fmt.Fprintf(w, "\nSkipped — already byte-identical (%d):\n", len(res.Skipped))
			for _, name := range res.Skipped {
				_, _ = fmt.Fprintf(w, "  · %s\n", name)
			}
		}
		if len(res.Conflicts) > 0 {
			_, _ = fmt.Fprintf(w, "\n⚠ Conflicts (%d) — exists with different content:\n", len(res.Conflicts))
			for _, name := range res.Conflicts {
				_, _ = fmt.Fprintf(w, "  ✗ %s\n", name)
			}
			_, _ = fmt.Fprintf(w, "\nRe-run with --force to overwrite, or edit the conflicting files manually.\n")
		}
	}
	return err
}
