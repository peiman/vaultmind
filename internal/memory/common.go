package memory

// EstimateTokens estimates the number of LLM tokens for the given content.
// Uses the heuristic of 4 characters per token with ceiling division.
func EstimateTokens(content string) int {
	n := len(content)
	if n == 0 {
		return 0
	}
	return (n + 3) / 4
}
