package hooks

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
	// CodexModified: approved once, changed since (e.g. an upgrade rewrote the
	// command). Codex shows it as "modified" in /hooks and skips it.
	CodexModified CodexApprovalState = "modified"
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
	// Key and Hash are Codex's own names for this hook's approval: the
	// [hooks.state."<key>"] entry, and the trusted_hash it must carry.
	Key  string `json:"key"`
	Hash string `json:"hash"`
}

// CodexApproval reports the approvals for a project's Codex hooks.
type CodexApproval struct {
	HooksFile string              `json:"hooks_file"`
	Hooks     []CodexHookApproval `json:"hooks"`
	// ScriptsDir is where Codex runs the scripts from (.vaultmind/scripts);
	// Scripts compares each wired script there with the canonical copy.
	ScriptsDir string         `json:"scripts_dir"`
	Scripts    []ScriptStatus `json:"scripts"`
}

// ScriptCounts tallies the Codex scripts' states.
func (a *CodexApproval) ScriptCounts() (inSync, drifted, missing int) {
	return countScripts(a.Scripts)
}

// wiredScripts lists each VaultMind script the Codex hooks run, once, in order.
func (a *CodexApproval) wiredScripts() []string {
	var out []string
	seen := map[string]bool{}
	for _, h := range a.Hooks {
		if !seen[h.Script] {
			seen[h.Script] = true
			out = append(out, h.Script)
		}
	}
	return out
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
	var file codexHooksFileJSON
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
				hash := codexHookHash(event, group.Matcher, h)
				entry := CodexHookApproval{Event: event, Script: script, State: CodexNotApproved, Hash: hash,
					Key: fmt.Sprintf("%s:%s:%d:%d", hooksFile, codexEventLabel(event), gi, hi)}
				for _, src := range sources {
					key := fmt.Sprintf("%s:%s:%d:%d", src, codexEventLabel(event), gi, hi)
					st, ok := approved[key]
					if !ok {
						continue
					}
					entry.Key = key
					switch {
					case st.TrustedHash != hash:
						entry.State = CodexModified
					case st.Enabled != nil && !*st.Enabled:
						entry.State = CodexDisabled
					default:
						entry.State = CodexApproved
					}
					break
				}
				report.Hooks = append(report.Hooks, entry)
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
		if commandReferencesScript(command, script) {
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

// codexHooksFileJSON is .codex/hooks.json with every field Codex hashes.
type codexHooksFileJSON struct {
	Hooks map[string][]codexGroupJSON `json:"hooks"`
}

type codexGroupJSON struct {
	Matcher *string         `json:"matcher"`
	Hooks   []codexHookJSON `json:"hooks"`
}

type codexHookJSON struct {
	Type                   string  `json:"type"`
	Command                string  `json:"command"`
	Timeout                *int    `json:"timeout"`
	Async                  bool    `json:"async"`
	StatusMessage          *string `json:"statusMessage"`
	AdditionalContextLimit *int    `json:"additionalContextLimit"`
}

// Codex's hook normalization constants (codex-rs 0.156.1).
const (
	codexDefaultTimeout          = 600  // any event but SessionEnd
	codexSessionEndDefault       = 1    // SESSION_END_DEFAULT_TIMEOUT_SEC
	codexDefaultOutputTokenLimit = 2500 // DEFAULT_HOOK_OUTPUT_TOKEN_LIMIT
)

// codexContextEvents are the events whose additionalContextLimit Codex keeps.
var codexContextEvents = map[string]bool{
	"PreToolUse": true, "PostToolUse": true, "SessionStart": true,
	"UserPromptSubmit": true, "SubagentStart": true,
}

// codexHookHash is the trusted_hash Codex records when a hook is approved
// (codex-rs hooks/src/engine/discovery.rs hook_hash, config/src/fingerprint.rs
// version_for_toml, 0.156.1): sha256 over the canonical (sorted-key, compact)
// JSON of the NORMALIZED hook. Normalization fills the timeout default (600s;
// SessionEnd 1s, clamped to 1..3s), always includes async, and keeps
// additionalContextLimit only on context-capable events and only when it is
// not the 2500 default. Unset optional fields are absent, not null.
// An independent implementation reproduced all 10 real trusted hashes on a
// real machine before this was written.
func codexHookHash(event string, matcher *string, h codexHookJSON) string {
	timeout := codexDefaultTimeout
	if event == codexSessionEndEvent {
		timeout = codexSessionEndDefault
		if h.Timeout != nil {
			timeout = min(max(*h.Timeout, 1), codexSessionEndMaxTimeout)
		}
	} else if h.Timeout != nil {
		timeout = max(*h.Timeout, 1)
	}
	handler := map[string]any{"type": h.Type, "command": h.Command, "timeout": timeout, "async": h.Async}
	if h.StatusMessage != nil {
		handler["statusMessage"] = *h.StatusMessage
	}
	if l := h.AdditionalContextLimit; l != nil && codexContextEvents[event] && *l != codexDefaultOutputTokenLimit {
		handler["additionalContextLimit"] = *l
	}
	identity := map[string]any{"event_name": codexEventLabel(event), "hooks": []any{handler}}
	if matcher != nil {
		identity["matcher"] = *matcher
	}
	// encoding/json sorts map keys; serde_json does not HTML-escape, so neither may we.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(identity); err != nil {
		return ""
	}
	sum := sha256.Sum256(bytes.TrimRight(buf.Bytes(), "\n"))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// sortedEvents lists the events in lifecycle order, unknown ones last.
func sortedEvents[G any](m map[string][]G) []string {
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
