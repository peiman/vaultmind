package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ScopeOverlap names the scripts an agent dispatches from both scopes on an event.
type ScopeOverlap struct {
	Agent           Agent    `json:"agent"`
	Event           string   `json:"event"`
	Scripts         []string `json:"scripts"`
	UserSettings    string   `json:"user_settings"`
	ProjectSettings string   `json:"project_settings"`
}

// BrokenCommand reports the actual wiring, independently of installed copies.
type BrokenCommand struct {
	Agent        Agent  `json:"agent"`
	Event        string `json:"event"`
	Command      string `json:"command"`
	SettingsPath string `json:"settings_path"`
	Reason       string `json:"reason"`
	Fix          string `json:"fix"`
}

type wiredCommand struct{ event, command, settings string }

func wiringPaths(root string, scope Scope, agent Agent) []string {
	switch agent {
	case AgentCodex:
		if scope == ScopeUser {
			return []string{UserCodexHooksPath()}
		}
		return []string{CodexHooksPath(root)}
	case AgentCursor:
		return []string{CursorHooksPath(root)}
	default:
		paths := []string{settingsFilePath(root, false)}
		if scope != ScopeUser {
			paths = append(paths, settingsFilePath(root, true))
		}
		return paths
	}
}

func commandsIn(data []byte, path string, agent Agent) ([]wiredCommand, error) {
	top, err := parseOrderedObject(data)
	if err != nil {
		return nil, err
	}
	raw, ok := top.get("hooks")
	if !ok {
		return nil, nil
	}
	obj, err := parseOrderedObjectFromRaw(raw)
	if err != nil {
		return nil, err
	}
	var out []wiredCommand
	for _, event := range obj.keys {
		var entries []json.RawMessage
		if err := json.Unmarshal(obj.values[event], &entries); err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if agent == AgentCursor {
				var h cursorHookEntry
				if err := json.Unmarshal(entry, &h); err != nil {
					return nil, err
				}
				out = append(out, wiredCommand{event, h.Command, path})
			} else {
				var g hookGroup
				if err := json.Unmarshal(entry, &g); err != nil {
					return nil, err
				}
				for _, h := range g.Hooks {
					out = append(out, wiredCommand{event, h.Command, path})
				}
			}
		}
	}
	return out, nil
}

func readWiring(root string, scope Scope, agent Agent) ([]wiredCommand, error) {
	var out []wiredCommand
	for _, path := range wiringPaths(root, scope, agent) {
		data, err := readSettingsFile(path)
		if err != nil {
			return nil, err
		}
		entries, err := commandsIn(data, path, agent)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		out = append(out, entries...)
	}
	return out, nil
}

func scriptsInCommand(command string, agent Agent) []string {
	words, _ := commandWords(command)
	if agent == AgentCursor {
		for i, word := range words {
			if filepath.Base(word.value) == cursorAdapterScript && i+1 < len(words) {
				s := words[i+1].value
				if cursorWiring[s].event != "" {
					return []string{s}
				}
			}
		}
	}
	var out []string
	for _, name := range allScriptNames() {
		found := commandReferencesScript(command, name)
		for _, word := range words {
			found = found || filepath.Base(word.value) == name
		}
		if found {
			out = append(out, name)
		}
	}
	return out
}

