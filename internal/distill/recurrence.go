package distill

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/peiman/vaultmind/internal/embedding"
)

// recurrenceThreshold is the cosine above which two proposals are treated as
// being about the same thing. Deliberately high: the cost of a missed
// recurrence is one unnoticed pattern, while the cost of a loose one is a
// report that groups everything and therefore says nothing.
const recurrenceThreshold = 0.80

// recurrenceTextMax bounds the text embedded per proposal. Long verbatim pushes
// made the first run take three minutes; the opening of a message carries the
// shape, and the tail is usually logistics.
const recurrenceTextMax = 600

// RecurrenceItem is one proposal considered for recurrence, tagged with the
// source it came from. Source is what makes the rule work — the signal is a
// shape crossing session boundaries, so items from the same source can never
// establish one between them.
type RecurrenceItem struct {
	Source string
	Text   string
	// Trigger is the phrase that made this a candidate, when one did. A group
	// whose members ALL share one trigger is explained by that phrase rather
	// than by a recurring transformation, and is dropped — see FindRecurrences.
	Trigger string
}

// RecurrenceGroup is one shape that recurred across several sources.
type RecurrenceGroup struct {
	Members     []string `json:"members"`
	Sources     []string `json:"sources"`
	SourceCount int      `json:"source_count"`
}

// Vectorizer turns proposal texts into vectors. An interface so this package
// keeps no dependency on the embedding stack, and so the clustering can be
// tested without a model.
type Vectorizer interface {
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
}

// FindRecurrences groups proposals that talk about the same thing and returns
// the groups spanning at least minSources distinct sources, broadest first.
//
// This is Rule 2 from the 2026-05-31 distillation review — "the proven money
// rule; the one thing a per-episode reader cannot do". A single instance of a
// recurring failure looks unremarkable; the fifth is a structural finding. Only
// something reading ACROSS sources can tell those apart.
//
// Only the counting is mechanised. What a recurrence means, and whether it
// clears the arc bar, stays with the mind — the review found that judgement
// unreliable to automate, and nothing here attempts it.
//
// A nil vectorizer yields no groups and no error: recurrence detection is an
// enhancement, and a vault without embeddings must still get its proposals.
func FindRecurrences(ctx context.Context, items []RecurrenceItem, vz Vectorizer, minSources int) ([]RecurrenceGroup, error) {
	if vz == nil || len(items) < 2 || minSources < 2 {
		return nil, nil
	}

	items = dedupeIdenticalText(items)
	if len(items) < 2 {
		return nil, nil
	}
	texts := make([]string, len(items))
	for i, it := range items {
		texts[i] = headOf(it.Text, recurrenceTextMax)
	}
	vecs, err := vz.EmbedTexts(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embedding proposals for recurrence: %w", err)
	}
	if len(vecs) != len(items) {
		return nil, fmt.Errorf("embedding returned %d vectors for %d proposals", len(vecs), len(items))
	}

	// Single-link agglomeration via union-find: A and C join the same shape when
	// each is close to B, even if they drifted apart in wording. That matches
	// how a recurring failure actually presents — the vocabulary changes as the
	// understanding does, while the underlying shape holds.
	parent := make([]int, len(items))
	for i := range parent {
		parent[i] = i
	}
	// Iterative with path compression, so no self-reference is needed.
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if embedding.CosineSimilarity(vecs[i], vecs[j]) >= recurrenceThreshold {
				if ri, rj := find(i), find(j); ri != rj {
					parent[ri] = rj
				}
			}
		}
	}

	clusters := map[int][]int{}
	for i := range items {
		root := find(i)
		clusters[root] = append(clusters[root], i)
	}

	groups := make([]RecurrenceGroup, 0, len(clusters))
	for _, members := range clusters {
		sources := distinctSources(items, members)
		if len(sources) < minSources {
			continue
		}
		if explainedByTrigger(items, members) {
			continue
		}
		texts := make([]string, 0, len(members))
		for _, m := range members {
			texts = append(texts, items[m].Text)
		}
		sort.Strings(texts)
		groups = append(groups, RecurrenceGroup{
			Members: texts, Sources: sources, SourceCount: len(sources),
		})
	}

	// Broadest first: a shape crossing five sessions is a stronger structural
	// claim than one crossing two, so it earns the reader's attention first.
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].SourceCount != groups[j].SourceCount {
			return groups[i].SourceCount > groups[j].SourceCount
		}
		return strings.Join(groups[i].Sources, ",") < strings.Join(groups[j].Sources, ",")
	})
	return groups, nil
}

// distinctSources lists the unique sources a cluster spans, sorted. Repeats
// within one source collapse to one entry — the same moment mentioned twice is
// not a pattern across time.
func distinctSources(items []RecurrenceItem, members []int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(members))
	for _, m := range members {
		if s := items[m].Source; s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// dedupeIdenticalText drops proposals whose text is byte-identical to one
// already seen.
//
// Identical text is never evidence of recurrence — it is one message counted
// twice. The real corpus produces these: a compaction summary replays an
// earlier turn, so the same push is captured under two episode ids and looks
// like a shape crossing sessions. The first run reported exactly that as a
// finding, which is the kind of false positive that teaches a reader to stop
// trusting the section.
func dedupeIdenticalText(items []RecurrenceItem) []RecurrenceItem {
	seen := map[string]bool{}
	out := make([]RecurrenceItem, 0, len(items))
	for _, it := range items {
		key := strings.Join(strings.Fields(it.Text), " ")
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, it)
	}
	return out
}

// explainedByTrigger reports whether every member of a cluster fired on the
// same trigger phrase.
//
// Such a group is the detector finding its own lexeme. Run over the real
// corpus, the first version reported two "recurrences" that were only the
// partner saying "manifesto lens on" in different sessions — true, and not a
// transformation. The signal Rule 2 exists to catch lives in what the AGENT
// kept doing, not in the phrasing that surfaced it, so a cluster held together
// by a shared trigger is discarded rather than presented as structural.
func explainedByTrigger(items []RecurrenceItem, members []int) bool {
	first := strings.ToLower(strings.TrimSpace(items[members[0]].Trigger))
	if first == "" {
		return false // desk entries carry no trigger; they are never suppressed
	}
	for _, m := range members[1:] {
		if strings.ToLower(strings.TrimSpace(items[m].Trigger)) != first {
			return false
		}
	}
	return true
}
