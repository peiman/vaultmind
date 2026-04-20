package experiment

import (
	"math"
	"testing"
)

func TestJaccardAtK_FullOverlap(t *testing.T) {
	a := []string{"n1", "n2", "n3", "n4", "n5"}
	b := []string{"n5", "n4", "n3", "n2", "n1"}
	if got := JaccardAtK(a, b, 5); got != 1.0 {
		t.Fatalf("expected 1.0 for identical sets at K=5, got %v", got)
	}
}

func TestJaccardAtK_NoOverlap(t *testing.T) {
	a := []string{"n1", "n2", "n3"}
	b := []string{"n4", "n5", "n6"}
	if got := JaccardAtK(a, b, 3); got != 0.0 {
		t.Fatalf("expected 0.0 for disjoint sets, got %v", got)
	}
}

func TestJaccardAtK_PartialOverlap(t *testing.T) {
	a := []string{"n1", "n2", "n3", "n4"}
	b := []string{"n3", "n4", "n5", "n6"}
	got := JaccardAtK(a, b, 4)
	want := 1.0 / 3.0
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestJaccardAtK_KSmallerThanLists(t *testing.T) {
	a := []string{"n1", "n2", "n3", "n4", "n5"}
	b := []string{"n1", "n2", "n9", "n4", "n5"}
	if got := JaccardAtK(a, b, 2); got != 1.0 {
		t.Fatalf("expected 1.0 at K=2, got %v", got)
	}
}

func TestJaccardAtK_EmptyLists(t *testing.T) {
	if got := JaccardAtK(nil, nil, 5); got != 1.0 {
		t.Fatalf("two empty lists should have Jaccard 1.0, got %v", got)
	}
	if got := JaccardAtK([]string{"n1"}, nil, 5); got != 0.0 {
		t.Fatalf("one empty list should yield Jaccard 0.0, got %v", got)
	}
}

func TestKendallTauShared_Identical(t *testing.T) {
	a := []string{"n1", "n2", "n3", "n4"}
	b := []string{"n1", "n2", "n3", "n4"}
	got, n := KendallTauShared(a, b)
	if got != 1.0 {
		t.Fatalf("identical ranks should give tau=1.0, got %v", got)
	}
	if n != 4 {
		t.Fatalf("expected 4 shared items, got %d", n)
	}
}

func TestKendallTauShared_Reversed(t *testing.T) {
	a := []string{"n1", "n2", "n3", "n4"}
	b := []string{"n4", "n3", "n2", "n1"}
	got, n := KendallTauShared(a, b)
	if got != -1.0 {
		t.Fatalf("reversed ranks should give tau=-1.0, got %v", got)
	}
	if n != 4 {
		t.Fatalf("expected 4 shared items, got %d", n)
	}
}

func TestKendallTauShared_SingleSwap(t *testing.T) {
	a := []string{"n1", "n2", "n3", "n4"}
	b := []string{"n2", "n1", "n3", "n4"}
	got, n := KendallTauShared(a, b)
	want := 4.0 / 6.0
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("expected %v, got %v", want, got)
	}
	if n != 4 {
		t.Fatalf("expected 4 shared items, got %d", n)
	}
}

func TestKendallTauShared_PartialOverlap(t *testing.T) {
	a := []string{"n1", "n2", "n3", "n4"}
	b := []string{"n2", "n1", "n9", "n8"}
	got, n := KendallTauShared(a, b)
	if got != -1.0 {
		t.Fatalf("expected -1.0 for fully swapped shared pair, got %v", got)
	}
	if n != 2 {
		t.Fatalf("expected 2 shared items, got %d", n)
	}
}

func TestKendallTauShared_LessThanTwoShared(t *testing.T) {
	a := []string{"n1", "n2"}
	b := []string{"n3", "n4"}
	got, n := KendallTauShared(a, b)
	if !math.IsNaN(got) {
		t.Fatalf("expected NaN when <2 shared items, got %v", got)
	}
	if n != 0 {
		t.Fatalf("expected 0 shared items, got %d", n)
	}
}

func TestExtractEventPairs_SingleShadow(t *testing.T) {
	eventData := map[string]any{
		"variants": map[string]any{
			"hybrid": map[string]any{
				"results": []any{
					map[string]any{"note_id": "n1", "rank": 1},
					map[string]any{"note_id": "n2", "rank": 2},
				},
			},
			"activation_v1": map[string]any{
				"results": []any{
					map[string]any{"note_id": "n2", "rank": 1},
					map[string]any{"note_id": "n1", "rank": 2},
				},
			},
		},
	}
	pairs, err := ExtractEventPairs(eventData, "hybrid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(pairs))
	}
	p := pairs[0]
	if p.PrimaryVariant != "hybrid" || p.ShadowVariant != "activation_v1" {
		t.Fatalf("unexpected pair labels: %+v", p)
	}
	if len(p.PrimaryList) != 2 || p.PrimaryList[0] != "n1" {
		t.Fatalf("unexpected primary list: %+v", p.PrimaryList)
	}
	if len(p.ShadowList) != 2 || p.ShadowList[0] != "n2" {
		t.Fatalf("unexpected shadow list: %+v", p.ShadowList)
	}
}

