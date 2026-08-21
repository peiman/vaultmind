package hooks

import "sort"

// sortStrings keeps RecordableNames deterministic — a help string or an error
// message that reorders between runs is the same class of defect as generated
// docs that reshuffle: it cannot be diffed, so it cannot be gated.
func sortStrings(s []string) { sort.Strings(s) }
