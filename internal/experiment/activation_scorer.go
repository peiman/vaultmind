package experiment

import "time"

// maxSessionWindowLimit is the maximum number of recent sessions to consider
// when partitioning time into active/idle periods.
const maxSessionWindowLimit = 100

// ActivationParams holds tunable parameters for activation scoring.
type ActivationParams struct {
	Gamma float64 // idle time compression (0.0-1.0)
	D     float64 // decay exponent (ACT-R default: 0.5)
	Alpha float64 // retrieval strength weight
	Beta  float64 // storage strength weight
	Delta float64 // spreading activation / similarity weight
}

// DefaultActivationParams returns params with research-based defaults.
// Delta defaults to 0.0 (no similarity component) for backward compatibility.
func DefaultActivationParams(gamma float64) ActivationParams {
	return ActivationParams{Gamma: gamma, D: 0.5, Alpha: 0.6, Beta: 0.4, Delta: 0.0}
}

var variantGammas = map[string]float64{
	"compressed-0.2": 0.2,
	"compressed-0.5": 0.5,
	"wall-clock":     1.0,
	"none":           0.0,
}

// VariantGamma returns the gamma for a known variant name.
func VariantGamma(variant string) (float64, bool) {
	g, ok := variantGammas[variant]
	return g, ok
}

// ComputeBatchScores computes activation scores for a batch of notes.
// Returns noteID->score and noteID->features maps.
// similarities is optional (nil = no spreading activation). When provided,
// it maps noteID -> cosine similarity with the current query.
func ComputeBatchScores(db *DB, noteIDs []string, params ActivationParams, similarities map[string]float64) (map[string]float64, map[string]map[string]float64, error) {
	if len(noteIDs) == 0 {
		return make(map[string]float64), make(map[string]map[string]float64), nil
	}

	accessMap, err := db.BatchNoteAccessTimes(noteIDs)
	if err != nil {
		return nil, nil, err
	}

	windows, err := db.RecentSessionWindows(maxSessionWindowLimit)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	scores, features := ScoreFromData(noteIDs, accessMap, windows, now, params, similarities)
	return scores, features, nil
}

// ScoreFromData computes activation scores from pre-fetched data.
// Returns (scores, features). Use this to avoid redundant DB queries when
// computing multiple variants over the same data.
// similarities is optional (nil = no similarity component). When provided,
// it maps noteID -> cosine similarity with the current query, implementing
// ACT-R spreading activation.
func ScoreFromData(noteIDs []string, accessMap map[string][]time.Time, windows []SessionWindow, now time.Time, params ActivationParams, similarities map[string]float64) (map[string]float64, map[string]map[string]float64) {
	scores := make(map[string]float64, len(noteIDs))
	features := make(map[string]map[string]float64, len(noteIDs))

	for _, noteID := range noteIDs {
		accessTimes := accessMap[noteID]
		sim := 0.0
		if similarities != nil {
			sim = similarities[noteID]
		}

		if len(accessTimes) == 0 {
			scores[noteID] = CombinedScore(0.0, 0.0, sim, params.Alpha, params.Beta, params.Delta)
			features[noteID] = map[string]float64{
				"retrieval_strength": 0.0,
				"storage_strength":   0.0,
				"similarity":         sim,
				"access_count":       0.0,
			}
			continue
		}

		retrieval := ComputeRetrieval(accessTimes, now, windows, params.Gamma, params.D)
		storage := ComputeStorage(len(accessTimes))
		score := CombinedScore(retrieval, storage, sim, params.Alpha, params.Beta, params.Delta)

		scores[noteID] = score
		features[noteID] = map[string]float64{
			"retrieval_strength": retrieval,
			"storage_strength":   storage,
			"similarity":         sim,
			"access_count":       float64(len(accessTimes)),
		}
	}

	return scores, features
}
