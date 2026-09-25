package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	profile    string
	vault      string
	merge      bool
	local      bool
	dryRun     bool
	agent      string
	vaults     string
}

// Agents `hooks install` can wire.
const (
	hooksAgentClaude = "claude"
	hooksAgentCodex  = "codex"

	codexTrustNotice = "\n⚠ Codex runs these hooks ONLY after two approvals, and skips them silently until then:\n" +
		"  1. trust this project when Codex asks, and\n" +
		"  2. run /hooks inside Codex and trust the VaultMind hooks.\n" +
		"  Until both are done, Codex starts with no memory and does not say so.\n" +
		"  Approve again after every upgrade: a new or changed hook is skipped until you do.\n" +
		"  Check it any time: vaultmind hooks status <project-dir>\n" +
		"  Not wired for Codex yet: read-tracking, the pre-compaction prompt.\n"
)

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
	// An unrecognised profile is rejected BEFORE anything is written: silently
	// treating a typo as "full" would install a persona loader into a vault
	// that has no persona and then call it healthy.
	profile, perr := hooks.ParseProfile(strings.TrimSpace(p.profile))
	if perr != nil {
		return perr
	}
	// No --profile: keep what the project already declared. Defaulting to
	// "full" re-declared a knowledge vault as full and wired a persona into it
	// on the very upgrade command the release notes gave.
	if strings.TrimSpace(p.profile) == "" {
		if profile, perr = hooks.DeclaredProfile(p.projectDir); perr != nil {
			return perr
		}
	}

	vaults, verr := parseHookVaults(p.vaults)
	if verr != nil {
		return verr
	}
	// Pinned absolute for the same reason as --vaults: hooks run from the
	// project, so a relative vault resolves against the wrong directory.
	if p.vault, verr = resolveHookVault(p.vault); verr != nil {
		return verr
	}
	agent := strings.TrimSpace(p.agent)
	if agent == "" {
		agent = hooksAgentClaude
	}
	if agent != hooksAgentClaude && agent != hooksAgentCodex {
		return fmt.Errorf("--agent %q: must be %q or %q", agent, hooksAgentClaude, hooksAgentCodex)
	}
	if agent == hooksAgentCodex {
		if p.local {
			return fmt.Errorf("--local is a Claude Code settings option; Codex reads .codex/hooks.json")
		}
		return runHooksInstallCodex(cmd, p, onlyList, profile, vaults)
	}

	prov, retErr := hooks.Provision(hooks.InstallConfig{
		ProjectDir: p.projectDir,
		Force:      p.force,
		Only:       onlyList,
		VaultPath:  strings.TrimSpace(p.vault),
		Vaults:     vaults,
		Profile:    profile,
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

	writeHooksInstallHuman(w, res, mergeRes, installGuidance{
		rerun:        hooksInstallRerun(p, hooksAgentClaude, vaults),
		missingVault: missingVaultWarning(p.projectDir, strings.TrimSpace(p.vault), vaults),
	})
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
func writeHooksInstallHuman(w io.Writer, res *hooks.InstallResult, mergeRes *hooks.MergeFileResult, g installGuidance) {
	if res == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "Project: %s\n", res.ProjectDir)
	_, _ = fmt.Fprintf(w, "Scripts dir: %s\n", res.ScriptsDir)
	if len(res.Written) > 0 {
		writtenVerb := "Written"
		if res.DryRun {
			writtenVerb = "Would write"
		}
		_, _ = fmt.Fprintf(w, "\n%s (%d):\n", writtenVerb, len(res.Written))
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

	if g.missingVault != "" {
		_, _ = fmt.Fprintf(w, "\n%s\n", g.missingVault)
	}
	if mergeRes != nil {
		writeMergeOutcome(w, mergeRes)
		return // merge handles the wiring messaging; the paste stanza is redundant
	}
	if res.SettingsStanza != "" {
		// Lead with the one command that does this correctly. The stanza below
		// is ~60 lines of JSON, so anything said after it has already scrolled
		// past — which left hand-merging as the de-facto default even though
		// --merge is additive, idempotent, and keeps a project's own hooks.
		// The preview flag goes in the same breath: the reason people paste by
		// hand is not wanting a tool to edit settings.json unseen.
		_, _ = fmt.Fprintf(w, "\nNext: wire these into .claude/settings.json.\n")
		_, _ = fmt.Fprintf(w, "  %s --merge --dry-run   preview the merged file, write nothing\n", g.rerun)
		_, _ = fmt.Fprintf(w, "  %s --merge             apply it (additive — existing hooks preserved)\n", g.rerun)
		_, _ = fmt.Fprintf(w, "\nOr paste this yourself, merging under an existing \"hooks\" key if present:\n\n%s\n", res.SettingsStanza)
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
	// Removal is the one thing a merge does that a person might not expect:
	// name every hook taken out.
	if len(mergeRes.Removed) > 0 {
		verb := "Removed"
		if mergeRes.DryRun {
			verb = "Would remove"
		}
		_, _ = fmt.Fprintf(w, "%s VaultMind hooks outside this profile: %s\n", verb, strings.Join(mergeRes.Removed, ", "))
	}
}

// runHooksInstallCodex installs the same scripts, then wires Codex instead of
// Claude Code. Scripts first and only then the wiring, with a conflict gating
// the merge — the same order Provision enforces for Claude Code.
func runHooksInstallCodex(cmd *cobra.Command, p hooksInstallParams, only []string, profile hooks.Profile, vaults []string) error {
	projectDir, err := filepath.Abs(p.projectDir)
	if err != nil {
		return fmt.Errorf("resolving project dir: %w", err)
	}
	vault := strings.TrimSpace(p.vault)
	prov, retErr := hooks.Provision(hooks.InstallConfig{
		ProjectDir: projectDir,
		Force:      p.force,
		Only:       only,
		VaultPath:  vault,
		Vaults:     vaults,
		Profile:    profile,
		Agent:      hooks.AgentCodex,
	}, false, false, p.dryRun)
	res := prov.Install

	var mergeRes *hooks.MergeFileResult
	if retErr == nil && res != nil {
		if p.merge {
			mergeRes, retErr = hooks.MergeIntoCodexHooksFor(projectDir, vault, vaults, profile, p.dryRun)
		} else if stanza, serr := hooks.CodexHooksStanzaFor(projectDir, vault, vaults, profile); serr == nil {
			res.SettingsStanza = stanza
		}
	}

	w := cmd.OutOrStdout()
	if p.jsonOut {
		payload := &hooksInstallPayload{InstallResult: res, Merge: mergeRes}
		env := envelope.OK("hooks install", payload)
		if retErr != nil {
			env.Status = "error"
			env.Errors = append(env.Errors, envelope.Issue{Code: hooksInstallErrorCode(res), Message: retErr.Error()})
		}
		_ = json.NewEncoder(w).Encode(env)
		return retErr
	}
	writeHooksInstallCodexHuman(w, res, mergeRes, installGuidance{
		rerun:        hooksInstallRerun(p, hooksAgentCodex, vaults),
		missingVault: missingVaultWarning(projectDir, vault, vaults),
	})
	return retErr
}

// writeHooksInstallCodexHuman is the Codex variant of the human summary: the
// wiring target is .codex/hooks.json, and the silent-skip trust rule is said
// every time, because it is the one failure the operator cannot see.
func writeHooksInstallCodexHuman(w io.Writer, res *hooks.InstallResult, mergeRes *hooks.MergeFileResult, g installGuidance) {
	if res == nil {
		return
	}
	stanza := res.SettingsStanza
	res.SettingsStanza = "" // the Claude Code paste instructions do not apply
	writeHooksInstallHuman(w, res, mergeRes, g)
	if mergeRes == nil && stanza != "" {
		_, _ = fmt.Fprintf(w, "\nNext: wire these into .codex/hooks.json.\n")
		_, _ = fmt.Fprintf(w, "  %s --merge --dry-run   preview, write nothing\n", g.rerun)
		_, _ = fmt.Fprintf(w, "  %s --merge             apply it (additive)\n", g.rerun)
		_, _ = fmt.Fprintf(w, "\nOr paste this yourself:\n\n%s\n", stanza)
	}
	_, _ = io.WriteString(w, codexTrustNotice)
}

// parseHookVaults turns --vaults into absolute paths. Hooks run from wherever
// the agent is, so a relative vault would resolve against the wrong directory;
// a list of one is not a federation and is rejected rather than silently
// accepted as one.
func parseHookVaults(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var out []string
	for _, v := range strings.Split(raw, ",") {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		abs, err := filepath.Abs(v)
		if err != nil {
			return nil, fmt.Errorf("--vaults: resolving %q: %w", v, err)
		}
		out = append(out, abs)
	}
	if len(out) < 2 {
		return nil, fmt.Errorf("--vaults needs at least two vaults to search together; for one, use --vault")
	}
	return out, nil
}

// installGuidance is what the human summary needs beyond the install result:
// the command to suggest, and a warning about the vault the hooks will use.
type installGuidance struct {
	rerun        string
	missingVault string
}

// hooksInstallRerun rebuilds the command the user ran, so a suggested next step
// is that command plus one flag. The bare `vaultmind hooks install --merge` it
// used to print dropped the project dir, and run from where the user stood it
// wired a different directory (stranger test, 2026-09-23).
func hooksInstallRerun(p hooksInstallParams, agent string, vaults []string) string {
	parts := []string{"vaultmind hooks install"}
	if d := strings.TrimSpace(p.projectDir); d != "" && d != "." {
		parts = append(parts, shellWord(d))
	}
	if agent == hooksAgentCodex {
		parts = append(parts, "--agent codex")
	}
	if v := strings.TrimSpace(p.vault); v != "" {
		parts = append(parts, "--vault "+shellWord(v))
	}
	if len(vaults) > 0 {
		parts = append(parts, "--vaults "+shellWord(strings.Join(vaults, ",")))
	}
	if pr := strings.TrimSpace(p.profile); pr != "" {
		parts = append(parts, "--profile "+shellWord(pr))
	}
	return strings.Join(parts, " ")
}

// shellWord quotes s only when a shell would otherwise split or expand it.
func shellWord(s string) string {
	for _, r := range s {
		if !shellSafe(r) {
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
}

// shellSafe reports whether r needs no quoting in a POSIX shell word.
func shellSafe(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune("/._-,:=~", r)
}

// missingVaultWarning says so when no vault was named and the default the
// hooks fall back to (<project>/vaultmind-identity) does not exist. Following
// the README, nothing asked for a vault: the hooks were wired to a folder that
// was never there, and loaded nothing without a word.
func missingVaultWarning(projectDir, vault string, vaults []string) string {
	if vault != "" {
		if _, err := os.Stat(vault); err != nil {
			return fmt.Sprintf("⚠ --vault %s does not exist, so these hooks will load nothing until it does.", vault)
		}
		return ""
	}
	if len(vaults) > 0 {
		return ""
	}
	def := filepath.Join(projectDir, "vaultmind-identity")
	if _, err := os.Stat(def); err == nil {
		return ""
	}
	return fmt.Sprintf("⚠ No --vault given, so these hooks will read %s — which does not exist.\n"+
		"  Until it does they load nothing. Point them at your vault: add --vault <path-to-your-vault>.", def)
}

// resolveHookVault pins --vault to an absolute path. Hooks run from the
// project directory, so `--vault ./my-vault` typed from elsewhere used to
// resolve to <project>/my-vault and load nothing (stranger test, 2026-09-23).
func resolveHookVault(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}
	abs, err := filepath.Abs(v)
	if err != nil {
		return "", fmt.Errorf("--vault: resolving %q: %w", v, err)
	}
	return abs, nil
}
