package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Scope selects a project's hooks or the user's hooks across projects.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeUser    Scope = "user"
)

// ParseScope accepts the two supported scopes. Empty keeps project defaults.
func ParseScope(raw string) (Scope, error) {
	switch s := Scope(strings.TrimSpace(raw)); s {
	case "", ScopeProject:
		return ScopeProject, nil
	case ScopeUser:
		return s, nil
	default:
		return "", fmt.Errorf("--scope %q: must be project or user", raw)
	}
}

// scriptsDirFor separates storage from the agent's settings: user scripts
// always live under HOME/.vaultmind, including Claude Code's.
func scriptsDirFor(root string, agent Agent, scope Scope) string {
	if scope == ScopeUser {
		return ScriptsDir(root, AgentCodex)
	}
	return ScriptsDir(root, agent)
}

func claudeHooksForScope(root, vault string, vaults []string, scope Scope) []canonicalHook {
	if scope != ScopeUser {
		return canonicalHooksFor(vault, vaults)
	}
	return canonicalHooksWith(vault, vaults, func(script string) string {
		return singleQuote(filepath.Join(scriptsDirFor(root, AgentClaude, scope), script))
	})
}

// UserScopeInstalled notices legacy installs too, without declaring them
// healthy or changing the project's own status/exit code.
func UserScopeInstalled(home string) bool {
	if home == "" {
		return false
	}
	for _, n := range allScriptNames() {
		b, err := readSettingsFile(filepath.Join(scriptsDirFor(home, AgentClaude, ScopeUser), n))
		if err == nil && b != nil {
			return true
		}
	}
	for _, e := range eventWiringForProfile(home, ProfileFull) {
		if e.State != EventUnwired {
			return true
		}
	}
	return false
}

// UserAgentStanza renders a verified user hook file. Codex runs from session
// cwd (https://learn.chatgpt.com/docs/hooks); Cursor's adapter gets the project
// from workspace_roots (https://cursor.com/docs/hooks).
func UserAgentStanza(home, vault string, vaults []string, p Profile, a Agent) (string, error) {
	if a == AgentCursor {
		b, err := mergeCursorHooks(nil, cursorHooksForScope(home, vault, vaults, p, ScopeUser))
		return string(b), err
	}
	return renderStanza(codexHooksForScope(home, vault, vaults, p, ScopeUser))
}

// MergeUserAgentHooks uses the same preservation/idempotency rules as the
// corresponding project install, targeting the harness's user config.
func MergeUserAgentHooks(home, vault string, vaults []string, p Profile, a Agent, dryRun bool) (*MergeFileResult, error) {
	path := UserCodexHooksPath()
	if a == AgentCursor {
		path = CursorHooksPath(home)
	}
	existing, err := readSettingsFile(path)
	if err != nil {
		return nil, err
	}
	var merged []byte
	var changed bool
	if a == AgentCursor {
		merged, err = mergeCursorHooks(existing, cursorHooksForScope(home, vault, vaults, p, ScopeUser))
		changed = existing == nil || !sameCursorDoc(existing, merged)
	} else {
		merged, changed, err = mergeHooksWith(existing, codexHooksForScope(home, vault, vaults, p, ScopeUser), true)
	}
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

// UserCodexHooksPath honors CODEX_HOME, like Codex's own config discovery.
func UserCodexHooksPath() string { return filepath.Join(codexHomeDir(), codexHooksFile) }

// InstallProfileForScope keeps user declarations independent from a project
// install made with HOME as its root. Legacy undeclared user scripts are full.
func InstallProfileForScope(root string, scope Scope) (Profile, error) {
	if scope != ScopeUser {
		return InstallProfile(root)
	}
	p, declared, err := declaredProfileFor(root, []Agent{AgentCodex})
	if err != nil || declared {
		return p, err
	}
	if _, err := os.Stat(scriptsDirFor(root, AgentClaude, scope)); err == nil {
		return ProfileFull, nil
	}
	return ProfileKnowledge, nil
}

func declaredProfileForScope(root string, scope Scope) (Profile, error) {
	if scope != ScopeUser {
		return DeclaredProfile(root)
	}
	p, declared, err := declaredProfileFor(root, []Agent{AgentCodex})
	if err != nil || declared {
		return p, err
	}
	return ProfileFull, nil
}
