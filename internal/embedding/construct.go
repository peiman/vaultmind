package embedding

import "context"

// ConstructBGEM3 builds the BGE-M3 embedder. Ranking and embed-on-write both
// go through it, so a test can count how many times a process loads the model.
// The production value is NewBGEM3Embedder.
var ConstructBGEM3 = func(ctx context.Context, cfg HugotConfig) (Embedder, error) {
	return NewBGEM3Embedder(ctx, cfg)
}

// ConstructMiniLM builds the MiniLM embedder. Same seam as ConstructBGEM3.
var ConstructMiniLM = func(ctx context.Context, cfg HugotConfig) (Embedder, error) {
	return NewHugotEmbedder(ctx, cfg)
}
