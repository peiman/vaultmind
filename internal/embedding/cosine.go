package embedding

import "math"

// CosineSimilarity is the cosine between two vectors, or 0 when they are
// unusable (mismatched length, empty, or zero-magnitude).
//
// It lives in the embedding package because both the retrieval layer and the
// distillation layer need it, and the architecture forbids one importing the
// other (ADR-009). A second copy would drift — and this project has an arc
// about a similarity number that silently meant the wrong thing, so one
// definition is the point.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
