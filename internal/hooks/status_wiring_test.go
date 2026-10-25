package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatus_ProjectSettingsUnion(t *testing.T) {
	group := func(matcher string) string {
		return `{"matcher":"` + matcher + `","hooks":[{"command":"bash .claude/scripts/vault-reach.sh"}]}`
	}
	different := func(matcher string) string {
		return strings.Replace(group(matcher), "bash ", "sh ", 1)
	}
	for _, tc := range []struct {
		name, shared, local string
		state               EventState
		files               []string
		warning, note       bool
	}{
		{"local only", "", group(reachMatcher), EventWired, []string{settingsLocalName}, false, false},
		{"shared only", group(reachMatcher), "", EventWired, []string{settingsFileName}, false, false},
		{"neither", "", "", EventUnwired, nil, false, false},
		{"both", group(reachMatcher), group(reachMatcher), EventWired, []string{settingsFileName, settingsLocalName}, false, true},
		{"local stale", "", group("Bash"), EventStaleMatcher, []string{settingsLocalName}, false, false},
		{"stale in both", group("Bash"), group("Bash"), EventStaleMatcher, []string{settingsFileName, settingsLocalName}, false, true},
		{"stale shared canonical local", group("Bash"), group(reachMatcher), EventStaleMatcher, []string{settingsFileName, settingsLocalName}, false, true},
		{"canonical shared stale local", group(reachMatcher), group("Bash"), EventStaleMatcher, []string{settingsFileName, settingsLocalName}, false, true},
		{"local hand wired", "", group("Bash") + "," + group("Edit"), EventWired, []string{settingsLocalName}, false, false},
		{"hand wired across files", group("Bash"), group("Edit"), EventStaleMatcher, []string{settingsFileName, settingsLocalName}, false, false},
		{"different overlapping commands", group(reachMatcher), different("Edit|Read"), EventWired, []string{settingsFileName, settingsLocalName}, true, false},
		{"different disjoint commands", group("Read"), different("Edit"), EventWired, []string{settingsFileName, settingsLocalName}, false, false},
		{"wildcard", group("*"), different("Edit"), EventWired, []string{settingsFileName, settingsLocalName}, true, false},
		{"omitted matcher", `{"hooks":[{"command":"bash .claude/scripts/vault-reach.sh"}]}`, different("Edit"), EventWired, []string{settingsFileName, settingsLocalName}, true, false},
		{"comma list", group("Read, Edit"), different("Edit"), EventWired, []string{settingsFileName, settingsLocalName}, true, false},
		{"regex overlaps exact", group("^Ed.*"), different("Edit"), EventWired, []string{settingsFileName, settingsLocalName}, true, false},
		{"regex disjoint from exact", group("^Read$"), different("Edit"), EventWired, []string{settingsFileName, settingsLocalName}, false, false},
		{"all commands in a group checked", strings.Replace(group("Edit"), `"}]}`, `"},{"command":"sh .claude/scripts/vault-reach.sh"}]}`, 1), group("Edit"), EventWired, []string{settingsFileName, settingsLocalName}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(project, ".claude", "scripts"), 0o750))
			require.NoError(t, os.WriteFile(filepath.Join(project, ".claude", "scripts", "capture-episode.sh"), []byte("#!/bin/bash\n"), 0o700))
			for local, groups := range map[bool]string{false: tc.shared, true: tc.local} {
				if groups != "" {
					require.NoError(t, os.WriteFile(settingsFilePath(project, local), []byte(`{"hooks":{"PreToolUse":[`+groups+`]}}`), 0o600))
				}
			}
			report, err := Status(project)
			require.NoError(t, err)
			require.Len(t, report.Events, len(CanonicalEventScripts()))
			byScript := map[string]EventStatus{}
			for _, e := range report.Events {
				byScript[e.Script] = e
			}
			require.Contains(t, byScript, "capture-episode.sh")
			assert.Equal(t, EventUnwired, byScript["capture-episode.sh"].State, "a script on disk still needs event wiring")
			require.Contains(t, byScript, "vault-reach.sh")
			e := byScript["vault-reach.sh"]
			assert.Equal(t, tc.state, e.State)
			body, err := json.Marshal(e)
			require.NoError(t, err)
			var details struct {
				Files    []string `json:"files"`
				Warnings []string `json:"warnings"`
				Notes    []string `json:"notes"`
			}
			require.NoError(t, json.Unmarshal(body, &details))
			assert.Equal(t, tc.files, details.Files)
			if tc.warning {
				require.Len(t, details.Warnings, 1)
				assert.Contains(t, details.Warnings[0], "hooks can fire twice")
			} else {
				assert.Empty(t, details.Warnings)
			}
			if tc.note {
				require.Len(t, details.Notes, 1)
				assert.Contains(t, details.Notes[0], "wired in both settings files; Claude Code runs it once — keep one")
			} else {
				assert.Empty(t, details.Notes)
			}
		})
	}
}

