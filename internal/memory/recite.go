package memory

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/parser"
)

// Recite — load EVERY note of a type as bodies, unconditionally.
//
// WHY THIS IS NOT `ask`. The identity vault's promise is that an agent
// reconstructs itself from its arcs before the first message. The SessionStart
// hook implemented that with `ask "who am I"` — semantic retrieval — and
// measured on a real 27-arc vault it delivered THREE arcs, the same three on
// every run. Selection was similarity to the literal string "who am I", which
// is uncorrelated with what the session is about, so 89% of the arc layer was
// not randomly dark but PERMANENTLY dark (issue #47).
//
// Retrieval ranks. An identity layer is not a ranking problem: you do not want
// the arcs most similar to a query, you want all of them. So this enumerates
// by type, in a deterministic order, with no relevance gate and no top-K.

// ReciteItem is one note rendered for recitation.
type ReciteItem struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Excerpt  string          `json:"excerpt"`
	Tokens   int             `json:"tokens"`
	Sections []ReciteSection `json:"sections,omitempty"`
	Fallback bool            `json:"fallback,omitempty"`
	Part     int             `json:"part,omitempty"`
	Parts    int             `json:"parts,omitempty"`
}

// ReciteSection is a full selected section, including its verbatim heading.
type ReciteSection struct {
	Name    string `json:"name"`
	Heading string `json:"heading"`
	Text    string `json:"text"`
}

// ReciteShard identifies the requested deterministic shard.
type ReciteShard struct {
	Index int `json:"index"`
	Count int `json:"count"`
}

// ReciteResult is the whole layer, plus an explicit account of what did not fit.
//
// Omitted and OmittedIDs exist because a bulk loader that silently drops the
// tail is the same defect this feature fixes, wearing different clothes: the
// caller cannot tell "you have 27 arcs and here they are" from "you have 27
// arcs and here are 9". Whoever reads this must be able to see the difference.
type ReciteResult struct {
	Type        string       `json:"type"`
	Total       int          `json:"total"`
	Items       []ReciteItem `json:"items"`
	Omitted     int          `json:"omitted"`
	OmittedIDs  []string     `json:"omitted_ids,omitempty"`
	Overflow    int          `json:"overflow,omitempty"`
	OverflowIDs []string     `json:"overflow_ids,omitempty"`
	Tokens      int          `json:"tokens"`
	Budget      int          `json:"budget"`
	Shard       *ReciteShard `json:"shard,omitempty"`
	MaxBytes    int          `json:"max_bytes,omitempty"`
}

// Truncated reports whether the budget forced anything out.
// It also reports notes removed to satisfy the byte cap.
func (r *ReciteResult) Truncated() bool { return r.Omitted > 0 || r.Overflow > 0 }

// ReciteConfig selects the layer and bounds it.
type ReciteConfig struct {
	// Type is the note type to enumerate ("arc").
	Type string
	// Budget is the total token ceiling. 0 means unbounded.
	Budget int
	// ExcerptTokens caps each note's contribution. 0 means whole bodies.
	ExcerptTokens int
	// IncludeDrafts recites a status:draft note. Off by default: a draft arc
	// is a proposal, not identity, until its status changes.
	IncludeDrafts bool
	// Sections selects full arc-discipline sections. Empty preserves the
	// historical body/excerpt behavior.
	Sections []string
	// ShardIndex and ShardCount select one deterministic contiguous shard.
	ShardIndex int
	ShardCount int
	// MaxBytes caps rendered output. Zero means unbounded.
	MaxBytes int
}