func TestExtractEventPairs_NoShadows(t *testing.T) {
	eventData := map[string]any{
		"variants": map[string]any{
			"hybrid": map[string]any{
				"results": []any{map[string]any{"note_id": "n1", "rank": 1}},
			},
		},
	}
	pairs, err := ExtractEventPairs(eventData, "hybrid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pairs) != 0 {
		t.Fatalf("expected 0 pairs when only primary present, got %d", len(pairs))
	}
}

func TestExtractEventPairs_PrimaryMissing(t *testing.T) {
	eventData := map[string]any{
		"variants": map[string]any{
			"activation_v1": map[string]any{
				"results": []any{map[string]any{"note_id": "n1", "rank": 1}},
			},
		},
	}
	_, err := ExtractEventPairs(eventData, "hybrid")
	if err == nil {
		t.Fatalf("expected error when primary variant is absent")
	}
}

func TestAggregateComparisons_TwoEventsOnePair(t *testing.T) {
	events := []ComparableEvent{
		{
			EventID: "e1",
			Pairs: []EventPair{
				{
					PrimaryVariant: "hybrid", ShadowVariant: "activation_v1",
					PrimaryList: []string{"n1", "n2", "n3"},
					ShadowList:  []string{"n1", "n2", "n3"},
				},
			},
		},
		{
			EventID: "e2",
			Pairs: []EventPair{
				{
					PrimaryVariant: "hybrid", ShadowVariant: "activation_v1",
					PrimaryList: []string{"n1", "n2", "n3"},
					ShadowList:  []string{"n3", "n2", "n1"},
				},
			},
		},
	}
	agg := AggregateComparisons(events, 3)
	if len(agg) != 1 {
		t.Fatalf("expected 1 aggregate row, got %d", len(agg))
	}
	row := agg[0]
	if row.PrimaryVariant != "hybrid" || row.ShadowVariant != "activation_v1" {
		t.Fatalf("unexpected labels: %+v", row)
	}
	if row.EventCount != 2 {
		t.Fatalf("expected EventCount=2, got %d", row.EventCount)
	}
	if math.Abs(row.MeanJaccardAtK-1.0) > 1e-9 {
		t.Fatalf("expected MeanJaccardAtK=1.0, got %v", row.MeanJaccardAtK)
	}
	if math.Abs(row.MeanKendallTau-0.0) > 1e-9 {
		t.Fatalf("expected MeanKendallTau=0.0, got %v", row.MeanKendallTau)
	}
	if row.KendallEventCount != 2 {
		t.Fatalf("expected KendallEventCount=2, got %d", row.KendallEventCount)
	}
}

func TestAggregateComparisons_SkipsInsufficientShared(t *testing.T) {
	events := []ComparableEvent{
		{
			EventID: "e1",
			Pairs: []EventPair{
				{
					PrimaryVariant: "hybrid", ShadowVariant: "activation_v1",
					PrimaryList: []string{"n1"},
					ShadowList:  []string{"n2"},
				},
			},
		},
	}
	agg := AggregateComparisons(events, 5)
	if len(agg) != 1 {
		t.Fatalf("expected 1 row, got %d", len(agg))
	}
	if agg[0].EventCount != 1 {
		t.Fatalf("EventCount should still count the event: got %d", agg[0].EventCount)
	}
	if agg[0].KendallEventCount != 0 {
		t.Fatalf("KendallEventCount should be 0 when no pair has >=2 shared: got %d", agg[0].KendallEventCount)
	}
	if !math.IsNaN(agg[0].MeanKendallTau) {
		t.Fatalf("MeanKendallTau should be NaN when no pair contributed, got %v", agg[0].MeanKendallTau)
	}
	if math.Abs(agg[0].MeanJaccardAtK-0.0) > 1e-9 {
		t.Fatalf("MeanJaccardAtK should be 0.0, got %v", agg[0].MeanJaccardAtK)
	}
}

func TestExtractEventPairs_MultipleShadows(t *testing.T) {
	eventData := map[string]any{
		"variants": map[string]any{
			"hybrid":        map[string]any{"results": []any{map[string]any{"note_id": "n1", "rank": 1}}},
			"activation_v1": map[string]any{"results": []any{map[string]any{"note_id": "n2", "rank": 1}}},
			"activation_v2": map[string]any{"results": []any{map[string]any{"note_id": "n3", "rank": 1}}},
		},
	}
	pairs, err := ExtractEventPairs(eventData, "hybrid")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pairs) != 2 {
		t.Fatalf("expected 2 shadow pairs, got %d", len(pairs))
	}
	if pairs[0].ShadowVariant != "activation_v1" || pairs[1].ShadowVariant != "activation_v2" {
		t.Fatalf("expected sorted shadow names, got %q %q", pairs[0].ShadowVariant, pairs[1].ShadowVariant)
	}
}
