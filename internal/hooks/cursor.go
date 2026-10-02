package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Cursor support — the same scripts, run through cursor-adapter.sh.
//
// Probed live with cursor-agent 2026.10.01 (2026-10-02), against Cursor's docs:
//
//   - .cursor/hooks.json is {"version": 1, "hooks": {event: [{command,
//     timeout}]}} — flat entries, no matcher groups; project hooks run from
//     the project root.
//   - Only sessionStart and postToolUse add context, and every hook's context
//     on an event is kept. beforeSubmitPrompt cannot add context, so there is
//     no per-prompt recall; preCompact and sessionEnd cannot either.
//   - Tools arrive after they ran ("postToolUse"): the code map follows a
//     read or an edit, and reach follows the commit rather than preceding it.
//
// Scripts live under .vaultmind/scripts, shared with Codex; the adapter is
// installed only for Cursor.

const (
	cursorDir           = ".cursor"
	cursorHooksFile     = "hooks.json"
	cursorHooksVersion  = 1
	cursorAdapterScript = "cursor-adapter.sh"
	// cursorHookTimeout bounds each hook (seconds): a hook that hangs must not
	// hang the agent. The searching scripts carry their own shorter budgets.
	cursorHookTimeout = 30
)

// cursorWiring is the Cursor event and the tool pattern for each script that
// can deliver under Cursor. A script absent here is not wired.
var cursorWiring = map[string]struct{ event, tools string }{
	hookSessionStartScript: {"sessionStart", ""},
	hookHealthScript:       {"sessionStart", ""},
	hookPreToolUseScript:   {"postToolUse", "Read"},
	hookReachScript:        {"postToolUse", "Shell|Write"},
	hookCodeMapScript:      {"postToolUse", "Read|Write"},
}

// cursorEntry is one Cursor hook: the event, the script it runs (the merge
// key), and the entry written to hooks.json.
type cursorEntry struct {
	Event  string
	Script string
	Hook   cursorHookEntry
}

type cursorHookEntry struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// cursorHooks derives Cursor's wiring from the canonical hooks (one source of
// truth for scripts and vault settings), filtered to what Cursor can deliver
// and to the profile.
func cursorHooks(projectDir, vaultPath string, vaults []string, p Profile) []cursorEntry {
	allowed := map[string]bool{}
	for _, n := range ScriptsForProfile(p) {
		allowed[n] = true
	}
	adapter := singleQuote(filepath.Join(ScriptsDir(projectDir, AgentCursor), cursorAdapterScript))
	ref := func(script string) string {
		ref := adapter + " " + script
		if w := cursorWiring[script]; w.tools != "" {
			ref += " " + singleQuote(w.tools)
		}
		return ref
	}
	prefix := "export " + envProjectDir + "=" + singleQuote(projectDir) + "; "
	var out []cursorEntry
	for _, ch := range canonicalHooksWith(vaultPath, vaults, ref) {
		w, ok := cursorWiring[ch.Script]
		if !ok || !allowed[ch.Script] {
			continue
		}
		for _, h := range ch.Group.Hooks {
			out = append(out, cursorEntry{Event: w.event, Script: ch.Script,
				Hook: cursorHookEntry{Command: prefix + h.Command, Timeout: cursorHookTimeout}})
		}
	}
	return out
}

// CursorHooksStanza renders .cursor/hooks.json for projectDir.
func CursorHooksStanza(projectDir, vaultPath string, p Profile) (string, error) {
	return CursorHooksStanzaFor(projectDir, vaultPath, nil, p)
}

// CursorHooksStanzaFor is CursorHooksStanza with an optional federation.
func CursorHooksStanzaFor(projectDir, vaultPath string, vaults []string, p Profile) (string, error) {
	b, err := mergeCursorHooks(nil, cursorHooks(projectDir, vaultPath, vaults, p))
	return string(b), err
}

// CursorHooksPath is where the Cursor wiring for projectDir lives.
func CursorHooksPath(projectDir string) string {
	return filepath.Join(projectDir, cursorDir, cursorHooksFile)
}

// cursorAdapterRef is how every command we write names the adapter: the
// quoted absolute path ends in it.
const cursorAdapterRef = "/" + cursorAdapterScript + "'"

