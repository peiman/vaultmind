package experiment

import "fmt"

// ScoredNote is a note with a computed score, used as input/output for variant scorers.
type ScoredNote struct {
	NoteID   string             `json:"note_id"`
	Score    float64            `json:"score"`
	Features map[string]float64 `json:"features,omitempty"`
}

// Scorer computes variant-specific scores for a list of notes.
type Scorer interface {
	Score(notes []ScoredNote) []ScoredNote
}

// ScorerFunc adapts a plain function to the Scorer interface.
type ScorerFunc func([]ScoredNote) []ScoredNote

// Score implements Scorer for ScorerFunc.
func (f ScorerFunc) Score(notes []ScoredNote) []ScoredNote { return f(notes) }

// Dispatcher routes scoring requests to registered variant scorers.
type Dispatcher struct {
	scorers map[string]Scorer
}

// NewDispatcher creates a dispatcher with the built-in "none" scorer registered.
func NewDispatcher() *Dispatcher {
	d := &Dispatcher{
		scorers: make(map[string]Scorer),
	}
	d.scorers["none"] = ScorerFunc(func(notes []ScoredNote) []ScoredNote {
		return notes
	})
	return d
}

// Register adds a scorer for the given variant name.
func (d *Dispatcher) Register(variant string, s Scorer) {
	d.scorers[variant] = s
}

// Score runs the scorer for the given variant. Returns error for unknown variants.
func (d *Dispatcher) Score(variant string, notes []ScoredNote) ([]ScoredNote, error) {
	s, ok := d.scorers[variant]
	if !ok {
		return nil, fmt.Errorf("unknown variant %q", variant)
	}
	return s.Score(notes), nil
}

// RunAll runs all specified variants and returns map[variant][]ScoredNote.
func (d *Dispatcher) RunAll(variants []string, notes []ScoredNote) (map[string][]ScoredNote, error) {
	results := make(map[string][]ScoredNote, len(variants))
	for _, v := range variants {
		scored, err := d.Score(v, notes)
		if err != nil {
			return nil, err
		}
		results[v] = scored
	}
	return results, nil
}
