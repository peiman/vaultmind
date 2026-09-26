package query

import (
	"sync"

	"github.com/peiman/vaultmind/internal/embedding"
)

// shared holds the one embedding model this process has loaded.
//
// A federated ask builds a retriever for every vault and again for the vault
// that delivers, and each build loaded the model — about 1.1s each, four times
// for a three-vault ask, measured as half of its wall time. The model is now
// loaded once and kept. One slot, not a map: asking for a different model
// closes the loaded one first, so the process never holds two ONNX sessions,
// the constraint federation was written around.
var shared struct {
	mu    sync.Mutex
	key   string
	emb   embedding.Embedder
	close func()
}

// sharedEmbedderFor returns the loaded model for key, building it on first
// use. The returned cleanup does nothing: the model stays loaded for the next
// caller and is released by CloseSharedEmbedder or at process exit. A failed
// build is not remembered, so the next caller tries again.
func sharedEmbedderFor(key string, build func() (embedding.Embedder, func(), error)) (embedding.Embedder, func(), error) {
	shared.mu.Lock()
	defer shared.mu.Unlock()
	if shared.emb != nil && shared.key == key {
		return shared.emb, func() {}, nil
	}
	releaseSharedLocked()
	emb, closeFn, err := build()
	if err != nil {
		return nil, nil, err
	}
	shared.key, shared.emb, shared.close = key, emb, closeFn
	return emb, func() {}, nil
}

// CloseSharedEmbedder releases the process's model, if one is loaded.
func CloseSharedEmbedder() {
	shared.mu.Lock()
	defer shared.mu.Unlock()
	releaseSharedLocked()
}

func releaseSharedLocked() {
	if shared.close != nil {
		shared.close()
	}
	shared.key, shared.emb, shared.close = "", nil, nil
}
