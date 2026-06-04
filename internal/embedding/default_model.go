package embedding

// DefaultModel returns the embedding model to use when the operator
// hasn't picked one explicitly. Adapts to the backend the binary was
// built against:
//
//   - ORT-tagged binaries → "bge-m3" (4-way hybrid retrieval — fast
//     on this build path; what the README's retrieval description is
//     built around).
//   - Pure-Go binaries → "minilm" (BGE-M3 indexing on pure-Go takes
//     hours per medium vault; minilm is the always-fast baseline).
//
// The default is conservative: it never picks a model the binary
// can't run reasonably. Users who want minilm on an ORT binary (e.g.
// for fast re-indexing during development) can pass --model minilm
// explicitly. Users who want bge-m3 on a pure-Go binary can opt in
// via --model bge-m3 + --allow-slow-backend.
//
// The 2026-05-05 dogfood surfaced this gap: the prior hardcoded
// "minilm" default contradicted the system's own framing. A user
// running `vaultmind index --embed` on an ORT-capable build silently
// got MiniLM-only embeddings, learning about it only from doctor's
// post-hoc warning. The runtime-aware default closes that gap by
// matching the model to what the binary can actually run well.
func DefaultModel() string {
	if BackendName() == "ort" {
		return "bge-m3"
	}
	return "minilm"
}
