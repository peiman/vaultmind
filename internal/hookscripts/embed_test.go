package hookscripts_test

import (
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The canonical Claude Code hook scripts are the SSOT for
// VaultMind's integration. Any consuming agent
// running `vaultmind hooks install` writes copies of these
// embedded files. If any embed regresses (file deleted, package
// renamed, embed pattern broken), every consumer's install
// breaks silently. Tests pin the contract.
//
// Original 5 (persona / recall / read-tracking / read-blocking /
// episode-capture). Auto-RAG additions 2026-05-07 (slice B of
// companion-project handoff absorption): auto-rag-guard.sh + shell-strip.sh
// + auto-rag-evaluate.sh.

// TestAll_EmbedsAllCanonicalHookScripts — pin every canonical
// hook script is embedded under its expected name. Adding a new
// script to internal/hookscripts/ should also extend this list,
// or the script becomes orphaned ceremony (no install path).
func TestAll_EmbedsAllCanonicalHookScripts(t *testing.T) {
	scripts := hookscripts.All()
	for _, name := range []string{
		// Original persona / lifecycle hooks:
		"load-persona.sh",
		"vault-recall.sh",
		"vault-track-read.sh",
		"vault-block-read.sh",
		"capture-episode.sh",
		// Auto-RAG framework (2026-05-07 absorption from the companion project v0.3):
		"auto-rag-guard.sh",
		"auto-rag-evaluate.sh",
		"shell-strip.sh",
	} {
		body, ok := scripts[name]
		assert.True(t, ok, "embed must include %q", name)
		assert.Greater(t, len(body), 200,
			"%q under 200 bytes suggests a stub or empty file got embedded", name)
		assert.True(t, strings.HasPrefix(string(body), "#!"),
			"%q should start with shebang — bash hook script", name)
	}
}

// TestNames_ReturnsSortedDeterministicOrder — `Names()` returns
// the same order every call. Doctor's drift report and hooks
// install's per-file output rely on stable iteration; without
// this, output diff-noise would obscure real changes.
func TestNames_ReturnsSortedDeterministicOrder(t *testing.T) {
	first := hookscripts.Names()
	second := hookscripts.Names()
	assert.Equal(t, first, second)
	require.Greater(t, len(first), 0)
	for i := 1; i < len(first); i++ {
		assert.LessOrEqual(t, first[i-1], first[i],
			"Names() must return sorted output")
	}
}

// TestLoadPersonaScript_HasVaultPathOverride — companion project 2026-05-07
// HIGH-2: load-persona.sh hardcoded `vaultmind-identity` as the
// vault dir, silently producing empty persona for any consumer
// (any consumer project) whose vault has a different name.
// Pin the env-var override contract.
func TestLoadPersonaScript_HasVaultPathOverride(t *testing.T) {
	body, ok := hookscripts.Get("load-persona.sh")
	require.True(t, ok)
	src := string(body)
	assert.Contains(t, src, `${LOAD_PERSONA_VAULT:-`,
		"load-persona.sh must support LOAD_PERSONA_VAULT env-var override per companion project 2026-05-07 HIGH-2")
	assert.Contains(t, src, `${LOAD_PERSONA_RESEARCH_VAULT:-`,
		"load-persona.sh must support LOAD_PERSONA_RESEARCH_VAULT for the optional research-vault second-query path")
}

// TestVaultTrackReadScript_HasVaultPathPatternOverride — companion project
// 2026-05-07 HIGH-1: vault-track-read.sh's `*/vaultmind-*/*.md`
// glob silently filtered out reads on `companion-vault/*.md`,
// turning the read-tracking hook inert. Pin the env-var override
// (explicit VAULT_PATH_PATTERN still wins) and the legacy default
// fallback (`*/vaultmind-*/*.md`) so neither regresses when the
// VAULTMIND_VAULT-derived pattern was added (issue #41.6).
func TestVaultTrackReadScript_HasVaultPathPatternOverride(t *testing.T) {
	body, ok := hookscripts.Get("vault-track-read.sh")
	require.True(t, ok)
	src := string(body)
	assert.Contains(t, src, `VAULT_PATH_PATTERN`,
		"vault-track-read.sh must support VAULT_PATH_PATTERN env-var override per companion project 2026-05-07 HIGH-1")
	assert.Contains(t, src, `*/vaultmind-*/*.md`,
		"vault-track-read.sh must keep the legacy default pattern as the final fallback")
}

// TestVaultTrackReadScript_GuardsOptionalVarsUnderSetU — the script runs under
// `set -u`, where referencing an *unset* variable aborts with "unbound variable".
// VAULT_PATH_PATTERN and VAULTMIND_VAULT are optional (the PreToolUse command
// doesn't set them in the common case), so they MUST be `${VAR:-}`-guarded.
// Regression: a bare `[ -n "$VAULT_PATH_PATTERN" ]` made the hook abort on every
// vault Read with "VAULT_PATH_PATTERN: unbound variable" (field report 2026-06-04).
func TestVaultTrackReadScript_GuardsOptionalVarsUnderSetU(t *testing.T) {
	body, ok := hookscripts.Get("vault-track-read.sh")
	require.True(t, ok)
	src := string(body)
	require.Contains(t, src, "set -u", "this guard only matters under set -u")
	assert.Contains(t, src, `${VAULT_PATH_PATTERN:-}`,
		"VAULT_PATH_PATTERN must be ${VAR:-}-guarded so set -u doesn't abort when it's unset")
	assert.Contains(t, src, `${VAULTMIND_VAULT:-}`,
		"VAULTMIND_VAULT must be ${VAR:-}-guarded under set -u")
	assert.NotContains(t, src, `[ -n "$VAULT_PATH_PATTERN" ]`,
		"the bare unguarded reference is the bug — it aborts under set -u when unset")
}

// TestScripts_HonorVaultmindVaultOverride — issue #41.6: vault-recall.sh
// and capture-episode.sh hardcoded `vaultmind-identity` with no override,
// so a consumer whose vault has a different name had to rewrite the
// scripts by hand. `hooks install --vault` bakes VAULTMIND_VAULT into the
// emitted settings.json stanza; these scripts must honor it. load-persona.sh
// keeps LOAD_PERSONA_VAULT as the highest-precedence override and falls back
// to VAULTMIND_VAULT so a single var drives every hook.
func TestScripts_HonorVaultmindVaultOverride(t *testing.T) {
	recall, ok := hookscripts.Get("vault-recall.sh")
	require.True(t, ok)
	assert.Contains(t, string(recall), `${VAULTMIND_VAULT:-`,
		"vault-recall.sh must honor VAULTMIND_VAULT (issue #41.6)")

	episode, ok := hookscripts.Get("capture-episode.sh")
	require.True(t, ok)
	assert.Contains(t, string(episode), `${VAULTMIND_VAULT:-`,
		"capture-episode.sh must honor VAULTMIND_VAULT (issue #41.6)")

	persona, ok := hookscripts.Get("load-persona.sh")
	require.True(t, ok)
	assert.Contains(t, string(persona), `VAULTMIND_VAULT`,
		"load-persona.sh must fall back to VAULTMIND_VAULT so one var drives every hook")

	// vault-track-read.sh must derive its match pattern from VAULTMIND_VAULT
	// when set, else read-tracking is silently inert for a consumer vault
	// whose name isn't `vaultmind-*` (issue #41.6 HIGH — the stanza bakes
	// VAULTMIND_VAULT into the PreToolUse command, so the script must honor it).
	track, ok := hookscripts.Get("vault-track-read.sh")
	require.True(t, ok)
	assert.Contains(t, string(track), `VAULTMIND_VAULT`,
		"vault-track-read.sh must honor VAULTMIND_VAULT so --vault repoints read-tracking too (issue #41.6)")
}

// TestVaultRecallScript_UsesRelevanceFloor — the per-prompt recall hook must
// pass --quiet-on-no-match so off-domain prompts inject silence instead of
// irrelevant pointers (the noise an agent feels every turn otherwise).
func TestVaultRecallScript_UsesRelevanceFloor(t *testing.T) {
	body, ok := hookscripts.Get("vault-recall.sh")
	require.True(t, ok)
	assert.Contains(t, string(body), "--quiet-on-no-match",
		"vault-recall.sh must gate injection on the noise floor so off-domain prompts stay silent")
}

// TestGet_ExactMatchOnly — Get treats the input as a base
// filename, not a path. No traversal, no globbing. Pins the
// security-shaped contract: an attacker can't `Get("../../etc/passwd")`
// and have it resolve.
func TestGet_ExactMatchOnly(t *testing.T) {
	body, ok := hookscripts.Get("load-persona.sh")
	require.True(t, ok)
	require.NotEmpty(t, body)

	for _, bad := range []string{
		"",
		"load-persona",                // no extension
		"./load-persona.sh",           // relative path prefix
		"../load-persona.sh",          // traversal
		"hookscripts/load-persona.sh", // package-qualified
		"nonexistent.sh",              // not embedded
	} {
		_, ok := hookscripts.Get(bad)
		assert.False(t, ok, "Get(%q) must return ok=false", bad)
	}
}
