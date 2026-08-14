package query

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/peiman/vaultmind/internal/embedding"
	"github.com/peiman/vaultmind/internal/index"
)

// ArcTypeName is the note type de-duplication compares proposals against. A
// proposal can only duplicate an ARC; resembling a principle or a reference says
// nothing about whether this transformation is already recorded.
const ArcTypeName = "arc"

// ArcMatch is an existing arc that resembles a proposal, with the cosine that
// says how much. It is evidence for a human judgement, never the judgement.
type ArcMatch struct {
	ID    string
	Title string
	Score float64
}

// arcVector is one existing arc with its stored dense embedding.
type arcVector struct {
	id    string
	title string
	vec   []float32
}

// Finder answers "which existing arcs does this resemble?" by cosine
// similarity against the arcs' stored dense embeddings.
//
// Two deliberate choices, both learned the hard way by running the first
// version against the real vault:
//
//  1. Cosine, not the hybrid retriever's fused score. The 4-lane RRF score is
//     built for ORDERING within one query; its magnitude is not a similarity.
//     The first implementation used it and every neighbour printed "0.02" — three
//     ties, no discrimination, a number that looks like evidence and isn't. This
//     project already has an arc about exactly that mistake (RRF fusion scores
//     are not cosines); it recurred here because a plausible API was reached for
//     without checking what its numbers mean.
//  2. Load the arc vectors ONCE. The first version ran a full hybrid search per
//     proposal — 27 proposals took 2m34s, which is not a tool anyone runs twice.
//     One embed per proposal against a preloaded matrix is the whole job.
type ArcFinder struct {
	arcs     []arcVector
	embedder embedding.Embedder
}

// NewArcFinder loads the arc vectors from an already-open index. The caller
// owns the DB and the embedder — this type borrows both and closes neither,
// which keeps the ownership obvious at the one place that opened them.
//
// A vault with no embedded arcs yields a finder that answers nothing: correct,
// and still cheap.
func NewArcFinder(db *index.DB, embedder embedding.Embedder) (*ArcFinder, error) {
	all, err := index.LoadAllEmbeddings(db)
	if err != nil {
		return nil, fmt.Errorf("loading arc embeddings: %w", err)
	}
	arcs := make([]arcVector, 0, len(all))
	for _, ne := range all {
		if strings.EqualFold(ne.Type, ArcTypeName) && len(ne.Embedding) > 0 {
			arcs = append(arcs, arcVector{id: ne.NoteID, title: ne.Title, vec: ne.Embedding})
		}
	}
	return &ArcFinder{arcs: arcs, embedder: embedder}, nil
}

// NearestArcs returns the arcs most similar to text, best first, scored by
// cosine similarity in [0,1] — a number the reader can actually compare across
// proposals, which is the entire point of showing it.
func (f *ArcFinder) NearestArcs(ctx context.Context, text string, limit int) ([]ArcMatch, error) {
	text = strings.TrimSpace(text)
	if f == nil || f.embedder == nil || text == "" || len(f.arcs) == 0 {
		return nil, nil
	}
	vec, err := f.embedder.Embed(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("embedding proposal text: %w", err)
	}

	scored := make([]ArcMatch, 0, len(f.arcs))
	for _, a := range f.arcs {
		scored = append(scored, ArcMatch{
			ID: a.id, Title: a.title, Score: CosineSimilarity(vec, a.vec),
		})
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored, nil
}

// EmbedTexts vectorizes many proposal texts at once, satisfying the recurrence
// detector's Vectorizer. Batched because recurrence embeds every proposal in
// the report, where per-item calls were what made the first de-duplication
// implementation unusably slow.
func (f *ArcFinder) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if f == nil || f.embedder == nil || len(texts) == 0 {
		return nil, nil
	}
	return f.embedder.EmbedBatch(ctx, texts)
}
