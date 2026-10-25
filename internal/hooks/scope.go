package hooks

import (
	"encoding/json"
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
	for _, e := range eventWiringForScope(home, ProfileFull, ScopeUser) {
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
	return SelectedAgentStanza(home, vault, vaults, p, a, ScopeUser, nil)
}

// SelectedAgentStanza renders only the scripts requested by --only.
func SelectedAgentStanza(home, vault string, vaults []string, p Profile, a Agent, scope Scope, only []string) (string, error) {
	if a == AgentCursor {
		b, err := mergeCursorHooks(nil, selectedCursorHooks(cursorHooksForScope(home, vault, vaults, p, scope), only))
		return string(b), err
	}
	return renderStanza(selectedHooks(codexHooksForScope(home, vault, vaults, p, scope), only))
}

// MergeUserAgentHooks uses the same preservation/idempotency rules as the
// corresponding project install, targeting the harness's user config.
func MergeUserAgentHooks(home, vault string, vaults []string, p Profile, a Agent, dryRun bool) (*MergeFileResult, error) {
	return MergeSelectedAgentHooks(home, vault, vaults, p, a, ScopeUser, dryRun, nil)
}

// MergeSelectedAgentHooks wires only the requested scripts, preserving other
// entries; user commands are migrated only when their shape is generated.
func MergeSelectedAgentHooks(home, vault string, vaults []string, p Profile, a Agent, scope Scope, dryRun bool, only []string) (*MergeFileResult, error) {
	path := UserCodexHooksPath()
	if scope != ScopeUser {
		path = CodexHooksPath(home)
	}
	if a == AgentCursor {
		path = CursorHooksPath(home)
	}
	existing, err := readSettingsFile(path)
	if err != nil {
		return nil, err
	}
	var merged []byte
	original := existing
	if scope == ScopeUser {
		existing, err = migrateUserCommands(existing, home, a)
		if err != nil {
			return nil, fmt.Errorf("merging into %s: %w", path, err)
		}
	}
	var changed bool
	if a == AgentCursor {
		want := selectedCursorHooks(cursorHooksForScope(home, vault, vaults, p, scope), only)
		if len(only) > 0 {
			merged, err = mergeSelectedCursorHooks(existing, want)
		} else {
			merged, err = mergeCursorHooks(existing, want)
		}
		changed = existing == nil || !sameCursorDoc(existing, merged)
	} else {
		merged, changed, err = mergeHooksWith(existing, selectedHooks(codexHooksForScope(home, vault, vaults, p, scope), only), scope != ScopeUser)
	}
	if err != nil {
		return nil, fmt.Errorf("merging into %s: %w", path, err)
	}
	changed = changed || string(original) != string(existing)
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

func selectedScript(script string, only []string) bool {
	if len(only) == 0 {
		return true
	}
	for _, s := range only {
		if s == script {
			return true
		}
	}
	return false
}

func selectedHooks(all []canonicalHook, only []string) []canonicalHook {
	var out []canonicalHook
	for _, h := range all {
		if selectedScript(h.Script, only) {
			out = append(out, h)
		}
	}
	return out
}

func selectedCursorHooks(all []cursorEntry, only []string) []cursorEntry {
	var out []cursorEntry
	for _, h := range all {
		if selectedScript(h.Script, only) {
			out = append(out, h)
		}
	}
	return out
}

func mergeSelectedCursorHooks(existing []byte, want []cursorEntry) ([]byte, error) {
	d, err := parseCursorDoc(existing)
	if err != nil {
		return nil, err
	}
	for _, w := range want {
		kept := d.hooks[w.Event][:0:0]
		for _, raw := range d.hooks[w.Event] {
			if cursorScriptOf(raw) != w.Script {
				kept = append(kept, raw)
			}
		}
		raw, err := json.Marshal(w.Hook)
		if err != nil {
			return nil, err
		}
		d.hooks[w.Event] = append(kept, raw)
	}
	return d.render()
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
