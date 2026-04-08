package embedding

import (
	"math"
)

// L2Normalize returns a unit vector. Returns zero vector if magnitude is zero.
func L2Normalize(vec []float32) []float32 {
	var mag float64
	for _, v := range vec {
		mag += float64(v) * float64(v)
	}
	if mag == 0 {
		out := make([]float32, len(vec))
		return out
	}
	invMag := float32(1.0 / math.Sqrt(mag))
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = v * invMag
	}
	return out
}

// DenseHead extracts the CLS token embedding (index 0) and L2-normalizes it.
// Input: hiddenStates[seq_len][dims]. Output: [dims] unit vector.
func DenseHead(hiddenStates [][]float32) []float32 {
	if len(hiddenStates) == 0 {
		return nil
	}
	return L2Normalize(hiddenStates[0])
}

// SparseHead computes learned lexical weights per token.
// For each non-special token: weight = ReLU(dot(hidden, w) + bias).
// Weights scattered to vocabulary positions via tokenIDs.
// Duplicate token IDs keep the maximum weight.
func SparseHead(hiddenStates [][]float32, tokenIDs, specialMask []uint32, weights []float32, bias float32) map[int32]float32 {
	sparse := make(map[int32]float32)
	for i, hidden := range hiddenStates {
		if i < len(specialMask) && specialMask[i] == 1 {
			continue
		}
		var val float32
		for j, h := range hidden {
			if j < len(weights) {
				val += h * weights[j]
			}
		}
		val += bias
		if val <= 0 {
			continue
		}
		tid := tokenIDs[i]
		if tid > math.MaxInt32 {
			continue
		}
		vocabID := int32(tid) //nolint:gosec // guarded by MaxInt32 check above
		if existing, ok := sparse[vocabID]; !ok || val > existing {
			sparse[vocabID] = val
		}
	}
	return sparse
}

// ColBERTHead projects each non-CLS token through a linear layer and L2-normalizes.
// Input: hiddenStates[seq_len][dims], weights[out_dims][in_dims], bias[out_dims].
// Output: [seq_len-1][out_dims] (CLS at index 0 is skipped).
func ColBERTHead(hiddenStates [][]float32, weights [][]float32, bias []float32) [][]float32 {
	if len(hiddenStates) <= 1 {
		return nil
	}
	outDims := len(weights)
	result := make([][]float32, 0, len(hiddenStates)-1)
	for i := 1; i < len(hiddenStates); i++ {
		vec := make([]float32, outDims)
		for j := 0; j < outDims; j++ {
			var sum float32
			for k, h := range hiddenStates[i] {
				if k < len(weights[j]) {
					sum += h * weights[j][k]
				}
			}
			if j < len(bias) {
				sum += bias[j]
			}
			vec[j] = sum
		}
		result = append(result, L2Normalize(vec))
	}
	return result
}

// SparseDotProduct computes the dot product between two sparse vectors.
// Only overlapping keys contribute.
func SparseDotProduct(a, b map[int32]float32) float64 {
	if len(a) > len(b) {
		a, b = b, a
	}
	var sum float64
	for id, wa := range a {
		if wb, ok := b[id]; ok {
			sum += float64(wa) * float64(wb)
		}
	}
	return sum
}

// MaxSimScore computes the ColBERT MaxSim score between query and document token matrices.
// For each query token, finds max cosine similarity across all doc tokens, then sums.
func MaxSimScore(queryTokens, docTokens [][]float32) float64 {
	var total float64
	for _, qVec := range queryTokens {
		var maxSim float64 = -1
		for _, dVec := range docTokens {
			sim := cosineF32(qVec, dVec)
			if sim > maxSim {
				maxSim = sim
			}
		}
		if maxSim > -1 {
			total += maxSim
		}
	}
	return total
}

func cosineF32(a, b []float32) float64 {
	var dot, normA, normB float64
	for i := range a {
		if i >= len(b) {
			break
		}
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
