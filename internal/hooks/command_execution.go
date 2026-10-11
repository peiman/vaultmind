package hooks

import (
	"path/filepath"
	"strings"
)

// executedScript selects only the script operand of a known shell, or a
// directly executed .sh path. It never searches ordinary command arguments.
// This is intentionally a small grammar, not a shell evaluator: wrappers,
// command strings (-c), stdin (-s), noexec (-n), unknown options and ambiguous
// expansions are not evidence that a canonical script executes.
func executedScript(command string) ([]commandWord, int, bool) {
	words, ok := commandWords(command)
	if !ok {
		return nil, 0, false
	}
	// Codex and Cursor generate this one export before their command; older
	// versions exported CLAUDE_PROJECT_DIR instead.
	if len(words) >= 3 && words[0].value == "export" &&
		(strings.HasPrefix(command[words[1].start:words[1].end], envProjectDir+"=") ||
			strings.HasPrefix(command[words[1].start:words[1].end], "CLAUDE_PROJECT_DIR=")) &&
		command[words[2].start:words[2].end] == ";" {
		words = words[3:]
	}
	// Understand simple commands separated by semicolons, but select only the
	// first. Reject other shell operators and incomplete command lists rather
	// than claiming a script executes from a possibly invalid shell program.
	end, atStart := len(words), true
	for i, w := range words {
		raw := command[w.start:w.end]
		if strings.HasPrefix(raw, "#") {
			end = min(end, i)
			break
		}
		if i > 0 && strings.ContainsAny(command[words[i-1].end:w.start], "\r\n") {
			return words, 0, false
		}
		if raw == ";" && !atStart {
			end, atStart = min(end, i), true
			continue
		}
		if strings.ContainsAny(raw[:1], ";|&<>()") || (atStart && shellReservedWord(raw)) {
			return words, 0, false
		}
		atStart = false
	}
	words = words[:end]
	i := 0
	for i < len(words) && shellAssignment(command[words[i].start:words[i].end]) {
		i++
	}
	if i == len(words) || !canonicalScriptWord(command, words[i]) {
		return words, 0, false
	}
	shell := filepath.Base(words[i].value)
	switch shell {
	case "bash", "sh", "zsh":
		i++
		for i < len(words) {
			option := words[i]
			if option.value == "--" {
				i++
				break
			}
			if !strings.HasPrefix(option.value, "-") && !strings.HasPrefix(option.value, "+") {
				break
			}
			if option.dynamic || option.expansion {
				return words, 0, false
			}
			operands, valid := shellOption(shell, option.value)
			if !valid {
				return words, 0, false
			}
			i += 1 + operands
		}
	default:
		if !strings.Contains(words[i].value, "/") || !strings.HasSuffix(words[i].value, ".sh") {
			return words, 0, false
		}
	}
	return words, i, i < len(words)
}

func shellReservedWord(raw string) bool {
	switch raw {
	case "if", "then", "else", "elif", "fi", "for", "while", "until", "do", "done", "case", "esac", "function", "!", "{", "}", "[[", "]]":
		return true
	default:
		return false
	}
}

func shellAssignment(raw string) bool {
	key, _, ok := strings.Cut(raw, "=")
	if !ok || key == "" {
		return false
	}
	for i, c := range key {
		valid := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(i > 0 && c >= '0' && c <= '9')
		if !valid {
			return false
		}
	}
	return true
}

// shellOption returns the number of separate operands consumed by an option.
// Only known startup options are accepted; -c/-s/-n never select a script file.
func shellOption(shell, option string) (int, bool) {
	if shell == "bash" {
		switch option {
		case "--rcfile", "--init-file":
			return 1, true
		case "--noprofile", "--norc", "--posix", "--restricted", "--verbose", "--login":
			return 0, true
		}
	}
	allowed := "abefhilmprtuvxBCEHPT"
	switch shell {
	case "sh":
		allowed = "abefilmpruvxC"
	case "zsh":
		allowed = "defilruvx"
	}
	if len(option) < 2 {
		return 0, false
	}
	for i, c := range option[1:] {
		if c == 'o' || (c == 'O' && shell == "bash") {
			return 1, i == len(option)-2
		}
		if !strings.ContainsRune(allowed, c) {
			return 0, false
		}
	}
	return 0, true
}

func canonicalScriptWord(command string, w commandWord) bool {
	if w.expansion {
		return false
	}
	// Keep diagnosing the legacy generated path as stale at user scope.
	return !w.dynamic || command[w.start:w.end] == claudeScriptRef(filepath.Base(w.value)) ||
		command[w.start:w.end] == "$CLAUDE_PROJECT_DIR/.claude/scripts/"+filepath.Base(w.value)
}
