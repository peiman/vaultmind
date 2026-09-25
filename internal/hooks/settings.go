package hooks

import (
	"encoding/json"
	"fmt"
	"path/filepath"
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
	hookReachScript            = "vault-reach.sh"
	hookCodeMapScript          = "vault-code-map.sh"
	hookPreCompactScript       = "precompact-preserve.sh"
	hookSessionEndScript       = "capture-episode.sh"
)

// reachMatcher is the tools the reach hook stands in front of: Bash, where
// irreversible commands run, and the file-writing tools, where arcs are
// actually written. Bash alone left it silent on every arc written with Write.
const reachMatcher = "Bash|Edit|Write|MultiEdit"

// codeMapMatcher is the tools that open a file: reading it and changing it.
// The code map rides on them — the knowledge about a file arrives when the
// file is opened.
const codeMapMatcher = "Read|Edit|Write|MultiEdit"

// legacyMatchers are matchers an earlier release installed for a script. A
// group still carrying one is upgraded in place by the merge (only the matcher
// changes; the project's own command and timeout stay) and reported by status.
// A matcher the project set by hand is never touched.
var legacyMatchers = map[string][]string{
	hookReachScript: {"Bash"},
}

func isLegacyMatcher(script, matcher string) bool {
	for _, m := range legacyMatchers[script] {
		if m == matcher {
			return true
		}
	}
	return false
}

// Where each agent's hook files live in a project. Claude Code's are under
// .claude/ (its convention). Codex's are under .vaultmind/: a Codex project
// should not grow a .claude/ folder, nor show "Claude" in the hook commands a
// person reads when approving them in /hooks.
const (
	claudeBaseDir = ".claude"
	codexBaseDir  = ".vaultmind"
	scriptsSubdir = "scripts"
	// envProjectDir is the agent-neutral name the scripts read for the project
	// directory, before Claude Code's CLAUDE_PROJECT_DIR.
	envProjectDir = "VAULTMIND_PROJECT_DIR"
)

// Agent is the coding agent a project's hooks are installed for.
type Agent string

// Supported agents.
const (
	AgentClaude Agent = "claude"
	AgentCodex  Agent = "codex"
)

// baseDir is the per-project folder holding an agent's hook files.
func baseDir(projectDir string, a Agent) string {
	if a == AgentCodex {
		return filepath.Join(projectDir, codexBaseDir)
	}
	return filepath.Join(projectDir, claudeBaseDir)
}

// ScriptsDir is where the hook scripts for agent a live in projectDir.
func ScriptsDir(projectDir string, a Agent) string {
	return filepath.Join(baseDir(projectDir, a), scriptsSubdir)
}

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	// AdditionalContextLimit is Codex-only (nil for Claude Code, so its stanza
	// is unchanged). Codex moves context over ~2500 tokens to a file unless
	// told otherwise; 0 means never.
	AdditionalContextLimit *int `json:"additionalContextLimit,omitempty"`
	// Timeout (seconds) is Codex-only, set on SessionEnd — see codexHooks.
	Timeout *int `json:"timeout,omitempty"`
}

type hookGroup struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

// hooksObject uses explicit fields (not a map) so json.MarshalIndent emits
// the events in lifecycle order rather than alphabetical map-key order.
type hooksObject struct {
	SessionStart     []hookGroup `json:"SessionStart,omitempty"`
	UserPromptSubmit []hookGroup `json:"UserPromptSubmit,omitempty"`
	PreToolUse       []hookGroup `json:"PreToolUse,omitempty"`
	PreCompact       []hookGroup `json:"PreCompact,omitempty"`
	SessionEnd       []hookGroup `json:"SessionEnd,omitempty"`
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
	return canonicalHooksFor(vaultPath, nil)
}

// federatedScripts are the hooks that SEARCH, and so take the whole vault list.
// The persona loader is not among them: the self is one vault, not a federation.
var federatedScripts = map[string]bool{
	hookUserPromptSubmitScript: true,
	hookReachScript:            true,
	hookCodeMapScript:          true,
}

// canonicalHooksFor is canonicalHooks with an optional federation. With vaults
// set, the searching hooks get VAULTMIND_VAULTS; the primary vault (vaultPath,
// else the first listed) stays VAULTMIND_VAULT for everything else.
func canonicalHooksFor(vaultPath string, vaults []string) []canonicalHook {
	return canonicalHooksWith(vaultPath, vaults, claudeScriptRef)
}

