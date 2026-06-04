package query

import (
	"fmt"
	"io"
)

// WriteKeywordOnlyHint writes a user-facing diagnostic to w when the ask
// retrieval ran in keyword-only mode AND returned zero hits. That combination
// signals a vault without embeddings — paraphrase queries cannot match and
// the user has no other feedback explaining the silence. Reports whether
// the hint was written.
//
// Silent on hybrid mode (different problem — real zero-hit) and silent when
// keyword search actually found results (user got what they asked for).
func WriteKeywordOnlyHint(w io.Writer, retrievalMode string, hitCount int) bool {
	if retrievalMode != "keyword" || hitCount != 0 {
		return false
	}
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Note: this vault has no embeddings — running keyword search only.")
	_, _ = fmt.Fprintln(w, "Paraphrase queries (e.g. 'how do I mislead myself' for an arc titled")
	_, _ = fmt.Fprintln(w, "'The Judgment Gap') won't match unless the query echoes a title word.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "To enable semantic retrieval:")
	_, _ = fmt.Fprintln(w, "  vaultmind index --embed --model bge-m3 --vault <vault>")
	return true
}
