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
	Input            string
	Budget           int               // token budget
	Depth            int               // BFS traversal depth; 0 or 1 = direct neighbors only (default)
	MaxItems         int               // max context items to return; 0 = unlimited (default, backward-compat)
	Slim             bool              // reduce context item frontmatter to {type, title, status} only
	ActivationScores map[string]float64 // optional activation scores keyed by note ID
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
	Body         string                 `json:"body,omitempty"`
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
	noteID          string
	edgeType        string
	confidence      string
	priority        int     // lower = higher priority
	updated         string  // ISO date string from frontmatter; used for tertiary sort (desc)
	activationScore float64 // ACT-R activation score; used for secondary sort (desc)
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

	// noteCache avoids redundant DB round-trips: QueryFullNote fires 5 sequential
	// queries per call, so caching halves the query count when a note appears in
	// both the frontmatter packing pass and the body backfill pass.
	noteCache := make(map[string]*index.FullNote)
	loadNote := func(id string) (*index.FullNote, error) {
		if cached, ok := noteCache[id]; ok {
			return cached, nil
		}
		fullN, loadErr := db.QueryFullNote(id)
		if loadErr != nil {
			return nil, loadErr
		}
		noteCache[id] = fullN
		return fullN, nil
	}

	// Step 2: Load target note.
	full, err := loadNote(targetID)
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

	// Step 4: Collect candidates — depth-1 (direct edges) or multi-hop BFS.
	var candidates []contextCandidate
	if cfg.Depth > 1 {
		candidates, err = collectTraverseCandidates(resolver, targetID, cfg.Depth)
	} else {
		candidates, err = collectEdgeCandidates(db, targetID)
	}
	if err != nil {
		return nil, err
	}

	// Step 5: Enrich with updated date and sort by (priority ASC, activationScore DESC, updated DESC).
	if err := enrichAndSortCandidates(loadNote, candidates, cfg.ActivationScores); err != nil {
		return nil, err
	}

	// Step 6: Pack context items until budget exhausted.
	if cfg.MaxItems > 0 {
		packBodyFirst(candidates, cfg, result, &remaining, loadNote)
	} else {
		if err := packTwoPass(candidates, cfg, result, &remaining, loadNote); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// packBodyFirst packs frontmatter + body together for each candidate (body-first single-pass).
// This yields fewer but richer items instead of many frontmatter-only skeletons.
// Used when MaxItems > 0.
func packBodyFirst(
	candidates []contextCandidate,
	cfg ContextPackConfig,
	result *ContextPackResult,
	remaining *int,
	loadNote func(string) (*index.FullNote, error),
) {
	for _, c := range candidates {
		if len(result.Context) >= cfg.MaxItems {
			break
		}

		noteFull, qErr := loadNote(c.noteID)
		if qErr != nil {
			// Non-fatal: skip unavailable notes rather than propagating error
			// (body-first mode is best-effort for agent consumption).
			continue
		}

		fm := extractFrontmatter(noteFull, cfg.Slim)
		fmBytes, _ := json.Marshal(fm)
		fmTokens := EstimateTokens(string(fmBytes))

		if fmTokens > *remaining {
			result.BudgetExhausted = true
			break
		}

		item := ContextItem{
			ID:          c.noteID,
			EdgeType:    c.edgeType,
			Confidence:  c.confidence,
			Frontmatter: fm,
		}
		*remaining -= fmTokens
		result.UsedTokens += fmTokens

		// Try to include body within remaining budget.
		if noteFull != nil && noteFull.Body != "" {
			bodyTokens := EstimateTokens(noteFull.Body)
			if bodyTokens <= *remaining {
				item.BodyIncluded = true
				item.Body = noteFull.Body
				*remaining -= bodyTokens
				result.UsedTokens += bodyTokens
			}
		}

		result.Context = append(result.Context, item)
	}
}

// packTwoPass packs all frontmatter first, then backfills bodies with remaining budget.
// This is the backward-compatible default behavior (MaxItems == 0).
func packTwoPass(
	candidates []contextCandidate,
	cfg ContextPackConfig,
	result *ContextPackResult,
	remaining *int,
	loadNote func(string) (*index.FullNote, error),
) error {
	for _, c := range candidates {
		noteFull, qErr := loadNote(c.noteID)
		if qErr != nil {
			return fmt.Errorf("querying context note %q: %w", c.noteID, qErr)
		}

		fm := extractFrontmatter(noteFull, cfg.Slim)
		fmBytes, _ := json.Marshal(fm)
		tokens := EstimateTokens(string(fmBytes))

		if tokens > *remaining {
			result.BudgetExhausted = true
			break
		}

		result.Context = append(result.Context, ContextItem{
			ID:          c.noteID,
			EdgeType:    c.edgeType,
			Confidence:  c.confidence,
			Frontmatter: fm,
		})

		*remaining -= tokens
		result.UsedTokens += tokens
	}

	// Body backfill pass: fill remaining budget with context item bodies in priority order.
	for i := range result.Context {
		if *remaining <= 0 {
			break
		}
		fullNote, err := loadNote(result.Context[i].ID)
		if err != nil || fullNote == nil || fullNote.Body == "" {
			continue
		}
		bodyTokens := EstimateTokens(fullNote.Body)
		if bodyTokens <= *remaining {
			result.Context[i].BodyIncluded = true
			result.Context[i].Body = fullNote.Body
			*remaining -= bodyTokens
			result.UsedTokens += bodyTokens
		}
	}
	return nil
}

// extractFrontmatter returns the note's frontmatter, optionally slimmed.
// Returns an empty map if the note is nil.
func extractFrontmatter(noteFull *index.FullNote, slim bool) map[string]interface{} {
	if noteFull == nil {
		return map[string]interface{}{}
	}
	fm := noteFull.Frontmatter
	if slim {
		var noteType, noteTitle string
		if t, ok := fm["type"].(string); ok {
			noteType = t
		}
		if t, ok := fm["title"].(string); ok {
			noteTitle = t
		}
		return slimFrontmatter(fm, noteType, noteTitle)
	}
	return fm
}

// slimFrontmatter returns a reduced frontmatter map containing only type, title,
// and status. This saves tokens in body-first packing mode.
func slimFrontmatter(fm map[string]interface{}, noteType, title string) map[string]interface{} {
	slim := map[string]interface{}{"type": noteType, "title": title}
	if s, ok := fm["status"]; ok {
		slim["status"] = s
	}
	return slim
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

// collectTraverseCandidates uses BFS traversal to collect candidates up to maxDepth hops.
// Priority is distance*10 + edgePriority so that closer nodes always rank higher than
// farther ones, with edge type as a tiebreaker within the same distance band.
func collectTraverseCandidates(resolver *graph.Resolver, targetID string, maxDepth int) ([]contextCandidate, error) {
	tResult, err := resolver.Traverse(graph.TraverseConfig{
		StartID:       targetID,
		MaxDepth:      maxDepth,
		MinConfidence: "low",
		MaxNodes:      200,
	})
	if err != nil {
		return nil, fmt.Errorf("BFS traversal from %q: %w", targetID, err)
	}

	var candidates []contextCandidate
	for _, node := range tResult.Nodes {
		if node.ID == targetID || node.Distance == 0 {
			continue // skip the start node itself
		}
		edgeType := ""
		confidence := ""
		if node.EdgeFrom != nil {
			edgeType = node.EdgeFrom.EdgeType
			confidence = node.EdgeFrom.Confidence
		}
		combined := node.Distance*10 + edgePriority(edgeType, confidence)
		candidates = append(candidates, contextCandidate{
			noteID:     node.ID,
			edgeType:   edgeType,
			confidence: confidence,
			priority:   combined,
		})
	}
	return candidates, nil
}

// enrichAndSortCandidates loads the updated date for each candidate using the
// provided loader (which may be cache-backed) and sorts candidates by
// (priority ASC, activationScore DESC, updated DESC).
// activationScores may be nil, in which case activation is not used for sorting.
func enrichAndSortCandidates(loader func(string) (*index.FullNote, error), candidates []contextCandidate, activationScores map[string]float64) error {
	for i := range candidates {
		noteFull, err := loader(candidates[i].noteID)
		if err != nil {
			return fmt.Errorf("querying context note %q for sort: %w", candidates[i].noteID, err)
		}
		if noteFull != nil {
			if u, ok := noteFull.Frontmatter["updated"].(string); ok {
				candidates[i].updated = u
			}
		}
		if activationScores != nil {
			candidates[i].activationScore = activationScores[candidates[i].noteID]
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		if candidates[i].activationScore != candidates[j].activationScore {
			return candidates[i].activationScore > candidates[j].activationScore // desc: higher activation first
		}
		return candidates[i].updated > candidates[j].updated // desc: newer first
	})

	return nil
}
