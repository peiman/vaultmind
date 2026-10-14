package query

import "strings"

// QueryCapped reports that ranking used the first UsedWords of TotalWords.
// It is present on an ask result only when the cap shortened the query.
type QueryCapped struct {
	UsedWords  int `json:"used_words"`
	TotalWords int `json:"total_words"`
}

// CapRankingQuery is the only place the ranking-query shape is defined.
// maxWords <= 0 turns the cap off and returns text unchanged with a nil report.
// A query of at most maxWords words is returned unchanged, whitespace included.
// A longer query is the first maxWords words joined by a single space.
func CapRankingQuery(text string, maxWords int) (string, *QueryCapped) {
	if maxWords <= 0 {
		return text, nil
	}
	fields := strings.Fields(text)
	if len(fields) <= maxWords {
		return text, nil
	}
	return strings.Join(fields[:maxWords], " "), &QueryCapped{
		UsedWords:  maxWords,
		TotalWords: len(fields),
	}
}
