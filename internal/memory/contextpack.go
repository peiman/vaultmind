package memory

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
)

// ContextPackConfig holds parameters for a ContextPack operation.
type ContextPackConfig struct {
	Input  string
	Budget int // token budget
}

// ContextPackTarget holds the fully-loaded target note.
type ContextPackTarget struct {
	ID          string                 `json:"id"`
	Frontmatter map[string]interface{} `json:"frontmatter"`
	Body        string                 `json:"body,omitempty"`
}

// ContextItem holds context metadata for a single related note.
type ContextItem struct {
	ID           string                 `json:"id"`
	EdgeType     string                 `json:"edge_type"`
	Confidence   string                 `json:"confidence"`
	Frontmatter  map[string]interface{} `json:"frontmatter"`
	BodyIncluded bool                   `json:"body_included"`
}

// ContextPackResult is the full output of a ContextPack operation.
type ContextPackResult struct {
	TargetID        string             `json:"target_id"`
	BudgetTokens    int                `json:"budget_tokens"`
	UsedTokens      int                `json:"used_tokens"`
	BudgetExhausted bool               `json:"budget_exhausted"`
	Truncated       bool               `json:"truncated"`
	Target          *ContextPackTarget `json:"target"`
	Context         []ContextItem      `json:"context"`
}

// contextCandidate is an internal type for a ranked context edge.
type contextCandidate struct {
	noteID     string
	edgeType   string
	confidence string
	priority   int    // lower = higher priority
	updated    string // ISO date string from frontmatter; used for secondary sort (desc)
}

// edgePriority returns the sort priority for an edge type and confidence.
// explicit_relation(0) > explicit_link/embed(1) > medium confidence(2) > low confidence(3)
func edgePriority(edgeType, confidence string) int {
	if edgeType == "explicit_relation" {
		return 0
	}
	if edgeType == "explicit_link" || edgeType == "explicit_embed" {
		return 1
	}
	if confidence == "medium" {
		return 2
	}
	return 3
}

// ContextPack resolves the input, loads the target note, and fills a token budget
// with the target body and related note frontmatter in priority order.
func ContextPack(resolver *graph.Resolver, db *index.DB, cfg ContextPackConfig) (*ContextPackResult, error) {
	// Step 1: Resolve the input to a canonical note ID.
	resolved, err := resolver.Resolve(cfg.Input)
	if err != nil {
		return nil, fmt.Errorf("resolving input %q: %w", cfg.Input, err)
	}
	if !resolved.Resolved || len(resolved.Matches) == 0 {
		return nil, fmt.Errorf("could not resolve %q to a known note", cfg.Input)
	}
	targetID := resolved.Matches[0].ID

	result := &ContextPackResult{
		TargetID:     targetID,
		BudgetTokens: cfg.Budget,
		Context:      []ContextItem{},
	}

	// Step 2: Load target note.
	full, err := db.QueryFullNote(targetID)
	if err != nil {
		return nil, fmt.Errorf("querying full note %q: %w", targetID, err)
	}
	if full == nil {
		return nil, fmt.Errorf("note %q not found", targetID)
	}

	// Step 3: Estimate target tokens and fill budget.
	target, remaining := packTargetContent(full, cfg.Budget, result)
	result.Target = target

	if remaining <= 0 {
		return result, nil
	}

	// Step 4: Collect direct edge candidates.
	candidates, err := collectEdgeCandidates(db, targetID)
	if err != nil {
		return nil, err
	}

	// Step 5: Enrich with updated date and sort by (priority ASC, updated DESC).
	if err := enrichAndSortCandidates(db, candidates); err != nil {
		return nil, err
	}

	// Step 6: Pack context items until budget exhausted.
	for _, c := range candidates {
		noteFull, qErr := db.QueryFullNote(c.noteID)
		if qErr != nil {
			return nil, fmt.Errorf("querying context note %q: %w", c.noteID, qErr)
		}

		var fm map[string]interface{}
		if noteFull != nil {
			fm = noteFull.Frontmatter
		} else {
			fm = map[string]interface{}{}
		}

		fmBytes, _ := json.Marshal(fm)
		tokens := EstimateTokens(string(fmBytes))

		if tokens > remaining {
			result.BudgetExhausted = true
			break
		}

		result.Context = append(result.Context, ContextItem{
			ID:           c.noteID,
			EdgeType:     c.edgeType,
			Confidence:   c.confidence,
			Frontmatter:  fm,
			BodyIncluded: false,
		})

		remaining -= tokens
		result.UsedTokens += tokens
	}

	return result, nil
}

