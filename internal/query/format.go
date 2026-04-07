package query

import (
	"fmt"
	"io"
)

// FormatAsk writes a human-readable text representation of an AskResult.
func FormatAsk(result *AskResult, w io.Writer) error {
	if _, err := fmt.Fprintf(w, "Search: %q (%d hits)\n", result.Query, len(result.TopHits)); err != nil {
		return err
	}
	for _, h := range result.TopHits {
		if _, err := fmt.Fprintf(w, "  %.2f  %-40s  %s\n", h.Score, h.ID, h.Title); err != nil {
			return err
		}
	}
	if result.Context == nil {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\nContext from: %s (%d items, %d/%d tokens)\n",
		result.Context.TargetID, len(result.Context.Context),
		result.Context.UsedTokens, result.Context.BudgetTokens); err != nil {
		return err
	}
	if result.Context.Target != nil {
		noteType, _ := result.Context.Target.Frontmatter["type"].(string)
		title, _ := result.Context.Target.Frontmatter["title"].(string)
		if _, err := fmt.Fprintf(w, "  [%s] %s\n", noteType, title); err != nil {
			return err
		}
		if result.Context.Target.Body != "" {
			if _, err := fmt.Fprintf(w, "    %s\n", Truncate(result.Context.Target.Body, 120)); err != nil {
				return err
			}
		}
	}
	for _, item := range result.Context.Context {
		noteType, _ := item.Frontmatter["type"].(string)
		title, _ := item.Frontmatter["title"].(string)
		if _, err := fmt.Fprintf(w, "  [%s] %s\n", noteType, title); err != nil {
			return err
		}
		if item.BodyIncluded && item.Body != "" {
			if _, err := fmt.Fprintf(w, "    %s\n", Truncate(item.Body, 120)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Truncate shortens a string to maxLen runes, appending "..." if truncated.
func Truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
