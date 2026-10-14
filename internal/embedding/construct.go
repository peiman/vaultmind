package embedding

import "context"

// ConstructBGEM3 builds the BGE-M3 embedder. Ranking and embed-on-write both
// go through it, so a test can count how many times a process loads the model.
// The production value is NewBGEM3Embedder.
//
// A zero config is the default, resolved inside the seam so a stub does not
// run DefaultCacheDir. Evaluating BGEM3Config at the call site renames
// ~/.vaultmind/models before the stub runs.
var ConstructBGEM3 = func(ctx context.Context, cfg HugotConfig) (Embedder, error) {
	if cfg == (HugotConfig{}) {
		cfg = BGEM3Config()
	}
	return NewBGEM3Embedder(ctx, cfg)
}

// ConstructMiniLM builds the MiniLM embedder. Same seam as ConstructBGEM3.
// A zero config is the default, resolved here for the same reason.
var ConstructMiniLM = func(ctx context.Context, cfg HugotConfig) (Embedder, error) {
	if cfg == (HugotConfig{}) {
		cfg = DefaultHugotConfig()
	}
	return NewHugotEmbedder(ctx, cfg)
}
