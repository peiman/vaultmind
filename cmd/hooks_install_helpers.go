package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/spf13/cobra"
)

// hooksInstallParams bundles the resolved flags for a hooks-install run so the
// thin command wiring (cmd/hooks_install.go) stays under the ≤30-line cap
// (ADR-001) and the core has a single, named argument.
type hooksInstallParams struct {
	projectDir string
	force      bool
	jsonOut    bool
	only       string
	vault      string
	merge      bool
	local      bool
	dryRun     bool
}

// hooksInstallPayload is the JSON shape for an install run. InstallResult is
// embedded so its fields stay at the top level (backward-compatible with the
// pre-merge JSON); Merge is added only when --merge ran (omitempty), so a
// plain install emits exactly what it always did.
type hooksInstallPayload struct {
	*hooks.InstallResult
	Merge *hooks.MergeFileResult `json:"merge,omitempty"`
}

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

// runHooksInstallCore is the core of `vaultmind hooks install`. It writes the
// embedded scripts (hooks.Install), then — when --merge is set and the scripts
// installed cleanly — additively wires them into the project's settings file
// (hooks.MergeIntoSettings). Conflict-without-force surfaces the per-script
// remediation path explicitly; an empty `only` falls through to "install all".
func runHooksInstallCore(cmd *cobra.Command, p hooksInstallParams) error {
	// --dry-run only previews the settings merge; without --merge it would
	// silently write the scripts, the opposite of what "dry run" implies.
	if p.dryRun && !p.merge {
		return fmt.Errorf("--dry-run requires --merge (it previews the settings merge)")
	}

	var onlyList []string
	if strings.TrimSpace(p.only) != "" {
		for _, name := range strings.Split(p.only, ",") {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				onlyList = append(onlyList, trimmed)
			}
		}
	}
	// Install scripts and (with --merge) wire settings via the shared
	// provisioning engine — the same path init --wire-hooks uses. A script
	// conflict gates the merge inside Provision (never wire unresolved scripts).
	prov, retErr := hooks.Provision(hooks.InstallConfig{
		ProjectDir: p.projectDir,
		Force:      p.force,
		Only:       onlyList,
		VaultPath:  strings.TrimSpace(p.vault),
	}, p.merge, p.local, p.dryRun)
	res, mergeRes := prov.Install, prov.Merge

	w := cmd.OutOrStdout()
	if p.jsonOut {
		var payload interface{} = res
		if mergeRes != nil {
			payload = &hooksInstallPayload{InstallResult: res, Merge: mergeRes}
		}
		env := envelope.OK("hooks install", payload)
		if retErr != nil {
			env.Status = "error"
			env.Errors = append(env.Errors, envelope.Issue{Code: hooksInstallErrorCode(res), Message: retErr.Error()})
		}
		_ = json.NewEncoder(w).Encode(env)
		return retErr
	}

	writeHooksInstallHuman(w, res, mergeRes)
	return retErr
}

// hooksInstallErrorCode preserves the historical "hooks_install_conflict" code
// for script conflicts (backward compat for JSON consumers) and distinguishes a
// settings-merge failure, which can only occur after a clean install.
func hooksInstallErrorCode(res *hooks.InstallResult) string {
	if res != nil && len(res.Conflicts) > 0 {
		return "hooks_install_conflict"
	}
	return "hooks_install_merge_error"
}

// writeHooksInstallHuman renders the human-readable install summary, including
// the optional settings-merge outcome. When a merge ran, the copy-paste stanza
// is suppressed (the wiring is already done or previewed).
func writeHooksInstallHuman(w io.Writer, res *hooks.InstallResult, mergeRes *hooks.MergeFileResult) {
	if res == nil {
		return
	}
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

	if mergeRes != nil {
		writeMergeOutcome(w, mergeRes)
		return // merge handles the wiring messaging; the paste stanza is redundant
	}
	if res.SettingsStanza != "" {
		_, _ = fmt.Fprintf(w, "\nWire these into .claude/settings.json (merge under an existing \"hooks\" key if present), or re-run with --merge to apply automatically:\n\n%s\n", res.SettingsStanza)
	}
}

// writeMergeOutcome renders the settings-merge result for human output.
func writeMergeOutcome(w io.Writer, mergeRes *hooks.MergeFileResult) {
	switch {
	case mergeRes.DryRun:
		_, _ = fmt.Fprintf(w, "\nDry run — would merge into %s (nothing written):\n\n%s\n", mergeRes.SettingsPath, mergeRes.Merged)
	case mergeRes.Changed:
		_, _ = fmt.Fprintf(w, "\n✓ Merged VaultMind hooks into %s — existing hooks preserved.\n", mergeRes.SettingsPath)
	default:
		_, _ = fmt.Fprintf(w, "\n· %s already wired — no changes.\n", mergeRes.SettingsPath)
	}
}
