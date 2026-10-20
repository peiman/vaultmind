package index

import (
	"fmt"
	"os"
	"strings"
)

// noteEmbedText is the one string a stored note becomes before it is
// embedded (Principle 7). An empty title leaves the body. Otherwise the
// title is the first line the model sees, then a blank line, then the body.
func noteEmbedText(title, body string) string {
	if strings.TrimSpace(title) == "" {
		return body
	}
	return title + "\n\n" + body
}

// noteText is one note waiting to be embedded.
type noteText struct {
	id    string
	path  string
	title string
	body  string
}

func noteIDs(batch []noteText) []string {
	ids := make([]string, len(batch))
	for i := range batch {
		ids[i] = batch[i].id
	}
	return ids
}

// appendUnrewritten keeps the ids a title upgrade must clear. A pass that is
// not an upgrade leaves stored vectors alone, so it records nothing.
func appendUnrewritten(upgrade bool, dst, ids []string) []string {
	if !upgrade || len(ids) == 0 {
		return dst
	}
	return append(dst, ids...)
}

// embedNoteQueries selects the notes EmbedNotes will embed, and the count of
// notes it will report as already done. all selects every note: a title
// upgrade rewrites vectors in place, and those notes are not "skipped".
func embedNoteQueries(full, all bool) (pending, skip string) {
	cols := "SELECT id, path, COALESCE(title, ''), COALESCE(body_text, '') FROM notes"
	if all {
		return cols, "SELECT 0"
	}
	if full {
		return cols + " WHERE embedding IS NULL OR sparse_embedding IS NULL OR colbert_embedding IS NULL",
			"SELECT COUNT(*) FROM notes WHERE embedding IS NOT NULL AND sparse_embedding IS NOT NULL AND colbert_embedding IS NOT NULL"
	}
	return cols + " WHERE embedding IS NULL",
		"SELECT COUNT(*) FROM notes WHERE embedding IS NOT NULL"
}

// planNoteBatches sizes batches by the text the embedder will receive.
// Planning on the body alone and then sending title+body is what made the
// memory guard refuse notes that the plan had said would fit.
func (idx *Indexer) planNoteBatches(pending []noteText, full bool) ([][]int, error) {
	if err := idx.announceTitleReembed(len(pending)); err != nil {
		return nil, err
	}
	lengths := make([]int, len(pending))
	for i, nt := range pending {
		lengths[i] = len(noteEmbedText(nt.title, nt.body))
	}
	if full {
		return planBGEM3Batches(lengths), nil
	}
	return planEmbedBatches(lengths, embedBatchSize, embedBatchSize*embedPaddedBudgetPerSlot), nil
}

func (idx *Indexer) announceTitleReembed(n int) error {
	if !idx.reembedAll || n == 0 {
		return nil
	}
	w := idx.Progress
	if w == nil {
		w = os.Stderr
	}
	if _, err := fmt.Fprintf(w, "re-embedding %d notes (titles are now embedded)\n", n); err != nil {
		return fmt.Errorf("reporting title re-embed: %w", err)
	}
	return nil
}
