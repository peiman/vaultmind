package embedding_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/assert"
)

// DefaultModel adapts to the backend the binary was built against —
// bge-m3 on ORT-tagged builds (the fast hybrid path the README is
// built around), minilm on pure-Go builds (the always-fast baseline,
// because BGE-M3 on pure-Go takes hours per medium vault). The
// 2026-05-05 onboarding/workhorse dogfood surfaced that the prior
// hardcoded "minilm" default contradicted the system's own framing
// — a user running `vaultmind index --embed` on an ORT-capable build
// silently got MiniLM-only embeddings, then learned about it from
// doctor afterward.

// TestDefaultModel_MatchesBackend — pin the contract under whichever
// build tag this test runs against. ORT production builds (the one
// task build produces when libtokenizers.a is present) MUST default
// to bge-m3; pure-Go builds MUST default to minilm. CI running both
// build configs exercises both branches; the t.Fatalf on unknown
// backends forces explicit decision if a new backend is added.
func TestDefaultModel_MatchesBackend(t *testing.T) {
	switch embedding.BackendName() {
	case "ort":
		assert.Equal(t, "bge-m3", embedding.DefaultModel(),
			"ORT-tagged build defaults to bge-m3 — the recommended fast hybrid path")
	case "go":
		assert.Equal(t, "minilm", embedding.DefaultModel(),
			"pure-Go build defaults to minilm — BGE-M3 on pure-Go takes hours")
	default:
		t.Fatalf("unexpected BackendName: %q — DefaultModel contract not yet defined", embedding.BackendName())
	}
}
