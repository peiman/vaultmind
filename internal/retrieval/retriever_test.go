package retrieval_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ScoredResult is the shape of every search/ask JSON hit in the public CLI
// envelope (schema_version=v1). Downstream agents parse these tags
// directly; dropping a field, renaming a tag, or losing is_domain_note
// changes the contract in a way callers will only see at runtime. This
// test pins the wire format so a rename in retriever.go is visible at
// test time, not at agent-integration time.
func TestScoredResult_JSONContract(t *testing.T) {
	r := retrieval.ScoredResult{
		ID: "note-42", Type: "reference", Title: "Example",
		Path: "references/example.md", Snippet: "snippet text",
		Score: 0.75, IsDomain: true,
		Components: map[string]float64{"fts": 0.5, "dense": 0.25},
	}

	data, err := json.Marshal(r)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	// Every tag the CLI envelope promises must be present and correct.
	// Testing as map[string]any means a rename (e.g. "is_domain_note" →
	// "isDomain") fails this assertion, which is the point.
	assert.Equal(t, "note-42", decoded["id"])
	assert.Equal(t, "reference", decoded["type"])
	assert.Equal(t, "Example", decoded["title"])
	assert.Equal(t, "references/example.md", decoded["path"])
	assert.Equal(t, "snippet text", decoded["snippet"])
	assert.InDelta(t, 0.75, decoded["score"], 1e-9)
	assert.Equal(t, true, decoded["is_domain_note"])
	require.Contains(t, decoded, "components")
	components := decoded["components"].(map[string]any)
	assert.InDelta(t, 0.5, components["fts"], 1e-9)
	assert.InDelta(t, 0.25, components["dense"], 1e-9)
}

// Components carries per-sub-retriever RRF contributions and is populated
// only by HybridRetriever. Non-hybrid retrievers leave it nil; the wire
// contract hides the field via `omitempty` so downstream parsers do not
// see a misleading empty object. A regression where a hybrid path forgets
// to set Components would still encode as `{}` without omitempty — this
// test pins the absence.
func TestScoredResult_ComponentsOmittedWhenNil(t *testing.T) {
	r := retrieval.ScoredResult{ID: "note-1", Score: 0.5}

	data, err := json.Marshal(r)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	_, present := decoded["components"]
	assert.False(t, present, "components should be omitted when nil, got %s", string(data))
}

// The Retriever interface is the contract the baseline runner, the
// ask/search commands, and hybrid fusion all program against. It has one
// method; this test documents that surface so a signature change — e.g.
// adding a parameter or renaming Search — is explicit in the test
// failure rather than dispersed across every implementor.
func TestRetriever_InterfaceSurface(t *testing.T) {
	var r retrieval.Retriever = &stubRetriever{}
	results, total, err := r.Search(context.Background(), "q", 10, 0, index.SearchFilters{})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, results)
}

type stubRetriever struct{}

func (s *stubRetriever) Search(_ context.Context, _ string, _, _ int, _ index.SearchFilters) ([]retrieval.ScoredResult, int, error) {
	return nil, 0, nil
}

// NamedRetriever is how HybridRetriever labels per-component RRF
// contributions — the Name field is the key that appears in
// ScoredResult.Components. A reader constructing one by mistake without a
// Name would silently produce empty-string components keys; this test
// locks the field shape so the zero value is visible if someone
// reorganizes the struct.
func TestNamedRetriever_FieldShape(t *testing.T) {
	var stub retrieval.Retriever = &stubRetriever{}
	nr := retrieval.NamedRetriever{Name: "fts", Retriever: stub}
	assert.Equal(t, "fts", nr.Name)
	assert.NotNil(t, nr.Retriever)

	zero := retrieval.NamedRetriever{}
	assert.Empty(t, zero.Name, "zero value Name must be empty string (caller responsibility to set)")
	assert.Nil(t, zero.Retriever)
}
