package embedding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// These limits keep BGE-M3 embedding near 3 GB of memory. Each was measured
// (CHANGELOG 0.10.14: 12–16 GB at an 8190-token cap, about 6 GB at 2048,
// about 3 GB at 1024). Raising one needs a new peak measurement with
// `task test:memory`, and this test updated to the new bound.

func TestBGEM3EmbedMaxTokens_StaysWithinTheMeasuredBound(t *testing.T) {
	assert.LessOrEqual(t, BGEM3EmbedMaxTokens, 1024,
		"a larger per-note cap was measured at about 6 GB per embed pass; re-measure with task test:memory before raising it")
}

func TestBGEM3Config_TruncatesAtTheEmbedCap(t *testing.T) {
	cfg := BGEM3Config()
	assert.Equal(t, BGEM3EmbedMaxTokens, cfg.MaxTokens, "the embedder is configured with the embed cap, not the model limit")
	assert.Equal(t, BGEM3EmbedMaxTokens, safeTokenizerLimit(cfg.MaxTokens), "the tokenizer clamp honors the embed cap")
}

// ORT's CPU arena and memory-pattern planning keep a buffer per input shape
// and never give it back, so a long-lived process grows with every new
// length. The ORT-tagged session test checks the session applies these.
func TestORTMemorySettings_StayOff(t *testing.T) {
	assert.False(t, ortCPUMemArena, "ORT's CPU memory arena must stay off")
	assert.False(t, ortMemPattern, "ORT's memory-pattern planning must stay off")
}