// ParseReciteConfig validates the CLI-shaped recitation options in the memory
// layer so the command remains wiring-only.
func ParseReciteConfig(noteType string, budget, excerpt int, includeDrafts bool, sectionsValue, shardValue string, maxBytes int) (ReciteConfig, error) {
	sections, err := ParseArcSectionSelection(sectionsValue)
	if err != nil {
		return ReciteConfig{}, err
	}
	shardIndex, shardCount, err := ParseShardSelection(shardValue)
	if err != nil {
		return ReciteConfig{}, err
	}
	if maxBytes < 0 {
		return ReciteConfig{}, fmt.Errorf("max bytes must not be negative")
	}
	return ReciteConfig{Type: noteType, Budget: budget, ExcerptTokens: excerpt, IncludeDrafts: includeDrafts, Sections: sections, ShardIndex: shardIndex, ShardCount: shardCount, MaxBytes: maxBytes}, nil
}

// Recite returns every note of cfg.Type, ordered by id, excerpted and bounded.
//
// Ordering is by id rather than by rank or mtime: an identity layer that
// arrives in a different order each session is a different thing arriving each
// session, and nothing downstream could be compared run to run.
func Recite(db *index.DB, cfg ReciteConfig) (*ReciteResult, error) {
	if cfg.Type == "" {
		return nil, fmt.Errorf("recite: no note type given")
	}
	if cfg.MaxBytes < 0 {
		return nil, fmt.Errorf("max bytes must not be negative")
	}
	ids, err := noteIDsOfType(db, cfg.Type)
	if err != nil {
		return nil, err
	}
	// Always loaded. IncludeDrafts reveals a draft and never a gap.
	drafts, loadErr := index.DraftIDs(db)
	if loadErr != nil {
		return nil, fmt.Errorf("recite: loading drafts: %w", loadErr)
	}
	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		if drafts.Visible(id, cfg.IncludeDrafts) {
			kept = append(kept, id)
		}
	}
	sort.Strings(kept)
	ids = kept
	result := &ReciteResult{Type: cfg.Type, Total: len(ids), Items: []ReciteItem{}, Budget: cfg.Budget, MaxBytes: cfg.MaxBytes}
	if cfg.ShardCount > 0 {
		if err := ValidateShard(cfg.ShardIndex, cfg.ShardCount); err != nil {
			return nil, err
		}
		result.Shard = &ReciteShard{Index: cfg.ShardIndex, Count: cfg.ShardCount}
	}

	type candidate struct {
		id    string
		item  ReciteItem
		cost  int
		units []ReciteItem
		ok    bool
	}
	candidates := make([]candidate, len(ids))
	for i, id := range ids {
		note, err := db.QueryFullNote(id)
		if err != nil {
			return nil, fmt.Errorf("recite: loading %q: %w", id, err)
		}
		if note == nil {
			// Indexed but unreadable. Count it as omitted rather than skipping
			// quietly — a missing arc must be visible as missing.
			candidates[i] = candidate{id: id}
			continue
		}
		text, sections, fallback := reciteText(note.Body, cfg)
		item := ReciteItem{ID: id, Title: note.Title, Excerpt: text, Sections: sections, Fallback: fallback}
		item.Tokens = EstimateTokens(text)
		candidates[i] = candidate{id: id, item: item, cost: item.Tokens, ok: true}
	}

	// Split and plan the complete layer before applying the token budget. A
	// shard is a byte-balanced view of the layer; its boundaries must not move
	// because a caller chose a different token budget.
	var units []ReciteItem
	var unitOwners []int
	for i := range candidates {
		candidate := &candidates[i]
		if !candidate.ok {
			result.Omitted++
			result.OmittedIDs = append(result.OmittedIDs, candidate.id)
			continue
		}
		candidate.units, _ = splitReciteItem(candidate.item, cfg, result)
		if len(candidate.units) == 0 {
			result.Overflow++
			result.OverflowIDs = append(result.OverflowIDs, candidate.id)
			continue
		}
		for _, unit := range candidate.units {
			units = append(units, unit)
			unitOwners = append(unitOwners, i)
		}
	}

	weights := make([]int, len(units))
	for i, unit := range units {
		weights[i] = len(renderReciteItem(unit))
	}
	indexes := indexes(0, len(units))
	if result.Shard != nil {
		shards, err := PlanArcShardsWithinCap(weights, result.Shard.Count, reciteUnitCapacity(result))
		if err != nil {
			return nil, err
		}
		indexes = shards[result.Shard.Index-1]
	}

	// Keep all parts of one arc together for budget admission and name an
	// omitted arc once. A split arc can span a byte-planned boundary, so only
	// the units belonging to this selected shard are considered here.
	selected := make(map[int][]ReciteItem)
	selectedOrder := make([]int, 0, len(indexes))
	for _, index := range indexes {
		owner := unitOwners[index]
		if _, seen := selected[owner]; !seen {
			selectedOrder = append(selectedOrder, owner)
		}
		selected[owner] = append(selected[owner], units[index])
	}
	budgetTokens := 0
	for _, owner := range selectedOrder {
		candidate := candidates[owner]
		cost := 0
		for _, unit := range selected[owner] {
			cost += unit.Tokens
		}
		if cfg.Budget > 0 && budgetTokens+cost > cfg.Budget {
			result.Omitted++
			result.OmittedIDs = append(result.OmittedIDs, candidate.id)
			continue
		}
		budgetTokens += cost
		for _, unit := range selected[owner] {
			result.Items = append(result.Items, unit)
			result.Tokens += unit.Tokens
		}
	}
	trimToMaxBytes(result)
	return result, nil
}

