package query

import (
	"context"
	"errors"
	"testing"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingEmbedder struct{ closed int }

func (e *countingEmbedder) Embed(context.Context, string) ([]float32, error) {
	return []float32{1}, nil
}
func (e *countingEmbedder) EmbedBatch(context.Context, []string) ([][]float32, error) {
	return [][]float32{{1}}, nil
}
func (e *countingEmbedder) Dims() int    { return 1 }
func (e *countingEmbedder) Close() error { e.closed++; return nil }

// A federated ask builds a retriever per vault, and each build loaded the
// model (~1.1s) — four loads for a three-vault ask. One model stays loaded for
// the process; asking for it again reuses it.
func TestSharedEmbedder_TheSameModelIsLoadedOnce(t *testing.T) {
	CloseSharedEmbedder() // other tests in the package load real models into the slot
	t.Cleanup(CloseSharedEmbedder)
	builds := 0
	build := func() (embedding.Embedder, func(), error) {
		builds++
		e := &countingEmbedder{}
		return e, func() { _ = e.Close() }, nil
	}

	a, cleanA, err := sharedEmbedderFor("test-model-a", build)
	require.NoError(t, err)
	cleanA() // a caller's cleanup does not unload the shared model
	b, cleanB, err := sharedEmbedderFor("test-model-a", build)
	require.NoError(t, err)
	cleanB()

	assert.Equal(t, 1, builds)
	assert.Same(t, a, b)
	assert.Zero(t, a.(*countingEmbedder).closed)
}

// One slot: asking for another model closes the loaded one first, so a
// process never holds two ONNX sessions at once.
func TestSharedEmbedder_AnotherModelReplacesTheLoadedOne(t *testing.T) {
	CloseSharedEmbedder() // other tests in the package load real models into the slot
	t.Cleanup(CloseSharedEmbedder)
	first := &countingEmbedder{}
	_, _, err := sharedEmbedderFor("test-model-a", func() (embedding.Embedder, func(), error) {
		return first, func() { _ = first.Close() }, nil
	})
	require.NoError(t, err)

	second := &countingEmbedder{}
	got, _, err := sharedEmbedderFor("test-model-b", func() (embedding.Embedder, func(), error) {
		assert.Equal(t, 1, first.closed, "the loaded model is closed before the next is built")
		return second, func() { _ = second.Close() }, nil
	})
	require.NoError(t, err)
	assert.Same(t, second, got)

	CloseSharedEmbedder()
	assert.Equal(t, 1, second.closed)
}

func TestSharedEmbedder_AFailedLoadIsNotCached(t *testing.T) {
	CloseSharedEmbedder() // other tests in the package load real models into the slot
	t.Cleanup(CloseSharedEmbedder)
	calls := 0
	failing := func() (embedding.Embedder, func(), error) { calls++; return nil, nil, errors.New("runtime missing") }
	_, _, err := sharedEmbedderFor("test-model-a", failing)
	require.Error(t, err)
	_, _, err = sharedEmbedderFor("test-model-a", failing)
	require.Error(t, err)
	assert.Equal(t, 2, calls, "a failure is retried, not remembered")
}
