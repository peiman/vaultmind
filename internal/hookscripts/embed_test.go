package hookscripts_test

import (
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hookscripts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The canonical Claude Code hook scripts are the SSOT for
// VaultMind's integration. Any agent (focalc, Siavoush, workhorse)
// running `vaultmind hooks install` writes copies of these
// embedded files. If any embed regresses (file deleted, package
// renamed, embed pattern broken), every consumer's install
// breaks silently. Tests pin the contract.
//
// Original 5 (persona / recall / read-tracking / read-blocking /
// episode-capture). Auto-RAG additions 2026-05-07 (slice B of
// workhorse-handoff absorption): auto-rag-guard.sh + shell-strip.sh
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
		// Auto-RAG framework (2026-05-07 absorption from workhorse v0.3):
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

// TestLoadPersonaScript_HasVaultPathOverride — workhorse 2026-05-07
// HIGH-2: load-persona.sh hardcoded `vaultmind-identity` as the
// vault dir, silently producing empty persona for any consumer
// (workhorse, focalc, etc.) whose vault has a different name.
// Pin the env-var override contract.
func TestLoadPersonaScript_HasVaultPathOverride(t *testing.T) {
	body, ok := hookscripts.Get("load-persona.sh")
	require.True(t, ok)
	src := string(body)
	assert.Contains(t, src, `${LOAD_PERSONA_VAULT:-`,
		"load-persona.sh must support LOAD_PERSONA_VAULT env-var override per workhorse 2026-05-07 HIGH-2")
	assert.Contains(t, src, `${LOAD_PERSONA_RESEARCH_VAULT:-`,
		"load-persona.sh must support LOAD_PERSONA_RESEARCH_VAULT for the optional research-vault second-query path")
}

// TestVaultTrackReadScript_HasVaultPathPatternOverride — workhorse
// 2026-05-07 HIGH-1: vault-track-read.sh's `*/vaultmind-*/*.md`
// glob silently filtered out reads on `workhorse-vault/*.md`,
// turning the read-tracking hook inert. Pin the env-var override.
func TestVaultTrackReadScript_HasVaultPathPatternOverride(t *testing.T) {
	body, ok := hookscripts.Get("vault-track-read.sh")
	require.True(t, ok)
	src := string(body)
	assert.Contains(t, src, `${VAULT_PATH_PATTERN:-`,
		"vault-track-read.sh must support VAULT_PATH_PATTERN env-var override per workhorse 2026-05-07 HIGH-1")
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
