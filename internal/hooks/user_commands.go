package hooks

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type commandWord struct {
	value      string
	start, end int
	dynamic    bool
	expansion  bool
}

// commandWords reads shell words without executing or expanding anything.
// Positions retain the exact spelling for conservative generated-shape checks.
func commandWords(command string) ([]commandWord, bool) {
	var words []commandWord
	for i := 0; i < len(command); {
		if strings.ContainsRune(" \t\r\n", rune(command[i])) {
			i++
			continue
		}
		w := commandWord{start: i}
		var value strings.Builder
		quote := byte(0)
		for i < len(command) {
			c := command[i]
			if quote == 0 && strings.ContainsRune(" \t\r\n;|&<>()", rune(c)) {
				break
			}
			if c == '\\' && quote != '\'' {
				i++
				if i == len(command) {
					return nil, false
				}
				if quote == '"' && !strings.ContainsRune("$`\"\\\n", rune(command[i])) {
					value.WriteByte('\\')
				}
				if command[i] == '\n' {
					i++
					continue
				}
				value.WriteByte(command[i])
				i++
				continue
			}
			if c == '\'' || c == '"' {
				if quote == 0 {
					quote = c
					i++
					continue
				}
				if quote == c {
					quote = 0
					i++
					continue
				}
			}
			if quote != '\'' && (c == '$' || c == '`') {
				w.dynamic = true
			}
			if quote == 0 && strings.ContainsRune("*?[{}~", rune(c)) {
				w.expansion = true
			}
			value.WriteByte(c)
			i++
		}
		if quote != 0 {
			return nil, false
		}
		if i == w.start {
			value.WriteByte(command[i])
			i++
		}
		w.value, w.end = value.String(), i
		words = append(words, w)
	}
	return words, true
}

// generatedUserCommand accepts only our canonical command grammar. Basename
// matching alone is insufficient: a wrapper, extra argument or shell operation
// belongs to the user and must never be rewritten, even under --force.
func generatedUserCommand(command, home string, agent Agent) (string, bool) {
	words, ok := commandWords(command)
	if !ok || len(words) < 2 {
		return "", false
	}
	i := 0
	prefix := ""
	if words[i].value == "export" {
		if agent == AgentClaude || len(words) < 5 || words[2].value != ";" {
			return "", false
		}
		v, has := strings.CutPrefix(words[1].value, envProjectDir+"=")
		raw := command[words[1].start:words[1].end]
		if !has || (raw != envProjectDir+"="+singleQuote(v) && raw != `VAULTMIND_PROJECT_DIR="$PWD"` && !isPortableExportWord(raw)) {
			return "", false
		}
		i = 3
		if agent == AgentCodex {
			prefix = `export VAULTMIND_PROJECT_DIR="$PWD"; `
		}
	}
	for _, key := range []string{"VAULTMIND_VAULTS", "VAULTMIND_VAULT"} {
		if i >= len(words) {
			return "", false
		}
		if v, has := strings.CutPrefix(words[i].value, key+"="); has {
			raw := command[words[i].start:words[i].end]
			if raw != key+"="+singleQuote(v) {
				return "", false
			}
			prefix += raw + " "
			i++
		}
	}
	if i+1 >= len(words) || words[i].value != "bash" {
		return "", false
	}
	path := words[i+1]
	ref, ok := generatedScriptRef(command, path, home, agent)
	if !ok {
		return "", false
	}
	if agent == AgentCursor {
		suffix, ok := generatedCursorArgs(command, words[i+1:])
		if !ok {
			return "", false
		}
		ref += suffix
	} else if i+2 != len(words) {
		return "", false
	}
	return prefix + "bash " + ref, true
}

func generatedScriptRef(command string, path commandWord, home string, agent Agent) (string, bool) {
	script := filepath.Base(path.value)
	known := agent == AgentCursor && script == cursorAdapterScript
	for _, name := range allScriptNames() {
		known = known || script == name
	}
	if !known {
		return "", false
	}
	rawPath := command[path.start:path.end]
	if rawPath == codexPortableScriptRef(script) && agent == AgentCodex && isPortableCommand(command) {
		return singleQuote(filepath.Join(scriptsDirFor(home, agent, ScopeUser), script)), true
	}
	// Current absolute and legacy project-variable forms are emitted by us.
	if rawPath != singleQuote(path.value) && rawPath != claudeScriptRef(script) {
		return "", false
	}
	if !filepath.IsAbs(path.value) && rawPath != claudeScriptRef(script) {
		return "", false
	}
	if rawPath != claudeScriptRef(script) && !strings.HasSuffix(path.value, "/.claude/scripts/"+script) && !strings.HasSuffix(path.value, "/.vaultmind/scripts/"+script) {
		return "", false
	}
	return singleQuote(filepath.Join(scriptsDirFor(home, agent, ScopeUser), script)), true
}