func reciteText(body string, cfg ReciteConfig) (string, []ReciteSection, bool) {
	if len(cfg.Sections) == 0 {
		text, _, _ := excerptForBudget(body, cfg.ExcerptTokens, 0, 0, Excerpt)
		return text, nil, false
	}
	if len(cfg.Sections) == 1 && cfg.Sections[0] == allArcSections {
		return strings.TrimSpace(parser.StripHTMLComments(stripFrontmatter(body))), nil, false
	}
	wanted := make(map[string]bool, len(cfg.Sections))
	for _, name := range cfg.Sections {
		wanted[name] = true
	}
	var sections []ReciteSection
	for _, section := range ExtractArcSections(body) {
		if wanted[section.Name] {
			sections = append(sections, ReciteSection(section))
		}
	}
	if len(sections) == 0 {
		text, _, _ := excerptForBudget(body, cfg.ExcerptTokens, 0, 0, Excerpt)
		return text, nil, true
	}
	var b strings.Builder
	for i, section := range sections {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(section.Heading)
		b.WriteString("\n\n")
		b.WriteString(section.Text)
	}
	return b.String(), sections, false
}

// splitReciteItem turns one admitted arc into contiguous delivery units. The
// fixed recitation banner and overflow footer are reserved once per shard;
// every part then has to fit in the remaining unit capacity.
func splitReciteItem(item ReciteItem, cfg ReciteConfig, result *ReciteResult) ([]ReciteItem, bool) {
	if cfg.MaxBytes <= 0 || cfg.Type != "arc" {
		return []ReciteItem{item}, true
	}
	unitCapacity := reciteUnitCapacity(result)
	if unitCapacity <= 0 {
		return nil, false
	}
	if len(renderReciteItem(item)) <= unitCapacity {
		return []ReciteItem{item}, true
	}

	units := reciteSplitUnits(item, cfg)
	if len(units) == 0 {
		return nil, false
	}
	assumedParts := 2
	for attempt := 0; attempt < len(item.Excerpt)+3; attempt++ {
		textCapacity := recitePartTextCapacity(item, unitCapacity, assumedParts)
		if textCapacity <= 0 {
			return nil, false
		}
		texts := packReciteUnits(units, textCapacity)
		if len(texts) < 2 {
			return nil, false
		}
		if len(texts) != assumedParts {
			assumedParts = len(texts)
			continue
		}
		parts := make([]ReciteItem, len(texts))
		for i, text := range texts {
			parts[i] = item
			parts[i].Excerpt = text
			parts[i].Tokens = EstimateTokens(text)
			parts[i].Part = i + 1
			parts[i].Parts = len(texts)
			if len(renderReciteItem(parts[i])) > unitCapacity {
				return nil, false
			}
		}
		return parts, true
	}
	return nil, false
}