// canonicalHooksWith is canonicalHooksFor with the script reference supplied:
// Claude Code runs `"$CLAUDE_PROJECT_DIR"/.claude/scripts/<s>`, Codex an
// absolute path under .vaultmind/scripts (codex.go).
func canonicalHooksWith(vaultPath string, vaults []string, scriptRef func(string) string) []canonicalHook {
	if vaultPath == "" && len(vaults) > 0 {
		vaultPath = vaults[0]
	}
	cmd := func(script string) hookCommand {
		c := hookCommandWith(scriptRef(script), vaultPath)
		if len(vaults) > 0 && federatedScripts[script] {
			c = "VAULTMIND_VAULTS=" + singleQuote(strings.Join(vaults, ",")) + " " + c
		}
		return hookCommand{Type: "command", Command: c}
	}
	return []canonicalHook{
		{Event: "SessionStart", Script: hookSessionStartScript, Group: hookGroup{Matcher: "startup", Hooks: []hookCommand{cmd(hookSessionStartScript)}}},
		// vault-agnostic health/onboarding nudge — every adopter (persona AND
		// knowledge-vault) gets the "what do I do next?" answer at session
		// start. Additive: coexists with load-persona.sh on SessionStart.
		{Event: "SessionStart", Script: hookHealthScript, Group: hookGroup{Matcher: "startup", Hooks: []hookCommand{cmd(hookHealthScript)}}},
		{Event: "UserPromptSubmit", Script: hookUserPromptSubmitScript, Group: hookGroup{Hooks: []hookCommand{cmd(hookUserPromptSubmitScript)}}},
		{Event: "PreToolUse", Script: hookPreToolUseScript, Group: hookGroup{Matcher: "Read", Hooks: []hookCommand{cmd(hookPreToolUseScript)}}},
		// Pointers AT the reach, not on the turn. An arc that surfaces all day and
		// never at the instant it applies is not surfacing — allowlisted to
		// irreversible or outward-facing commands, because noise is this channel's
		// failure mode, not silence. Shares PreToolUse with read-tracking under a
		// different matcher.
		{Event: "PreToolUse", Script: hookReachScript, Group: hookGroup{Matcher: reachMatcher, Hooks: []hookCommand{cmd(hookReachScript)}}},
		// The knowledge about a file, when the file is opened: notes whose paths:
		// cover it, as a short map. Found by the path the agent is already reading,
		// not by a query it would have to guess.
		{Event: "PreToolUse", Script: hookCodeMapScript, Group: hookGroup{Matcher: codeMapMatcher, Hooks: []hookCommand{cmd(hookCodeMapScript)}}},
		// The WRITE-path trigger. Every other hook is read-path or telemetry;
		// nothing fired at the moment a transformation could be written down, and a
		// desk that depends on remembering collects nothing (measured: two entries
		// in ten weeks). Compaction is the only moment that is all three of: the
		// raw material still exists, it is provably about to be destroyed, and the
		// system knows in advance.
		{Event: "PreCompact", Script: hookPreCompactScript, Group: hookGroup{Hooks: []hookCommand{cmd(hookPreCompactScript)}}},
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
// buildHooksObject maps canonical hooks onto the lifecycle-ordered struct the
// stanza serializes. Separate from SettingsStanza so the unhandled-event guard
// below is reachable in a test: a guard nobody can exercise is a comment.
//
// Append (not assign) per event — an event can carry more than one canonical
// group. SessionStart wires both load-persona.sh and vaultmind-health.sh, and
// PreToolUse wires read-tracking and reach-pointers under different matchers.
func buildHooksObject(chs []canonicalHook) (hooksObject, error) {
	var obj hooksObject
	for _, ch := range chs {
		switch ch.Event {
		case "SessionStart":
			obj.SessionStart = append(obj.SessionStart, ch.Group)
		case "UserPromptSubmit":
			obj.UserPromptSubmit = append(obj.UserPromptSubmit, ch.Group)
		case "PreToolUse":
			obj.PreToolUse = append(obj.PreToolUse, ch.Group)
		case "PreCompact":
			obj.PreCompact = append(obj.PreCompact, ch.Group)
		case "SessionEnd":
			obj.SessionEnd = append(obj.SessionEnd, ch.Group)
		default:
			// A canonical hook whose event has no field here would render as
			// nothing at all — the wiring table would list it, the stanza would
			// omit it, and the hook would simply never fire. Fail loudly:
			// adding an event to canonicalHooks and forgetting the field is a
			// programming error, not a runtime condition. Observed once already,
			// as `"PreCompact": null`.
			return hooksObject{}, fmt.Errorf(
				"canonical hook %q has an unhandled event %q — add it to hooksObject", ch.Script, ch.Event)
		}
	}
	return obj, nil
}

func SettingsStanza(vaultPath string) (string, error) {
	return SettingsStanzaForProfile(ProfileFull, vaultPath)
}

// SettingsStanzaForProfile renders only the hooks the declared profile runs,
// so a knowledge vault is not wired for a persona it does not have.
func SettingsStanzaForProfile(p Profile, vaultPath string) (string, error) {
	allowed := map[string]bool{}
	for _, n := range ScriptsForProfile(p) {
		allowed[n] = true
	}
	hooks := make([]canonicalHook, 0, len(allowed))
	for _, ch := range canonicalHooks(vaultPath) {
		if allowed[ch.Script] {
			hooks = append(hooks, ch)
		}
	}
	return renderStanza(hooks)
}

func renderStanza(hooks []canonicalHook) (string, error) {
	obj, err := buildHooksObject(hooks)
	if err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(settingsStanza{Hooks: obj}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("rendering settings stanza: %w", err)
	}
	return string(out), nil
}

// claudeScriptRef is how Claude Code hooks name a script: through the
// CLAUDE_PROJECT_DIR Claude Code sets for every hook.
func claudeScriptRef(script string) string {
	return `"$CLAUDE_PROJECT_DIR"/` + claudeBaseDir + "/" + scriptsSubdir + "/" + script
}

// hookCommandWith builds the shell command for one hook: run the script at ref.
// A non-empty vaultPath is single-quoted and set as VAULTMIND_VAULT so the
// literal path survives JSON encoding and the shell does not expand it.
func hookCommandWith(ref, vaultPath string) string {
	base := "bash " + ref
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