// isOurCursorHook reports whether a hooks.json entry runs our adapter.
func isOurCursorHook(raw json.RawMessage) bool {
	var e cursorHookEntry
	if json.Unmarshal(raw, &e) != nil {
		return false
	}
	return strings.Contains(e.Command, cursorAdapterRef)
}

// cursorDoc is hooks.json with every key we do not own kept as it was.
type cursorDoc struct {
	top   map[string]json.RawMessage
	hooks map[string][]json.RawMessage
}

func parseCursorDoc(existing []byte) (*cursorDoc, error) {
	d := &cursorDoc{top: map[string]json.RawMessage{}, hooks: map[string][]json.RawMessage{}}
	if len(bytes.TrimSpace(existing)) == 0 {
		return d, nil
	}
	if err := json.Unmarshal(existing, &d.top); err != nil {
		return nil, fmt.Errorf("not a JSON object: %w", err)
	}
	if raw, ok := d.top["hooks"]; ok {
		if err := json.Unmarshal(raw, &d.hooks); err != nil {
			return nil, fmt.Errorf(`"hooks" is not an object of event arrays: %w`, err)
		}
	}
	return d, nil
}

// withoutOurs drops our entries from every event, and events left empty.
func (d *cursorDoc) withoutOurs() []string {
	var removed []string
	for event, entries := range d.hooks {
		kept := entries[:0:0]
		for _, e := range entries {
			if isOurCursorHook(e) {
				removed = append(removed, event)
				continue
			}
			kept = append(kept, e)
		}
		if len(kept) == 0 {
			delete(d.hooks, event)
		} else {
			d.hooks[event] = kept
		}
	}
	sort.Strings(removed)
	return removed
}

