package embedding

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knights-analytics/hugot/backends"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeOutput(v float32) *BGEM3Output {
	return &BGEM3Output{
		Dense:   []float32{v, v},
		Sparse:  map[int32]float32{7: v},
		ColBERT: [][]float32{{v, v}, {v, v}},
	}
}

// A three-vault ask ran the model 16 times on one query: every lane in every
// vault embedded it again. The same text must cost one forward pass.
func TestQueryMemo_SameTextRunsTheModelOnce(t *testing.T) {
	var m queryMemo
	var runs atomic.Int64
	compute := func() (*BGEM3Output, error) { runs.Add(1); return fakeOutput(1), nil }

	for range 16 {
		out, err := m.get("spreading activation", compute)
		require.NoError(t, err)
		assert.Equal(t, []float32{1, 1}, out.Dense)
	}
	assert.Equal(t, int64(1), runs.Load())

	_, err := m.get("a different query", compute)
	require.NoError(t, err)
	assert.Equal(t, int64(2), runs.Load(), "a different text is a different embedding")
}

// The lanes of one vault run concurrently, so their first requests arrive
// together; they must share the one pass in flight, not each start their own.
func TestQueryMemo_ConcurrentCallersShareOnePass(t *testing.T) {
	var m queryMemo
	var runs atomic.Int64
	release := make(chan struct{})
	compute := func() (*BGEM3Output, error) { runs.Add(1); <-release; return fakeOutput(2), nil }

	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := m.get("q", compute)
			assert.NoError(t, err)
			assert.Equal(t, []float32{2, 2}, out.Dense)
		}()
	}
	close(release)
	wg.Wait()
	assert.Equal(t, int64(1), runs.Load())
}

// Every caller gets its own copy: a lane that normalizes or reuses its slice
// in place must not change what the next lane scores with.
func TestQueryMemo_CallersCannotCorruptEachOther(t *testing.T) {
	var m queryMemo
	compute := func() (*BGEM3Output, error) { return fakeOutput(3), nil }

	first, err := m.get("q", compute)
	require.NoError(t, err)
	first.Dense[0] = 99
	first.Sparse[7] = 99
	first.ColBERT[0][0] = 99

	second, err := m.get("q", compute)
	require.NoError(t, err)
	assert.Equal(t, []float32{3, 3}, second.Dense)
	assert.Equal(t, map[int32]float32{7: 3}, second.Sparse)
	assert.Equal(t, [][]float32{{3, 3}, {3, 3}}, second.ColBERT)
}

// A failed pass is not remembered: the next caller tries again.
func TestQueryMemo_ErrorsAreNotCached(t *testing.T) {
	var m queryMemo
	var runs atomic.Int64
	fail := true
	compute := func() (*BGEM3Output, error) {
		runs.Add(1)
		if fail {
			return nil, errors.New("model hiccup")
		}
		return fakeOutput(4), nil
	}

	_, err := m.get("q", compute)
	require.Error(t, err)
	fail = false
	out, err := m.get("q", compute)
	require.NoError(t, err)
	assert.Equal(t, []float32{4, 4}, out.Dense)
	assert.Equal(t, int64(2), runs.Load())
}

// Bounded: a long-lived process embedding many texts one at a time keeps only
// the most recent few, so memory cannot grow without limit.
func TestQueryMemo_IsBounded(t *testing.T) {
	var m queryMemo
	compute := func() (*BGEM3Output, error) { return fakeOutput(5), nil }
	for i := range queryMemoSize * 3 {
		_, err := m.get(fmt.Sprintf("text %d", i), compute)
		require.NoError(t, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	assert.LessOrEqual(t, len(m.entries), queryMemoSize)
	assert.Len(t, m.order, len(m.entries), "the eviction order tracks exactly the remembered texts")
}

// EmbedFull goes through the memo, and a failed pass is retried rather than
// remembered — checked on the real embedder with a tokenizer that fails, so
// no model is needed.
func TestEmbedFull_FailedPassIsRetried(t *testing.T) {
	var calls atomic.Int64
	fail := func(_ *backends.PipelineBatch, _ []string) error {
		calls.Add(1)
		return errors.New("tokenizer down")
	}
	e := &BGEM3Embedder{maxTokens: 10, preprocess: fail}

	for range 2 {
		_, err := e.EmbedFull(context.Background(), "spreading activation")
		require.ErrorContains(t, err, "tokenizer down")
	}
	assert.GreaterOrEqual(t, calls.Load(), int64(2), "each call retried the pass")
}

// Callers waiting on a pass that fails all see its error.
func TestQueryMemo_WaitersShareAFailure(t *testing.T) {
	var m queryMemo
	release := make(chan struct{})
	started := make(chan struct{})
	compute := func() (*BGEM3Output, error) {
		close(started)
		<-release
		return nil, errors.New("model hiccup")
	}

	errs := make(chan error, 2)
	go func() { _, err := m.get("q", compute); errs <- err }()
	<-started
	go func() {
		_, err := m.get("q", func() (*BGEM3Output, error) { return fakeOutput(6), nil })
		errs <- err
	}()
	// Give the second caller time to join the pass in flight; neither may return
	// while it is held.
	require.Never(t, func() bool { return len(errs) > 0 }, 100*time.Millisecond, 10*time.Millisecond)
	close(release)
	// Had the second caller run its own pass, it would have succeeded.
	for range 2 {
		require.Error(t, <-errs)
	}
}