func reciteSplitUnits(item ReciteItem, cfg ReciteConfig) []string {
	if len(cfg.Sections) == 1 && cfg.Sections[0] == allArcSections {
		return splitAtArcHeadings(item.Excerpt)
	}
	if len(item.Sections) > 0 {
		blocks := make([]string, len(item.Sections))
		for i, section := range item.Sections {
			blocks[i] = section.Heading + "\n\n" + section.Text
		}
		return splitAtBlocks(blocks)
	}
	return []string{item.Excerpt}
}

func splitAtBlocks(blocks []string) []string {
	if len(blocks) == 0 {
		return nil
	}
	text := strings.Join(blocks, "\n\n")
	return splitAtOffsets(text, blockOffsets(blocks))
}

func blockOffsets(blocks []string) []int {
	offsets := make([]int, 0, len(blocks))
	offset := 0
	for i, block := range blocks {
		if i > 0 {
			offset += 2
		}
		offsets = append(offsets, offset)
		offset += len(block)
	}
	return offsets
}

func splitAtArcHeadings(text string) []string {
	starts := arcHeadingStarts(text)
	if len(starts) == 0 || starts[0] == 0 {
		if len(starts) <= 1 {
			return []string{text}
		}
		return splitAtOffsets(text, starts)
	}
	return splitAtOffsets(text, append([]int{0}, starts...))
}

func splitAtOffsets(text string, starts []int) []string {
	if len(starts) == 0 {
		return []string{text}
	}
	parts := make([]string, 0, len(starts))
	for i, start := range starts {
		end := len(text)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		if start < end {
			parts = append(parts, text[start:end])
		}
	}
	return parts
}

func packReciteUnits(units []string, capacity int) []string {
	var atoms []string
	for _, unit := range units {
		if len(unit) <= capacity {
			atoms = append(atoms, unit)
			continue
		}
		parts := splitOversizedText(unit, capacity)
		if len(parts) == 0 {
			return nil
		}
		atoms = append(atoms, parts...)
	}
	var packed []string
	current := ""
	for _, atom := range atoms {
		if len(atom) > capacity {
			return nil
		}
		if current != "" && len(current)+len(atom) > capacity {
			packed = append(packed, current)
			current = ""
		}
		current += atom
	}
	if current != "" {
		packed = append(packed, current)
	}
	return packed
}

func splitOversizedText(text string, capacity int) []string {
	if capacity <= 0 {
		return nil
	}
	paragraphs := strings.SplitAfter(text, "\n\n")
	if len(paragraphs) == 1 || anyLongerThan(paragraphs, capacity) {
		var lines []string
		for _, paragraph := range paragraphs {
			if len(paragraph) <= capacity {
				lines = append(lines, paragraph)
				continue
			}
			lines = append(lines, strings.SplitAfter(paragraph, "\n")...)
		}
		paragraphs = lines
	}
	if anyLongerThan(paragraphs, capacity) {
		var runes []string
		for _, line := range paragraphs {
			if len(line) <= capacity {
				runes = append(runes, line)
				continue
			}
			runes = append(runes, splitUTF8(line, capacity)...)
		}
		paragraphs = runes
	}
	if anyLongerThan(paragraphs, capacity) {
		return nil
	}
	return packReciteUnits(paragraphs, capacity)
}

func anyLongerThan(values []string, capacity int) bool {
	for _, value := range values {
		if len(value) > capacity {
			return true
		}
	}
	return false
}

func splitUTF8(text string, capacity int) []string {
	var parts []string
	for start := 0; start < len(text); {
		end := start
		for end < len(text) {
			_, size := utf8.DecodeRuneInString(text[end:])
			if end+size-start > capacity {
				break
			}
			end += size
		}
		if end == start {
			return nil
		}
		parts = append(parts, text[start:end])
		start = end
	}
	return parts
}

