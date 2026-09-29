package embedding

import "sync"

// MaxSimAll scores every doc against the query with MaxSimScore, spread over
// up to workers goroutines, and returns the scores in doc order. Each doc's
// score is independent and computed exactly as MaxSimScore does, so the result
// is identical to scoring one at a time — only faster on several cores.
//
// Why: ColBERT MaxSim over the candidate notes was 1.22 s of a 3.4 s
// three-vault ask, all on one core.
func MaxSimAll(queryTokens [][]float32, docs [][][]float32, workers int) []float64 {
	scores := make([]float64, len(docs))
	if workers > len(docs) {
		workers = len(docs)
	}
	if workers <= 1 {
		for i, d := range docs {
			scores[i] = MaxSimScore(queryTokens, d)
		}
		return scores
	}
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Interleaved, not blocked: long and short notes spread evenly.
			for i := w; i < len(docs); i += workers {
				scores[i] = MaxSimScore(queryTokens, docs[i])
			}
		}()
	}
	wg.Wait()
	return scores
}
