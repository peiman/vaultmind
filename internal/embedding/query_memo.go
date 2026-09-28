package embedding

import (
	"maps"
	"sync"
)

// queryMemoSize bounds the memo. One ask embeds one query text however many
// vaults and lanes it searches, so two entries cover it with one to spare.
// Kept small because not every caller embeds a query: the arc finder and the
// near-duplicate check embed whole notes, and at the model's 8192-token cap
// one output's ColBERT vectors are ~32 MB — two entries cap what is retained
// at ~64 MB.
const queryMemoSize = 2

// queryMemo remembers recent single-text embeddings, so a query costs one
// forward pass. A three-vault ask ran the model 16 times on the same text —
// every lane (dense, sparse, ColBERT) in every vault called EmbedFull, which
// computes all three outputs and kept one. The zero value is ready to use.
type queryMemo struct {
	mu      sync.Mutex
	entries map[string]*memoEntry
	order   []string // insertion order, oldest first, for eviction
}

type memoEntry struct {
	done chan struct{} // closed when out/err are set
	out  *BGEM3Output
	err  error
}

// get returns the embedding of text, calling compute at most once per text
// while it is remembered. Concurrent callers share the pass in flight. A
// failed pass is forgotten, so the next caller tries again. Each caller gets
// its own copy.
//
// The first caller's compute (and so its context) runs the pass. If that
// context is cancelled, callers waiting on the pass see the cancellation
// too; the error is not remembered, so a later caller runs its own pass.
func (m *queryMemo) get(text string, compute func() (*BGEM3Output, error)) (*BGEM3Output, error) {
	m.mu.Lock()
	if m.entries == nil {
		m.entries = map[string]*memoEntry{}
	}
	if e, ok := m.entries[text]; ok {
		m.mu.Unlock()
		<-e.done
		if e.err != nil {
			return nil, e.err
		}
		return cloneOutput(e.out), nil
	}
	e := &memoEntry{done: make(chan struct{})}
	m.entries[text] = e
	m.order = append(m.order, text)
	m.evictLocked()
	m.mu.Unlock()

	e.out, e.err = compute()
	close(e.done)
	if e.err != nil {
		m.forget(text, e)
		return nil, e.err
	}
	return cloneOutput(e.out), nil
}

// evictLocked drops the oldest entries beyond the bound.
func (m *queryMemo) evictLocked() {
	for len(m.order) > queryMemoSize {
		delete(m.entries, m.order[0])
		m.order = m.order[1:]
	}
}

// forget removes text's entry if it is still e (not a newer retry's).
func (m *queryMemo) forget(text string, e *memoEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries[text] != e {
		return
	}
	delete(m.entries, text)
	for i, t := range m.order {
		if t == text {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
}

func cloneOutput(o *BGEM3Output) *BGEM3Output {
	c := &BGEM3Output{
		Dense:  append([]float32(nil), o.Dense...),
		Sparse: maps.Clone(o.Sparse),
	}
	if o.ColBERT != nil {
		c.ColBERT = make([][]float32, len(o.ColBERT))
		for i, tok := range o.ColBERT {
			c.ColBERT[i] = append([]float32(nil), tok...)
		}
	}
	return c
}