func TestStatus_MalformedSettingsRemainSymmetric(t *testing.T) {
	for _, local := range []bool{false, true} {
		project := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(project, ".claude"), 0o750))
		require.NoError(t, os.WriteFile(settingsFilePath(project, local), []byte(`{"hooks":`), 0o600))
		report, err := Status(project)
		require.NoError(t, err, "the internal status check historically ignores malformed settings")
		wired, unwired := report.EventCounts()
		assert.Zero(t, wired)
		assert.Equal(t, len(CanonicalEventScripts()), unwired)
	}
}

func TestStatus_NonToolDuplicateCommands(t *testing.T) {
	for _, different := range []bool{false, true} {
		t.Run(map[bool]string{false: "identical", true: "different"}[different], func(t *testing.T) {
			project := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(project, ".claude"), 0o750))
			for _, local := range []bool{false, true} {
				command := "bash .claude/scripts/vault-recall.sh"
				if local && different {
					command = "sh .claude/scripts/vault-recall.sh"
				}
				matcher := ""
				if local {
					matcher = `"matcher":"",`
				}
				require.NoError(t, os.WriteFile(settingsFilePath(project, local), []byte(`{"hooks":{"UserPromptSubmit":[{`+matcher+`"hooks":[{"command":"`+command+`"}]}]}}`), 0o600))
			}
			byScript := map[string]EventStatus{}
			for _, e := range eventWiringForProfile(project, ProfileFull) {
				byScript[e.Script] = e
			}
			require.Contains(t, byScript, hookUserPromptSubmitScript)
			e := byScript[hookUserPromptSubmitScript]
			assert.Equal(t, EventWired, e.State)
			if different {
				require.Len(t, e.Warnings, 1)
				assert.Contains(t, e.Warnings[0], "hooks can fire twice")
			} else {
				assert.Empty(t, e.Warnings)
			}
		})
	}
}

func TestStatus_UserScopeIgnoresLocalSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	_, err := MergeIntoSettings(home, "", true, false)
	require.NoError(t, err)
	report, err := StatusForScope(home, ScopeUser)
	require.NoError(t, err)
	wired, _ := report.EventCounts()
	assert.Zero(t, wired)
	assert.False(t, UserScopeInstalled(home))
}

// TestStatus_ReportsCanonicalEventsThatAreNotWired closes the same gap this
// command was built to close, one layer up.
//
// hooks status exists because doctor reported DRIFT but nothing reported
// ABSENCE: a hook the binary ships and a project never installed rendered as
// nothing, which looks exactly like health. It then compared script CONTENTS
// only — so a canonical event wired to no script at all stayed invisible for
// precisely the same reason.
//
// Not hypothetical. An adopter had three read-side hooks wired and SessionEnd
// absent; capture-episode.sh was present on disk and had produced 13 episodes,
// so every content-based check passed while the write half was switched off. The
// same project ran --pointers-only for a whole release cycle behind this class
// of blind spot.
//
// The canonical event→script map already lives in settings.go, where install
// uses it. Status simply never read it.
func TestStatus_ReportsCanonicalEventsThatAreNotWired(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".claude", "scripts"), 0o755))

	// The adopter's real shape, plus one case that is load-bearing for the
	// matcher itself: SessionStart is PRESENT in settings but runs somebody
	// else's script. Without that row this test cannot fail — for an event with
	// no entries the loop body never executes, so the script comparison is never
	// reached and a matcher that always says "wired" survives untouched.
	// Verified by mutation: strings.Contains(h.Command, "") passed the earlier
	// version of this test.
	settings := `{
	  "hooks": {
	    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "$CLAUDE_PROJECT_DIR/.claude/scripts/vault-recall.sh"}]}],
	    "SessionStart": [{"hooks": [{"type": "command", "command": "$CLAUDE_PROJECT_DIR/.claude/scripts/some-other-tool.sh"}]}]
	  }
	}`
	require.NoError(t, os.WriteFile(
		filepath.Join(project, ".claude", "settings.json"), []byte(settings), 0o600))

	report, err := Status(project)
	require.NoError(t, err)

	byEvent := map[string]EventState{}
	for _, e := range report.Events {
		byEvent[e.Event+"/"+e.Script] = e.State
	}
	require.NotEmpty(t, report.Events, "status must report event wiring, not only script contents")

	require.Equal(t, EventWired, byEvent["UserPromptSubmit/vault-recall.sh"],
		"this one IS wired")
	require.Equal(t, EventUnwired, byEvent["SessionEnd/capture-episode.sh"],
		"the write half is switched off and must be named")
	require.Equal(t, EventUnwired, byEvent["PreCompact/precompact-preserve.sh"],
		"the other write-path trigger, likewise")
	require.Equal(t, EventUnwired, byEvent["SessionStart/load-persona.sh"],
		"the event is wired, but to another tool's script — that is not our hook running")

	_, unwired := report.EventCounts()
	require.Positive(t, unwired, "an unwired canonical event must be counted, so it can gate")
}
