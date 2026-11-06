package memory

import "github.com/peiman/vaultmind/internal/passage"

// EstimateTokens estimates the number of LLM tokens for the given content.
// Uses the heuristic of 4 characters per token with ceiling division.
func EstimateTokens(content string) int {
	return passage.EstimateTokens(content)
}