// ScopeOverlaps compares actual wiring, including project-local Claude settings.
// An optional preview substitutes just the file being merged, so dry runs warn
// about the wiring they would create without writing it first.
func ScopeOverlaps(project, home string, preview *MergeFileResult, agentFilter Agent) ([]ScopeOverlap, error) {
	var out []ScopeOverlap
	for _, agent := range []Agent{AgentClaude, AgentCodex, AgentCursor} {
		if agentFilter != "" && agent != agentFilter {
			continue
		}
		read := func(root string, scope Scope) ([]wiredCommand, error) {
			var entries []wiredCommand
			for _, path := range wiringPaths(root, scope, agent) {
				data, err := readSettingsFile(path)
				if err != nil {
					return nil, err
				}
				if preview != nil && sameSettingsPath(path, preview.SettingsPath) {
					data = []byte(preview.Merged)
				}
				commands, err := commandsIn(data, path, agent)
				if err != nil {
					return nil, fmt.Errorf("reading %s: %w", path, err)
				}
				entries = append(entries, commands...)
			}
			return entries, nil
		}
		user, err := read(home, ScopeUser)
		if err != nil {
			return nil, err
		}
		local, err := read(project, ScopeProject)
		if err != nil {
			return nil, err
		}
		for _, path := range wiringPaths(project, ScopeProject, agent) {
			byEvent := map[string]map[string]bool{}
			userPath := ""
			for _, u := range user {
				for _, p := range local {
					// HOME used as a project must not compare a file with itself.
					if p.settings != path || sameSettingsPath(u.settings, p.settings) || u.event != p.event {
						continue
					}
					for _, us := range scriptsInCommand(u.command, agent) {
						for _, ps := range scriptsInCommand(p.command, agent) {
							if us != ps {
								continue
							}
							if byEvent[p.event] == nil {
								byEvent[p.event] = map[string]bool{}
							}
							byEvent[p.event][ps] = true
							userPath = u.settings
						}
					}
				}
			}
			events := make([]string, 0, len(byEvent))
			for e := range byEvent {
				events = append(events, e)
			}
			sort.Strings(events)
			for _, e := range events {
				var scripts []string
				for s := range byEvent[e] {
					scripts = append(scripts, s)
				}
				sort.Strings(scripts)
				out = append(out, ScopeOverlap{agent, e, scripts, userPath, path})
			}
		}
	}
	return out, nil
}

func sameSettingsPath(a, b string) bool {
	aa, aerr := filepath.Abs(a)
	bb, berr := filepath.Abs(b)
	if aerr == nil && berr == nil && aa == bb {
		return true
	}
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	return aerr == nil && berr == nil && os.SameFile(ai, bi)
}

func brokenUserCommands(home string) ([]BrokenCommand, error) {
	var out []BrokenCommand
	for _, agent := range []Agent{AgentClaude, AgentCodex, AgentCursor} {
		entries, err := readWiring(home, ScopeUser, agent)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if reason := userCommandProblem(entry.command, agent); reason != "" {
				fix := "edit this command to use an existing absolute script path; custom commands are never rewritten"
				if _, ok := generatedUserCommand(entry.command, home, agent); ok {
					fix = fmt.Sprintf("vaultmind hooks install --scope user --agent %s --merge (preview with --dry-run)", agent)
				}
				out = append(out, BrokenCommand{agent, entry.event, entry.command, entry.settings, reason, fix})
			}
		}
	}
	return out, nil
}

func userCommandProblem(command string, agent Agent) string {
	words, ok := commandWords(command)
	if !ok {
		return "cannot resolve script path in command"
	}
	found := false
	for i, w := range words {
		if !isScriptWord(words, i) {
			continue
		}
		found = true
		path := w.value
		// Cursor's script argument is resolved beside the adapter, not cwd.
		if agent == AgentCursor && i > 0 && filepath.Base(words[i-1].value) == cursorAdapterScript {
			path = filepath.Join(filepath.Dir(words[i-1].value), path)
		}
		if w.dynamic || !filepath.IsAbs(path) {
			return "user-scope script path must be absolute and independent of $CLAUDE_PROJECT_DIR: " + w.value
		}
		info, err := os.Stat(path)
		if err != nil {
			return "script path unavailable: " + path + " (" + err.Error() + ")"
		}
		if !info.Mode().IsRegular() {
			return "script path is not a regular file: " + path
		}
	}
	if !found && len(scriptsInCommand(command, agent)) > 0 {
		return "cannot resolve script path in command"
	}
	return ""
}

func isScriptWord(words []commandWord, i int) bool {
	v := words[i].value
	if strings.Contains(v, "=") && !filepath.IsAbs(v) {
		return false
	}
	if i > 0 {
		switch filepath.Base(words[i-1].value) {
		case "bash", "sh", "python", "python3", "node", "perl", "ruby":
			return !strings.HasPrefix(v, "-")
		}
	}
	return strings.HasSuffix(v, ".sh")
}
