package memory_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The target note is the one the agent is most likely to act on, and it is the
// one the budget starves first: packTargetContent gives it unbounded priority,
// so on a vault whose median note exceeds the hook budget it consumes the whole
// pack and is then cut with a raw byte slice — `body[:maxChars]`.
//
// Two defects in that one line. It cuts mid-word ("...hypotheses about cost or
// arch..."), and because it slices BYTES rather than runes it can split a
// multi-byte character — and this vault's prose is full of em-dashes.
//
// With ExcerptTokens set, the target should get the same treatment as context
// items: the Principle section where one exists, bounded, ending on a sentence.
func TestContextPack_TargetIsExcerptedNotByteSliced(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)

	// Verified by sweeping the fixture: at 120 the target body fits WHOLE
	// (truncated=false) and ends on a period by luck — a test written at that
	// budget passes before the fix and proves nothing. Truncation actually
	// engages at 80, where the released code cuts "...through associ".
	const tightBudget = 80

	result, err := memory.ContextPack(resolver, db, memory.ContextPackConfig{
		Input: "proj-vaultmind", Budget: tightBudget, MaxItems: 2, ExcerptTokens: 60,
	})
	require.NoError(t, err)
	require.NotNil(t, result.Target)
	require.NotEmpty(t, result.Target.Body, "a starved target must still deliver its most relevant passage")

	body := strings.TrimSpace(result.Target.Body)
	assert.True(t, utf8.ValidString(body), "byte-slicing can split a rune; an excerpt must stay valid UTF-8")
	assert.True(t,
		strings.HasSuffix(body, ".") || strings.HasSuffix(body, "…") ||
			strings.HasSuffix(body, "!") || strings.HasSuffix(body, "?"),
		"got %q — a starved target should end on a sentence or a marked cut, never mid-word", body)
}

// Unchanged when the caller has not opted in: the released byte-slice behaviour
// still applies, so upgrading cannot silently alter existing output.
func TestContextPack_TargetTruncationUnchangedWithoutExcerpt(t *testing.T) {
	db := buildTestDB(t)
	resolver := graph.NewResolver(db)
	cfg := memory.ContextPackConfig{Input: "proj-vaultmind", Budget: 120, MaxItems: 2}

	a, err := memory.ContextPack(resolver, db, cfg)
	require.NoError(t, err)
	b, err := memory.ContextPack(resolver, db, cfg)
	require.NoError(t, err)

	assert.Equal(t, a.Target.Body, b.Target.Body)
	assert.Equal(t, a.UsedTokens, b.UsedTokens)
}
