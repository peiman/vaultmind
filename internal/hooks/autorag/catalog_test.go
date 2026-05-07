package autorag_test

import (
	"strings"
	"testing"

	"github.com/peiman/vaultmind/internal/hooks/autorag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The auto-RAG drift catalog is consumer-supplied JSON (per the
// workhorse v0.3 handoff shim outline): each entry names a drift
// signature and how it should be matched. The Go types here pin the
// schema; the bash engine consumes the same JSON shape via
// DRIFT_CATALOG env var (slice C2). Validation lives here so authors
// can lint a catalog without spawning bash.

// TestParseCatalog_ValidMinimal — the smallest legal catalog: one
// Bash-tool signature with required fields. Confirms ParseCatalog
// accepts valid JSON and surfaces all fields.
func TestParseCatalog_ValidMinimal(t *testing.T) {
	js := `[{
		"name": "rebuild-vaultmind",
		"tool": "Bash",
		"match": "go\\s+(build|install)",
		"decision": "inject",
		"query": "don't rebuild vaultmind"
	}]`
	cat, err := autorag.ParseCatalog([]byte(js))
	require.NoError(t, err)
	require.Len(t, cat.Signatures, 1)
	sig := cat.Signatures[0]
	assert.Equal(t, "rebuild-vaultmind", sig.Name)
	assert.Equal(t, "Bash", sig.Tool)
	assert.Equal(t, `go\s+(build|install)`, sig.Match)
	assert.Equal(t, "inject", sig.Decision)
	assert.Equal(t, "don't rebuild vaultmind", sig.Query)
}

func TestParseCatalog_MultipleSignatures(t *testing.T) {
	js := `[
		{"name":"a","tool":"Bash","match":"a","decision":"inject","query":"q1"},
		{"name":"b","tool":"Write","match":"b","decision":"deny","query":"q2"}
	]`
	cat, err := autorag.ParseCatalog([]byte(js))
	require.NoError(t, err)
	require.Len(t, cat.Signatures, 2)
	assert.Equal(t, "deny", cat.Signatures[1].Decision)
}

func TestParseCatalog_RejectsInvalidJSON(t *testing.T) {
	_, err := autorag.ParseCatalog([]byte(`{not json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

// Validate is the lint pass — runs after ParseCatalog. Each rule
// pins a schema invariant the bash engine assumes.

func TestValidate_RequiresName(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Tool: "Bash", Match: "x", Decision: "inject", Query: "q"},
	}}
	err := cat.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "name")
}

func TestValidate_RequiresTool(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Name: "x", Match: "x", Decision: "inject", Query: "q"},
	}}
	err := cat.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "tool")
}

func TestValidate_RequiresMatch(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Name: "x", Tool: "Bash", Decision: "inject", Query: "q"},
	}}
	err := cat.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "match")
}

func TestValidate_RequiresQuery(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Name: "x", Tool: "Bash", Match: "y", Decision: "inject"},
	}}
	err := cat.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "query")
}

// Decision must be one of the values the bash engine knows: inject |
// deny | allow. "ask" is documented as broken on Write/Edit in
// 2.1.129, so we reject it at lint time so consumers don't ship a
// catalog with a silently-ignored decision.
func TestValidate_RejectsUnknownDecision(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Name: "x", Tool: "Bash", Match: "y", Decision: "warn", Query: "q"},
	}}
	err := cat.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "decision")
}

func TestValidate_RejectsAskDecision(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Name: "x", Tool: "Write", Match: "y", Decision: "ask", Query: "q"},
	}}
	err := cat.Validate()
	require.Error(t, err)
	// The error must name the regression so authors understand why
	// "ask" is rejected, not just that it's not in the allowlist.
	assert.Contains(t, strings.ToLower(err.Error()), "ask")
}

// The bash engine pipes Match through grep -E, so an unparseable
// regex would cause every Read with that signature to silently
// fail-open (grep returns nonzero, the if-branch is false). Catch
// at lint time.
func TestValidate_RejectsInvalidRegex(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Name: "x", Tool: "Bash", Match: "(unclosed", Decision: "inject", Query: "q"},
	}}
	err := cat.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "regex")
}

// Tool must be one of the Claude Code PreToolUse matchers we
// actually support: Bash | Write | Edit. Future tools (Read,
// MultiEdit) will need engine work, so reject at lint time.
func TestValidate_RejectsUnknownTool(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Name: "x", Tool: "Notebook", Match: "y", Decision: "inject", Query: "q"},
	}}
	err := cat.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "tool")
}

// Multiple signatures with the same Name would let one shadow another
// in the report — surface as an error so authors fix it before
// shipping.
func TestValidate_RejectsDuplicateNames(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Name: "x", Tool: "Bash", Match: "y", Decision: "inject", Query: "q1"},
		{Name: "x", Tool: "Write", Match: "z", Decision: "deny", Query: "q2"},
	}}
	err := cat.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "duplicate")
}

func TestValidate_AcceptsValidCatalog(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Name: "rebuild", Tool: "Bash", Match: "go\\s+build", Decision: "inject", Query: "q"},
		{Name: "cross-write", Tool: "Write", Match: "/etc/", Decision: "deny", Query: "q"},
	}}
	require.NoError(t, cat.Validate())
}

// Empty catalog is legal — a consumer with no project-specific
// drifts but who still wants the engine installed for future use.
func TestValidate_AcceptsEmpty(t *testing.T) {
	cat := &autorag.Catalog{}
	require.NoError(t, cat.Validate())
}

// Whitespace-only required fields collapse to empty after trim;
// pin the contract so a future "let's allow tabs as identifiers"
// regression doesn't slip through.
func TestValidate_RejectsWhitespaceOnlyName(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Name: "   ", Tool: "Bash", Match: "x", Decision: "inject", Query: "q"},
	}}
	err := cat.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "name")
}

func TestValidate_RejectsWhitespaceOnlyQuery(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Name: "x", Tool: "Bash", Match: "y", Decision: "inject", Query: "   "},
	}}
	err := cat.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "query")
}

// The bash engine reads catalog dispatch results back via
// `IFS=$'\t' read -r DRIFT QUERY DECISION`. A TAB in name or query
// would corrupt the field split. Reject at lint time so the bash
// side never has to defend against this shape (HIGH severity per
// 2026-05-07 code-review).
func TestValidate_RejectsTabInName(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Name: "with\ttab", Tool: "Bash", Match: "x", Decision: "inject", Query: "q"},
	}}
	err := cat.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "tab")
}

func TestValidate_RejectsTabInQuery(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Name: "x", Tool: "Bash", Match: "y", Decision: "inject", Query: "q\twith\ttabs"},
	}}
	err := cat.Validate()
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "tab")
}

func TestValidate_RejectsNewlineInName(t *testing.T) {
	cat := &autorag.Catalog{Signatures: []autorag.Signature{
		{Name: "x\nbreaking", Tool: "Bash", Match: "y", Decision: "inject", Query: "q"},
	}}
	err := cat.Validate()
	require.Error(t, err)
}