func (d *cursorDoc) render() ([]byte, error) {
	if _, ok := d.top["version"]; !ok {
		d.top["version"] = json.RawMessage(fmt.Sprint(cursorHooksVersion))
	}
	hooks, err := json.Marshal(d.hooks)
	if err != nil {
		return nil, err
	}
	d.top["hooks"] = hooks
	out, err := json.MarshalIndent(d.top, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// mergeCursorHooks returns existing with our entries replaced by want: the
// user's entries keep their place and content, ours follow them on each event.
func mergeCursorHooks(existing []byte, want []cursorEntry) ([]byte, error) {
	d, err := parseCursorDoc(existing)
	if err != nil {
		return nil, err
	}
	d.withoutOurs()
	for _, e := range want {
		raw, err := json.Marshal(e.Hook)
		if err != nil {
			return nil, err
		}
		d.hooks[e.Event] = append(d.hooks[e.Event], raw)
	}
	return d.render()
}

// sameCursorDoc reports whether a and b hold the same JSON, whatever the
// formatting: a re-run must not rewrite a file whose content is unchanged.
func sameCursorDoc(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return bytes.Equal(xb, yb)
}

// MergeIntoCursorHooks merges VaultMind's Cursor wiring into
// <projectDir>/.cursor/hooks.json: the user's hooks kept, our own stale
// entries replaced, re-runs changing nothing, a malformed file refused.
func MergeIntoCursorHooks(projectDir, vaultPath string, p Profile, dryRun bool) (*MergeFileResult, error) {
	return MergeIntoCursorHooksFor(projectDir, vaultPath, nil, p, dryRun)
}

// MergeIntoCursorHooksFor is MergeIntoCursorHooks with an optional federation.
func MergeIntoCursorHooksFor(projectDir, vaultPath string, vaults []string, p Profile, dryRun bool) (*MergeFileResult, error) {
	path := CursorHooksPath(projectDir)
	existing, err := readSettingsFile(path)
	if err != nil {
		return nil, err
	}
	merged, err := mergeCursorHooks(existing, cursorHooks(projectDir, vaultPath, vaults, p))
	if err != nil {
		return nil, fmt.Errorf("merging into %s: %w", path, err)
	}
	changed := existing == nil || !sameCursorDoc(existing, merged)
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

// RemoveFromCursorHooks strips VaultMind's entries from .cursor/hooks.json,
// keeping the user's, and with removeScripts deletes the scripts from
// .vaultmind/scripts unless Codex still runs them.
func RemoveFromCursorHooks(projectDir string, removeScripts bool) (*RemoveFileResult, error) {
	path := CursorHooksPath(projectDir)
	res := &RemoveFileResult{SettingsPath: path}
	existing, err := readSettingsFile(path)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		d, err := parseCursorDoc(existing)
		if err != nil {
			return nil, fmt.Errorf("removing from %s: %w", path, err)
		}
		res.Removed = d.withoutOurs()
		res.Changed = len(res.Removed) > 0
		if res.Changed {
			out, err := d.render()
			if err != nil {
				return nil, err
			}
			if err := atomicWriteFile(path, out, 0o600); err != nil {
				return nil, fmt.Errorf("writing %s: %w", path, err)
			}
		}
	}
	if removeScripts {
		if err := removeSharedScripts(projectDir, AgentCursor, res); err != nil {
			return res, err
		}
	}
	return res, nil
}

// removeSharedScripts deletes .vaultmind/scripts, the folder Codex and Cursor
// share, unless the other agent's hooks still run from it: then the scripts
// stay, named in ScriptsKept, so removing one agent never breaks the other.
func removeSharedScripts(projectDir string, removing Agent, res *RemoveFileResult) error {
	dir := ScriptsDir(projectDir, removing)
	if otherAgentRunsScripts(projectDir, removing) {
		res.ScriptsKept = append(res.ScriptsKept, installedScripts(dir)...)
		return nil
	}
	deleted, err := deleteInstalledScripts(dir)
	res.ScriptsDeleted = deleted
	return err
}

// otherAgentRunsScripts reports whether the hooks of the agent sharing
// .vaultmind/scripts with removing still run them.
func otherAgentRunsScripts(projectDir string, removing Agent) bool {
	var path, marker string
	switch removing {
	case AgentCodex:
		path, marker = CursorHooksPath(projectDir), cursorAdapterRef
	case AgentCursor:
		path, marker = CodexHooksPath(projectDir), ScriptsDir(projectDir, AgentCodex)
	default:
		return false
	}
	b, err := readSettingsFile(path)
	return err == nil && bytes.Contains(b, []byte(marker))
}

// CursorWiring is what status reports for Cursor: the scripts its hooks.json
// runs and the state of each copy, so a Cursor project is judged on what it
// runs — not on Claude Code scripts it never had.
type CursorWiring struct {
	HooksPath  string         `json:"hooks_path"`
	ScriptsDir string         `json:"scripts_dir"`
	Wired      []string       `json:"wired"`
	Scripts    []ScriptStatus `json:"scripts"`
}

// ScriptCounts returns how many of Cursor's scripts are in each state.
func (c CursorWiring) ScriptCounts() (inSync, drifted, missing int) {
	return countScripts(c.Scripts)
}

// cursorWiringFor reads which scripts our .cursor/hooks.json entries run, and
// their state; nil when the project has none of ours.
func cursorWiringFor(projectDir string) (*CursorWiring, error) {
	path := CursorHooksPath(projectDir)
	existing, err := readSettingsFile(path)
	if err != nil || existing == nil {
		return nil, err
	}
	d, err := parseCursorDoc(existing)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	seen := map[string]bool{}
	var wired []string
	for _, entries := range d.hooks {
		for _, raw := range entries {
			if s := cursorScriptOf(raw); s != "" && !seen[s] {
				seen[s] = true
				wired = append(wired, s)
			}
		}
	}
	if len(wired) == 0 {
		return nil, nil //nolint:nilnil // no Cursor wiring of ours is not an error
	}
	sort.Strings(wired)
	c := &CursorWiring{HooksPath: path, ScriptsDir: ScriptsDir(projectDir, AgentCursor), Wired: wired}
	c.Scripts, err = scriptStatuses(c.ScriptsDir, append(append([]string(nil), wired...), cursorAdapterScript))
	return c, err
}

// cursorScriptOf is the script one of our entries runs: the word after the
// adapter's path. "" for an entry that is not ours.
func cursorScriptOf(raw json.RawMessage) string {
	var e cursorHookEntry
	if json.Unmarshal(raw, &e) != nil {
		return ""
	}
	_, rest, ok := strings.Cut(e.Command, cursorAdapterRef)
	if !ok {
		return ""
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// installedScripts names the canonical scripts present in dir.
func installedScripts(dir string) []string {
	var out []string
	for _, name := range allScriptNames() {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			out = append(out, name)
		}
	}
	return out
}