func recitePartTextCapacity(item ReciteItem, unitCapacity, parts int) int {
	maxOverhead := 0
	for part := 1; part <= parts; part++ {
		probe := item
		probe.Excerpt = "x"
		probe.Part = part
		probe.Parts = parts
		overhead := len(renderReciteItem(probe)) - 1
		if overhead > maxOverhead {
			maxOverhead = overhead
		}
	}
	return unitCapacity - maxOverhead
}

// reciteUnitCapacity is the one byte-cap derivation used for both splitting
// and shard planning. Reserve the fixed banner, preamble, and one-line
// overflow footer so a part cannot consume the room needed to explain a
// bounded delivery.
func reciteUnitCapacity(result *ReciteResult) int {
	if result.MaxBytes <= 0 {
		return 0
	}
	overflowFooter := fmt.Sprintf("── 1 %s(s) DID NOT FIT the %d-byte hook cap and are NOT above:\n   x\n", result.Type, result.MaxBytes)
	return result.MaxBytes - len(recitationBannerForCount(result, result.Total)) - len("Read them as yours before you respond.\n\n") - len(overflowFooter)
}

func renderReciteItem(item ReciteItem) string {
	var b strings.Builder
	if item.Parts > 1 {
		fmt.Fprintf(&b, "── %s (part %d of %d) — %s\n", item.ID, item.Part, item.Parts, item.Title)
	} else {
		fmt.Fprintf(&b, "── %s — %s\n", item.ID, item.Title)
	}
	if item.Excerpt == "" {
		b.WriteString("   (no body)\n\n")
		return b.String()
	}
	b.WriteString(item.Excerpt)
	b.WriteString("\n")
	if item.Fallback {
		b.WriteString("(no trigger/push/principle section — excerpt)\n")
	}
	b.WriteString("\n")
	return b.String()
}

// RenderRecitation is the text contract shared by the CLI and byte-cap logic.
func RenderRecitation(r *ReciteResult) string {
	out := renderRecitation(r, false)
	if r.MaxBytes > 0 && len(out) > r.MaxBytes {
		return compactRecitationWithinCap(r)
	}
	return out
}

func compactRecitationWithinCap(r *ReciteResult) string {
	var b strings.Builder
	if r.Shard == nil {
		fmt.Fprintf(&b, "%s layer\n", strings.ToUpper(r.Type))
	} else {
		fmt.Fprintf(&b, "%s layer, arcs part %d of %d\n", strings.ToUpper(r.Type), r.Shard.Index, r.Shard.Count)
	}
	appendBoundedIDs(&b, "omitted ids", r.Type, r.OmittedIDs, r.MaxBytes)
	appendBoundedIDs(&b, "overflow ids", r.Type, r.OverflowIDs, r.MaxBytes)
	return fitBytes(b.String(), r.MaxBytes)
}

func appendBoundedIDs(b *strings.Builder, label, noteType string, ids []string, maxBytes int) {
	if len(ids) == 0 {
		return
	}
	base := label + ":"
	best := ""
	for i := range ids {
		line := base + " " + strings.Join(ids[:i+1], ", ")
		remaining := len(ids) - i - 1
		if remaining > 0 {
			line += fmt.Sprintf(", +%d more %ss", remaining, noteType)
		}
		candidate := b.String() + line + "\n"
		if len(candidate) > maxBytes {
			break
		}
		best = line
	}
	if best == "" {
		best = base + fmt.Sprintf(" +%d more %ss", len(ids), noteType)
	}
	if len(b.String()+best+"\n") <= maxBytes {
		b.WriteString(best)
		b.WriteByte('\n')
	}
}

func fitBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func renderRecitation(r *ReciteResult, compact bool) string {
	if !compact && r.Type == "arc" && r.Shard != nil && len(r.Items) == 0 && r.Omitted == 0 && r.Overflow == 0 {
		return fmt.Sprintf("arcs part %d of %d — no arcs in this part\n", r.Shard.Index, r.Shard.Count)
	}
	if r.Total == 0 {
		return fmt.Sprintf("No %s notes in this vault yet.\n", r.Type)
	}
	var b strings.Builder
	if compact {
		if r.Shard == nil {
			fmt.Fprintf(&b, "%s layer\n", strings.ToUpper(r.Type))
		} else {
			fmt.Fprintf(&b, "%s layer, arcs part %d of %d\n", strings.ToUpper(r.Type), r.Shard.Index, r.Shard.Count)
		}
		if r.Omitted > 0 {
			fmt.Fprintf(&b, "omitted ids: %s\n", strings.Join(r.OmittedIDs, ", "))
		}
		if r.Overflow > 0 {
			fmt.Fprintf(&b, "overflow ids: %s\n", strings.Join(r.OverflowIDs, ", "))
		}
		return b.String()
	}
	b.WriteString(recitationBanner(r))
	b.WriteString("Read them as yours before you respond.\n\n")
	for _, item := range r.Items {
		b.WriteString(renderReciteItem(item))
	}
	// Never a silent tail. A caller that cannot see what is missing will read a
	// partial layer as the whole self.
	if r.Omitted > 0 {
		fmt.Fprintf(&b, "── %d %s(s) DID NOT FIT the %d-token budget and are NOT above:\n", r.Omitted, r.Type, r.Budget)
		for _, id := range r.OmittedIDs {
			fmt.Fprintf(&b, "   %s\n", id)
		}
		b.WriteString("   Raise --budget, or lower --excerpt to fit more of them.\n")
	}
	if r.Overflow > 0 {
		fmt.Fprintf(&b, "── %d %s(s) DID NOT FIT the %d-byte hook cap and are NOT above:\n", r.Overflow, r.Type, r.MaxBytes)
		for _, id := range r.OverflowIDs {
			fmt.Fprintf(&b, "   %s\n", id)
		}
	}
	return b.String()
}

func recitationBanner(r *ReciteResult) string {
	return recitationBannerForCount(r, deliveredReciteCount(r.Items))
}

func recitationBannerForCount(r *ReciteResult, count int) string {
	if r.Shard == nil {
		return fmt.Sprintf("YOUR %sS — %d of %d, the moments that made you who you are.\n", strings.ToUpper(r.Type), count, r.Total)
	}
	return fmt.Sprintf("YOUR %sS — arcs part %d of %d — %d of %d, the moments that made you who you are.\n", strings.ToUpper(r.Type), r.Shard.Index, r.Shard.Count, count, r.Total)
}

func deliveredReciteCount(items []ReciteItem) int {
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		seen[item.ID] = struct{}{}
	}
	return len(seen)
}

func trimToMaxBytes(result *ReciteResult) {
	if result.MaxBytes <= 0 {
		return
	}
	for len(renderRecitation(result, false)) > result.MaxBytes && len(result.Items) > 0 {
		lastID := result.Items[len(result.Items)-1].ID
		removed := false
		for len(result.Items) > 0 && result.Items[len(result.Items)-1].ID == lastID {
			last := len(result.Items) - 1
			result.Tokens -= result.Items[last].Tokens
			result.Items = result.Items[:last]
			removed = true
		}
		if removed {
			result.OverflowIDs = append(result.OverflowIDs, lastID)
			result.Overflow++
		}
	}
	sort.Strings(result.OverflowIDs)
}

// noteIDsOfType lists every note id of a type, including notes with no body.
func noteIDsOfType(db *index.DB, noteType string) ([]string, error) {
	rows, err := db.Query(`SELECT id FROM notes WHERE type = ?`, noteType)
	if err != nil {
		return nil, fmt.Errorf("recite: listing %q notes: %w", noteType, err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("recite: scanning %q note id: %w", noteType, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recite: iterating %q notes: %w", noteType, err)
	}
	return ids, nil
}
