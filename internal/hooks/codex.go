package hooks

import (
	"fmt"
	"os"
	"path/filepath"
)

// Codex support — the same scripts, wired for Codex CLI's hook system.
//
// Codex (verified on 0.156, 2026-09-22) fires the same events with the same
// stdin shape as Claude Code and reads `.codex/hooks.json` in the same
// {"hooks": {Event: [groups]}} form. Three differences decide this file:
//
//   - No CLAUDE_PROJECT_DIR. Every script path resolved to /.claude/scripts/…
//     and every hook "Failed". The project dir is baked in and EXPORTED first:
//     as a prefix assignment it would expand after the "$CLAUDE_PROJECT_DIR"
//     beside it — the exact bug hit while proving this.
//   - Context over ~2500 tokens is moved to a file by default. The identity
//     load is ~18KB, so the limit is set to 0 (never).
//   - Hooks run only once trusted (`/hooks`), and an untrusted hook is skipped
//     SILENTLY. That is the caller's to say out loud; nothing here can fix it.

const (
	codexDir       = ".codex"
	codexHooksFile = "hooks.json"
)

// codexScripts are the hooks that work under Codex. Deliberately absent:
//   - capture-episode.sh: it looks for the transcript under Claude Code's
//     layout, misses the Codex session, and falls back to the newest Claude
//     transcript — recording the WRONG session as this one, silently.
//   - vault-track-read.sh: matches Claude Code's Read tool; Codex has none.
//   - precompact-preserve.sh: Codex's PreCompact cannot emit context.
var codexScripts = map[string]bool{
	hookSessionStartScript:     true,
	hookHealthScript:           true,
	hookUserPromptSubmitScript: true,
	hookReachScript:            true,
}

// codexHooks derives Codex's wiring from the canonical hooks (one source of
// truth for events, matchers and scripts), filtered to what works under Codex
// and to the profile.
func codexHooks(projectDir, vaultPath string, vaults []string, p Profile) []canonicalHook {
	allowed := map[string]bool{}
	for _, n := range ScriptsForProfile(p) {
		allowed[n] = true
	}
	noLimit := 0
	prefix := "export CLAUDE_PROJECT_DIR=" + singleQuote(projectDir) + "; "
	var out []canonicalHook
	for _, ch := range canonicalHooksFor(vaultPath, vaults) {
		if !codexScripts[ch.Script] || !allowed[ch.Script] {
			continue
		}
		g := hookGroup{Matcher: ch.Group.Matcher}
		for _, h := range ch.Group.Hooks {
			h.Command = prefix + h.Command
			h.AdditionalContextLimit = &noLimit
			g.Hooks = append(g.Hooks, h)
		}
		ch.Group = g
		out = append(out, ch)
	}
	return out
}

// CodexHooksStanza renders .codex/hooks.json for projectDir.
func CodexHooksStanza(projectDir, vaultPath string, p Profile) (string, error) {
	return CodexHooksStanzaFor(projectDir, vaultPath, nil, p)
}

// CodexHooksStanzaFor is CodexHooksStanza with an optional federation.
func CodexHooksStanzaFor(projectDir, vaultPath string, vaults []string, p Profile) (string, error) {
	return renderStanza(codexHooks(projectDir, vaultPath, vaults, p))
}

// CodexHooksPath is where the Codex wiring for projectDir lives.
func CodexHooksPath(projectDir string) string {
	return filepath.Join(projectDir, codexDir, codexHooksFile)
}

// MergeIntoCodexHooks additively merges VaultMind's Codex wiring into
// <projectDir>/.codex/hooks.json — the same rules as the Claude Code merge:
// existing hooks preserved, re-runs change nothing, malformed files refused.
func MergeIntoCodexHooks(projectDir, vaultPath string, p Profile, dryRun bool) (*MergeFileResult, error) {
	return MergeIntoCodexHooksFor(projectDir, vaultPath, nil, p, dryRun)
}

// MergeIntoCodexHooksFor is MergeIntoCodexHooks with an optional federation.
func MergeIntoCodexHooksFor(projectDir, vaultPath string, vaults []string, p Profile, dryRun bool) (*MergeFileResult, error) {
	path := CodexHooksPath(projectDir)
	existing, err := readSettingsFile(path)
	if err != nil {
		return nil, err
	}
	merged, changed, err := mergeHooks(existing, codexHooks(projectDir, vaultPath, vaults, p))
	if err != nil {
		return nil, fmt.Errorf("merging into %s: %w", path, err)
	}
	res := &MergeFileResult{SettingsPath: path, Changed: changed, DryRun: dryRun, Merged: string(merged)}
	if dryRun || !changed {
		return res, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	if err := atomicWriteFile(path, merged, 0o600); err != nil {
		return nil, fmt.Errorf("writing %s: %w", path, err)
	}
	return res, nil
}