func generatedCursorArgs(command string, words []commandWord) (string, bool) {
	if len(words) < 2 || filepath.Base(words[0].value) != cursorAdapterScript {
		return "", false
	}
	target := words[1].value
	w, exists := cursorWiring[target]
	if !exists || command[words[1].start:words[1].end] != target {
		return "", false
	}
	suffix := " " + target
	count := 2
	if w.tools != "" {
		if count >= len(words) || command[words[count].start:words[count].end] != singleQuote(w.tools) {
			return "", false
		}
		suffix += " " + singleQuote(w.tools)
		count++
	}
	return suffix, count == len(words)
}

// migrateUserCommands changes only generated command strings, keeping group
// placement, matchers, timeouts and all unknown fields. Conflicts gate the file
// write and include the exact custom command needing manual attention.
func migrateUserCommands(existing []byte, home string, agent Agent) ([]byte, error) {
	top, err := parseOrderedObject(existing)
	if err != nil {
		return nil, err
	}
	raw, ok := top.get("hooks")
	if !ok {
		return existing, nil
	}
	hooks, err := parseOrderedObjectFromRaw(raw)
	if err != nil {
		return nil, err
	}
	changed := false
	for _, event := range hooks.keys {
		var entries []json.RawMessage
		if err := json.Unmarshal(hooks.values[event], &entries); err != nil {
			return nil, err
		}
		for i, entry := range entries {
			obj, err := parseOrderedObjectFromRaw(entry)
			if err != nil {
				return nil, err
			}
			if agent == AgentCursor {
				entries[i], ok, err = migrateUserEntry(entry, home, agent)
			} else if hs, has := obj.get("hooks"); has {
				var commands []json.RawMessage
				if err := json.Unmarshal(hs, &commands); err != nil {
					return nil, err
				}
				ok = false
				for j, c := range commands {
					var update bool
					commands[j], update, err = migrateUserEntry(c, home, agent)
					if err != nil {
						return nil, fmt.Errorf("%s: %w", event, err)
					}
					ok = ok || update
				}
				if ok {
					b, merr := json.Marshal(commands)
					if merr != nil {
						return nil, merr
					}
					obj.set("hooks", b)
					entries[i], err = obj.marshal()
				}
			} else {
				ok = false
			}
			if err != nil {
				return nil, fmt.Errorf("%s: %w", event, err)
			}
			changed = changed || ok
		}
		b, err := json.Marshal(entries)
		if err != nil {
			return nil, err
		}
		hooks.set(event, b)
	}
	if !changed {
		return existing, nil
	}
	b, err := hooks.marshal()
	if err != nil {
		return nil, err
	}
	top.set("hooks", b)
	return top.marshalIndent()
}

func migrateUserEntry(raw json.RawMessage, home string, agent Agent) (json.RawMessage, bool, error) {
	obj, err := parseOrderedObjectFromRaw(raw)
	if err != nil {
		return nil, false, err
	}
	c, has := obj.get("command")
	if !has {
		return raw, false, nil
	}
	var command string
	if err := json.Unmarshal(c, &command); err != nil {
		return nil, false, err
	}
	if len(scriptsInCommand(command, agent)) == 0 && !commandReferencesScript(command, cursorAdapterScript) {
		return raw, false, nil
	}
	replacement, generated := generatedUserCommand(command, home, agent)
	if !generated {
		return nil, false, fmt.Errorf("custom command conflict: %s; edit it to use an existing absolute script path or remove its wiring, then rerun; custom commands are never rewritten", command)
	}
	if replacement == command {
		return raw, false, nil
	}
	b, err := json.Marshal(replacement)
	if err != nil {
		return nil, false, err
	}
	obj.set("command", b)
	b, err = obj.marshal()
	return b, true, err
}
