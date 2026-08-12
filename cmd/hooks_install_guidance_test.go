package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `hooks install` without --merge writes the scripts, then prints the settings
// stanza for the user to paste. That stanza is ~60 lines of JSON, and the one
// line telling the reader there is a command that does it for them sat at the
// end of the sentence introducing it — after which the JSON scrolls the advice
// off the screen.
//
// --merge is the better path and was verified as safe: non-destructive,
// idempotent, and it preserves a project's pre-existing hooks. It should be
// offered BEFORE the wall of JSON, together with the dry-run that lets a
// cautious user see the change first. The manual stanza stays — a user who
// wants to paste it themselves keeps that option — but hand-merging should be
// the fallback, not the thing you land on because you never saw the shortcut.
func TestHooksInstallHuman_OffersMergeBeforeTheJSONWall(t *testing.T) {
	var buf bytes.Buffer
	res := &hooks.InstallResult{
		ProjectDir:     "/tmp/proj",
		ScriptsDir:     "/tmp/proj/.claude/scripts",
		Written:        []string{"load-persona.sh"},
		SettingsStanza: "{\n  \"hooks\": {}\n}",
	}
	writeHooksInstallHuman(&buf, res, nil)
	out := buf.String()

	mergeAt := strings.Index(out, "--merge")
	stanzaAt := strings.Index(out, res.SettingsStanza)
	require.NotEqual(t, -1, mergeAt, "the one-command path must be mentioned")
	require.NotEqual(t, -1, stanzaAt, "the paste-it-yourself stanza must still be available")
	assert.Less(t, mergeAt, stanzaAt,
		"advice printed after 60 lines of JSON is advice the reader has already scrolled past")

	assert.Contains(t, out, "--dry-run",
		"a user about to let a tool edit their settings.json deserves the preview flag in the same breath")
	assert.Contains(t, out, "existing hooks",
		"say the merge is non-destructive; that is the fear that makes people hand-paste")
}

// When a merge actually ran, the stanza stays suppressed — the wiring is done,
// so re-printing it would only invite a second, manual application.
func TestHooksInstallHuman_MergeOutcomeSuppressesStanza(t *testing.T) {
	var buf bytes.Buffer
	res := &hooks.InstallResult{
		ProjectDir:     "/tmp/proj",
		ScriptsDir:     "/tmp/proj/.claude/scripts",
		SettingsStanza: "{\n  \"hooks\": {}\n}",
	}
	writeHooksInstallHuman(&buf, res, &hooks.MergeFileResult{
		SettingsPath: "/tmp/proj/.claude/settings.json",
		Changed:      true,
	})
	out := buf.String()

	assert.NotContains(t, out, res.SettingsStanza)
	assert.Contains(t, out, "existing hooks preserved")
}
