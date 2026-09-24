package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"
)

// Codex approval check — see codex_approval_test.go for why it exists.

// CodexApprovalState is what Codex has recorded for one VaultMind hook.
type CodexApprovalState string

// Approval states.
const (
	CodexApproved    CodexApprovalState = "approved"
	CodexNotApproved CodexApprovalState = "not_approved"
	CodexDisabled    CodexApprovalState = "disabled"
)

const (
	codexHomeEnv    = "CODEX_HOME"
	codexConfigFile = "config.toml"
)

// CodexHookApproval is one VaultMind hook in .codex/hooks.json.
type CodexHookApproval struct {
	Event  string             `json:"event"`
	Script string             `json:"script"`
	State  CodexApprovalState `json:"state"`
}

// CodexApproval reports the approvals for a project's Codex hooks.
type CodexApproval struct {
	HooksFile string              `json:"hooks_file"`
	Hooks     []CodexHookApproval `json:"hooks"`
}

// Approved counts hooks Codex will run.
func (a *CodexApproval) Approved() int {
	n := 0
	for _, h := range a.Hooks {
		if h.State == CodexApproved {
			n++
		}
	}
	return n
}

// Unapproved counts hooks Codex will skip — never approved, or disabled.
func (a *CodexApproval) Unapproved() int { return len(a.Hooks) - a.Approved() }

// codexHookState is one [hooks.state."<key>"] table in Codex's config.toml.
type codexHookState struct {
	TrustedHash string `toml:"trusted_hash"`
	Enabled     *bool  `toml:"enabled"`
}

// codexHomeDir is $CODEX_HOME, else ~/.codex — where Codex keeps config.toml.
func codexHomeDir() string {
	if h := os.Getenv(codexHomeEnv); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, codexDir)
}

// codexApprovals checks every VaultMind hook in projectDir's .codex/hooks.json
// against the approvals in codexHome's config.toml. Hooks is empty when the
// project has no Codex hooks file, or none of its hooks are VaultMind's.
func codexApprovals(projectDir, codexHome string) (CodexApproval, error) {
	hooksFile := CodexHooksPath(projectDir)
	report := CodexApproval{HooksFile: hooksFile}
	// The project's own hooks file.
	// #nosec G304
	// nosemgrep: go-path-traversal
	raw, err := os.ReadFile(hooksFile)
	if errors.Is(err, fs.ErrNotExist) {
		return report, nil
	}
	if err != nil {
		return report, fmt.Errorf("reading %s: %w", hooksFile, err)
	}
	var file struct {
		Hooks map[string][]hookGroup `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return report, fmt.Errorf("parsing %s: %w", hooksFile, err)
	}
	approved, err := readCodexHookStates(codexHome)
	if err != nil {
		return report, err
	}

	// Codex keys the path as it resolved the project; accept either spelling.
	sources := []string{hooksFile}
	if abs, aerr := filepath.Abs(hooksFile); aerr == nil {
		sources = append(sources, abs)
	}
	if resolved, rerr := filepath.EvalSymlinks(hooksFile); rerr == nil {
		sources = append(sources, resolved)
	}

	for _, event := range sortedEvents(file.Hooks) {
		for gi, group := range file.Hooks[event] {
			for hi, h := range group.Hooks {
				script := vaultmindScriptIn(h.Command)
				if script == "" {
					continue
				}
				state := CodexNotApproved
				for _, src := range sources {
					key := fmt.Sprintf("%s:%s:%d:%d", src, codexEventLabel(event), gi, hi)
					if st, ok := approved[key]; ok {
						state = CodexApproved
						if st.Enabled != nil && !*st.Enabled {
							state = CodexDisabled
						}
						break
					}
				}
				report.Hooks = append(report.Hooks, CodexHookApproval{Event: event, Script: script, State: state})
			}
		}
	}
	return report, nil
}

// readCodexHookStates loads [hooks.state] from Codex's config.toml. A missing
// file is not an error: it means Codex has recorded no approvals here.
func readCodexHookStates(codexHome string) (map[string]codexHookState, error) {
	path := filepath.Join(codexHome, codexConfigFile)
	// Codex's own config file.
	// #nosec G304
	// nosemgrep: go-path-traversal
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]codexHookState{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var cfg struct {
		Hooks struct {
			State map[string]codexHookState `toml:"state"`
		} `toml:"hooks"`
	}
	if err := toml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if cfg.Hooks.State == nil {
		return map[string]codexHookState{}, nil
	}
	return cfg.Hooks.State, nil
}

// vaultmindScriptIn returns the VaultMind script a hook command runs, or "".
func vaultmindScriptIn(command string) string {
	for script := range codexScripts {
		if strings.Contains(command, "/"+script) {
			return script
		}
	}
	return ""
}

// codexEventLabel is Codex's key label for a hooks.json event name:
// "SessionStart" -> "session_start" (codex-rs hook_event_key_label).
func codexEventLabel(event string) string {
	var b strings.Builder
	for i, r := range event {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// sortedEvents lists the events in lifecycle order, unknown ones last.
func sortedEvents(m map[string][]hookGroup) []string {
	order := []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PreCompact", "SessionEnd"}
	var out []string
	seen := map[string]bool{}
	for _, e := range order {
		if _, ok := m[e]; ok {
			out = append(out, e)
			seen[e] = true
		}
	}
	var rest []string
	for e := range m {
		if !seen[e] {
			rest = append(rest, e)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}
