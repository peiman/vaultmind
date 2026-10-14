package query

import (
	"sync"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/rs/zerolog/log"
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
	gen   uint64
}

// sharedEmbedderFor returns the loaded model for key, building it on first
// use. The returned cleanup does nothing: the model stays loaded for the next
// caller and is released by CloseSharedEmbedder or at process exit. A failed
// build is not remembered, so the next caller tries again.
func sharedEmbedderFor(key string, build func() (embedding.Embedder, func(), error)) (embedding.Embedder, func(), error) {
	shared.mu.Lock()
	defer shared.mu.Unlock()
	if shared.emb != nil && shared.key == key {
		log.Debug().Str("model", key).Msg("reusing embedding model")
		return shared.emb, func() {}, nil
	}
	releaseSharedLocked()
	emb, closeFn, err := build()
	if err != nil {
		return nil, nil, err
	}
	shared.gen++
	shared.key, shared.emb, shared.close = key, emb, closeFn
	log.Debug().Str("model", key).Msg("loaded embedding model")
	return emb, func() {}, nil
}

// SharedHold is one caller's claim on the process embedder.
// The zero value acquired nothing.
type SharedHold struct {
	gen uint64
}

// Acquired reports whether this hold took the loaded session.
func (h SharedHold) Acquired() bool { return h.gen != 0 }

// Close releases the process embedder only when h acquired the session that
// is still loaded. A hold that never acquired, or that acquired a session
// already replaced, leaves the current one alone. A second Close is a no-op.
func (h SharedHold) Close() {
	if h.gen == 0 {
		return
	}
	shared.mu.Lock()
	defer shared.mu.Unlock()
	if shared.gen != h.gen || shared.emb == nil {
		return
	}
	releaseSharedLocked()
}

// CurrentSharedHold returns the hold for the session that is loaded.
// It is the zero hold when nothing is loaded.
func CurrentSharedHold() SharedHold {
	shared.mu.Lock()
	defer shared.mu.Unlock()
	if shared.emb == nil {
		return SharedHold{}
	}
	return SharedHold{gen: shared.gen}
}

// CloseSharedEmbedder releases the process's model, if one is loaded.
// A second call is a no-op. Callers that never acquired the session use
// SharedHold.Close instead, which leaves a holder they do not own in place.
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
