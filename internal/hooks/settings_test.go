package hooks_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The settings stanza is the connective tissue that turns "install scripts +
// hand-wire settings.json by reading the docs" into one command (issue #41).
// It is code-owned: this is what `hooks install` emits. These tests pin the
// contract (wiring, vault parameterization, shell quoting).

func TestSettingsStanza_WiresTheCanonicalHooks(t *testing.T) {
	stanza, err := hooks.SettingsStanza("")
	require.NoError(t, err)

	// Valid JSON with a top-level "hooks" object.
	var parsed struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal([]byte(stanza), &parsed), "stanza must be valid JSON")

	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "SessionEnd"} {
		assert.Contains(t, parsed.Hooks, event, "stanza must wire the %s hook", event)
	}
	// Each canonical script is referenced (vaultmind-health.sh is the
	// vault-agnostic SessionStart onboarding nudge — focalc field report P0).
	for _, script := range []string{"load-persona.sh", "vaultmind-health.sh", "vault-recall.sh", "vault-track-read.sh", "capture-episode.sh"} {
		assert.Contains(t, stanza, script, "stanza must reference %s", script)
	}
	// PreToolUse is matched on Read; SessionStart on startup.
	assert.Contains(t, stanza, `"Read"`, "PreToolUse must match Read")
	assert.Contains(t, stanza, `"startup"`, "SessionStart must match startup")
}

// SessionStart carries TWO canonical groups: the persona loader and the
// vault-agnostic health nudge. The stanza must render both (not overwrite).
func TestSettingsStanza_SessionStartWiresPersonaAndHealth(t *testing.T) {
	stanza, err := hooks.SettingsStanza("")
	require.NoError(t, err)

	var parsed struct {
		Hooks struct {
			SessionStart []struct {
				Hooks []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"SessionStart"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal([]byte(stanza), &parsed))

	var sawPersona, sawHealth bool
	for _, g := range parsed.Hooks.SessionStart {
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "load-persona.sh") {
				sawPersona = true
			}
			if strings.Contains(h.Command, "vaultmind-health.sh") {
				sawHealth = true
			}
		}
	}
	assert.True(t, sawPersona, "SessionStart must still wire load-persona.sh")
	assert.True(t, sawHealth, "SessionStart must wire vaultmind-health.sh (P0 onboarding nudge)")
}

func TestSettingsStanza_NoVaultPathLeavesCommandsUnparameterized(t *testing.T) {
	stanza, err := hooks.SettingsStanza("")
	require.NoError(t, err)
	// Without a vault path, no VAULTMIND_VAULT assignment is baked in —
	// the scripts fall back to their default ($CLAUDE_PROJECT_DIR/vaultmind-identity).
	assert.NotContains(t, stanza, "VAULTMIND_VAULT=",
		"unparameterized stanza must not bake a vault path")
	assert.Contains(t, stanza, `$CLAUDE_PROJECT_DIR`, "commands resolve scripts via $CLAUDE_PROJECT_DIR")
}

func TestSettingsStanza_VaultPathBakedIntoEveryCommand(t *testing.T) {
	const vaultPath = "/home/me/proj/my-knowledge"
	stanza, err := hooks.SettingsStanza(vaultPath)
	require.NoError(t, err)

	// Every hook command must export VAULTMIND_VAULT so recall/track/episode/
	// persona all point at the consumer's vault, not the default name (#41.6:
	// "vault-recall.sh hardcodes vaultmind-identity; I had to rewrite all four").
	var parsed struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal([]byte(stanza), &parsed))

	commandCount := 0
	for event, groups := range parsed.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				commandCount++
				assert.Truef(t, strings.Contains(h.Command, "VAULTMIND_VAULT="),
					"%s command must export VAULTMIND_VAULT: %q", event, h.Command)
				assert.Containsf(t, h.Command, vaultPath,
					"%s command must contain the vault path: %q", event, h.Command)
			}
		}
	}
	assert.Equal(t, 5, commandCount, "five hooks wired (persona + health on SessionStart, plus recall/track/episode)")
}

func TestSettingsStanza_VaultPathIsShellQuoted(t *testing.T) {
	// A vault path with a space must be quoted so the shell assignment is safe.
	// Single quotes (not double) so the literal path survives JSON encoding
	// without backslash-escaping and so the shell does not expand it.
	stanza, err := hooks.SettingsStanza("/home/me/my vault")
	require.NoError(t, err)
	assert.Contains(t, stanza, `VAULTMIND_VAULT='/home/me/my vault'`,
		"a vault path with spaces must be single-quoted in the assignment")
}

func TestSettingsStanza_VaultPathWithSingleQuoteIsEscaped(t *testing.T) {
	// A path containing a single quote must use the '\'' shell idiom so the
	// assignment stays valid. Assert against the PARSED command (the real
	// shell string) — the raw JSON doubles the backslash via its own escaping.
	stanza, err := hooks.SettingsStanza("/home/me/it's-a-vault")
	require.NoError(t, err)

	var parsed struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal([]byte(stanza), &parsed), "stanza must stay valid JSON")

	cmd := parsed.Hooks["UserPromptSubmit"][0].Hooks[0].Command
	assert.Contains(t, cmd, `VAULTMIND_VAULT='/home/me/it'\''s-a-vault'`,
		"embedded single quote must be escaped with the '\\'' shell idiom in the actual command")
}
