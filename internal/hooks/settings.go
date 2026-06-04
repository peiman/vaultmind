package hooks

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The canonical five-hook wiring for a VaultMind-backed Claude Code project.
// This is the source of truth for the stanza that `hooks install` EMITS, so
// a consumer doesn't have to hand-transcribe it from the onboarding doc
// (issue #41 — the single biggest install-time friction was wiring
// settings.json by reading ~8 files). Each entry maps a Claude Code hook
// event to one of the embedded scripts.
//
// Note: internal/onboard/AGENT_ONBOARDING.md still carries its own literal
// copy of the stanza for illustration; the two are kept semantically aligned
// by hand, not generated from this. If they drift, this code is authoritative
// for what the command produces.
//
// Order is the session lifecycle (start → prompt → read → end), preserved in
// the emitted JSON via the explicit struct fields below so the copy-paste
// block reads top-to-bottom the way the hooks fire.
const (
	hookSessionStartScript     = "load-persona.sh"
	hookHealthScript           = "vaultmind-health.sh"
	hookUserPromptSubmitScript = "vault-recall.sh"
	hookPreToolUseScript       = "vault-track-read.sh"
	hookSessionEndScript       = "capture-episode.sh"
)

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type hookGroup struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

// hooksObject uses explicit fields (not a map) so json.MarshalIndent emits
// the events in lifecycle order rather than alphabetical map-key order.
type hooksObject struct {
	SessionStart     []hookGroup `json:"SessionStart"`
	UserPromptSubmit []hookGroup `json:"UserPromptSubmit"`
	PreToolUse       []hookGroup `json:"PreToolUse"`
	SessionEnd       []hookGroup `json:"SessionEnd"`
}

type settingsStanza struct {
	Hooks hooksObject `json:"hooks"`
}

// canonicalHook pairs a Claude Code event with the VaultMind script that backs
// it (Script — used as the dedup key when merging/uninstalling) and the wiring
// group emitted for that event.
type canonicalHook struct {
	Event  string
	Script string
	Group  hookGroup
}

// canonicalHooks is the single source of truth for VaultMind's five-hook
// wiring, in session-lifecycle order (start → prompt → read → end). Both
// SettingsStanza (the copy-paste/print path) and MergeStanza (the in-place
// merge path) build from this slice, so the matcher values and event→script
// mapping are defined exactly once (manifesto principle 7 — SSOT).
func canonicalHooks(vaultPath string) []canonicalHook {
	cmd := func(script string) hookCommand {
		return hookCommand{Type: "command", Command: hookCommandString(script, vaultPath)}
	}
	return []canonicalHook{
		{Event: "SessionStart", Script: hookSessionStartScript, Group: hookGroup{Matcher: "startup", Hooks: []hookCommand{cmd(hookSessionStartScript)}}},
		// vault-agnostic health/onboarding nudge — every adopter (persona AND
		// knowledge-vault) gets the "what do I do next?" answer at session
		// start. Additive: coexists with load-persona.sh on SessionStart.
		{Event: "SessionStart", Script: hookHealthScript, Group: hookGroup{Matcher: "startup", Hooks: []hookCommand{cmd(hookHealthScript)}}},
		{Event: "UserPromptSubmit", Script: hookUserPromptSubmitScript, Group: hookGroup{Hooks: []hookCommand{cmd(hookUserPromptSubmitScript)}}},
		{Event: "PreToolUse", Script: hookPreToolUseScript, Group: hookGroup{Matcher: "Read", Hooks: []hookCommand{cmd(hookPreToolUseScript)}}},
		{Event: "SessionEnd", Script: hookSessionEndScript, Group: hookGroup{Hooks: []hookCommand{cmd(hookSessionEndScript)}}},
	}
}

// SettingsStanza renders the .claude/settings.json "hooks" object that wires
// VaultMind's five canonical hooks. The returned string is pretty-printed
// valid JSON for the operator (or an agent) to merge into their settings.
//
// When vaultPath is non-empty, every command is prefixed with a
// VAULTMIND_VAULT='<path>' assignment so recall, read-tracking, episode
// capture, and persona loading all target the consumer's vault instead of
// the built-in default ($CLAUDE_PROJECT_DIR/vaultmind-identity). The scripts
// honor VAULTMIND_VAULT as an override; an empty vaultPath leaves the
// commands unparameterized so they fall back to that default.
func SettingsStanza(vaultPath string) (string, error) {
	var obj hooksObject
	// Append (not assign) per event: an event can carry more than one canonical
	// group — SessionStart wires both load-persona.sh and vaultmind-health.sh.
	for _, ch := range canonicalHooks(vaultPath) {
		switch ch.Event {
		case "SessionStart":
			obj.SessionStart = append(obj.SessionStart, ch.Group)
		case "UserPromptSubmit":
			obj.UserPromptSubmit = append(obj.UserPromptSubmit, ch.Group)
		case "PreToolUse":
			obj.PreToolUse = append(obj.PreToolUse, ch.Group)
		case "SessionEnd":
			obj.SessionEnd = append(obj.SessionEnd, ch.Group)
		}
	}
	out, err := json.MarshalIndent(settingsStanza{Hooks: obj}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("rendering settings stanza: %w", err)
	}
	return string(out), nil
}

// hookCommandString builds the shell command for one hook. The script is
// resolved relative to $CLAUDE_PROJECT_DIR (Claude Code exports it), matching
// the onboarding doc's convention. A non-empty vaultPath is single-quoted and
// exported as VAULTMIND_VAULT so the literal path survives JSON encoding and
// the shell does not expand it.
func hookCommandString(script, vaultPath string) string {
	base := `bash "$CLAUDE_PROJECT_DIR"/.claude/scripts/` + script
	if vaultPath == "" {
		return base
	}
	return "VAULTMIND_VAULT=" + singleQuote(vaultPath) + " " + base
}

// singleQuote wraps s in single quotes, escaping any embedded single quote
// with the standard '\” shell idiom so the assignment is safe for arbitrary
// paths.
func singleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