// packTargetContent fills the token budget with the target note's frontmatter and body.
// It always accounts for frontmatter tokens in UsedTokens and returns the target and
// the remaining token budget after packing.
func packTargetContent(full *index.FullNote, budget int, result *ContextPackResult) (*ContextPackTarget, int) {
	fmJSON, _ := json.Marshal(full.Frontmatter)
	fmTokens := EstimateTokens(string(fmJSON))
	bodyTokens := EstimateTokens(full.Body)

	target := &ContextPackTarget{
		ID:          full.ID,
		Frontmatter: full.Frontmatter,
	}

	// Always account for frontmatter tokens, even if they exceed the budget.
	result.UsedTokens += fmTokens
	remaining := budget - fmTokens

	if fmTokens > budget {
		// Frontmatter alone exceeds budget — count it, mark truncated, omit body.
		result.Truncated = true
		return target, remaining
	}

	if bodyTokens <= remaining {
		target.Body = full.Body
		remaining -= bodyTokens
		result.UsedTokens += bodyTokens
		return target, remaining
	}

	// Truncate body to fit remaining budget (4 chars per token).
	maxChars := remaining * 4
	if maxChars > 0 && len(full.Body) > 0 {
		body := full.Body
		if len(body) > maxChars {
			body = body[:maxChars]
			result.Truncated = true
		}
		target.Body = body
		used := EstimateTokens(body)
		remaining -= used
		result.UsedTokens += used
	} else {
		result.Truncated = true
	}
	return target, remaining
}

// collectEdgeCandidates queries outbound and inbound resolved edges for targetID
// and returns deduplicated candidates ranked by edgePriority.
func collectEdgeCandidates(db *index.DB, targetID string) ([]contextCandidate, error) {
	seen := make(map[string]bool)
	seen[targetID] = true // exclude the target itself

	var candidates []contextCandidate

	outRows, err := db.Query(
		`SELECT dst_note_id, edge_type, confidence
		 FROM links
		 WHERE src_note_id = ? AND resolved = TRUE AND dst_note_id IS NOT NULL`,
		targetID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying outbound edges: %w", err)
	}
	defer func() { _ = outRows.Close() }()

	for outRows.Next() {
		var c contextCandidate
		if scanErr := outRows.Scan(&c.noteID, &c.edgeType, &c.confidence); scanErr != nil {
			return nil, fmt.Errorf("scanning outbound edge: %w", scanErr)
		}
		if !seen[c.noteID] {
			seen[c.noteID] = true
			c.priority = edgePriority(c.edgeType, c.confidence)
			candidates = append(candidates, c)
		}
	}
	if err := outRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating outbound edges: %w", err)
	}

	inRows, err := db.Query(
		`SELECT src_note_id, edge_type, confidence
		 FROM links
		 WHERE dst_note_id = ? AND resolved = TRUE`,
		targetID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying inbound edges: %w", err)
	}
	defer func() { _ = inRows.Close() }()

	for inRows.Next() {
		var c contextCandidate
		if scanErr := inRows.Scan(&c.noteID, &c.edgeType, &c.confidence); scanErr != nil {
			return nil, fmt.Errorf("scanning inbound edge: %w", scanErr)
		}
		if !seen[c.noteID] {
			seen[c.noteID] = true
			c.priority = edgePriority(c.edgeType, c.confidence)
			candidates = append(candidates, c)
		}
	}
	if err := inRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating inbound edges: %w", err)
	}

	return candidates, nil
}

// enrichAndSortCandidates loads the updated date for each candidate from the DB
// and sorts candidates by (priority ASC, updated DESC).
func enrichAndSortCandidates(db *index.DB, candidates []contextCandidate) error {
	for i := range candidates {
		noteFull, err := db.QueryFullNote(candidates[i].noteID)
		if err != nil {
			return fmt.Errorf("querying context note %q for sort: %w", candidates[i].noteID, err)
		}
		if noteFull != nil {
			if u, ok := noteFull.Frontmatter["updated"].(string); ok {
				candidates[i].updated = u
			}
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		return candidates[i].updated > candidates[j].updated // desc: newer first
	})

	return nil
}
