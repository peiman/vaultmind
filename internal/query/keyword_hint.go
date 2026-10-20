package query

import (
	"fmt"
	"io"

	"github.com/peiman/vaultmind/internal/index"
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
	_, _ = fmt.Fprintln(w, "Paraphrase queries (e.g. 'how is login throttled' for a note titled")
	_, _ = fmt.Fprintln(w, "'Rate Limiting') won't match unless the query echoes a title word.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "To enable semantic retrieval:")
	// Backend-agnostic on purpose: `--embed` lets the binary pick its default
	// model (bge-m3 on ORT builds, minilm on the pure-Go binary `go install`
	// yields). Naming `--model bge-m3` here would be *refused* on a pure-Go
	// binary — the most common adopter path — turning a remedy into an error.
	_, _ = fmt.Fprintln(w, "  vaultmind index --embed --vault <vault>")
	return true
}

// WriteEmbedderDownNotice says, in the output the agent reads, that semantic
// search is down and why — when the vault HAS embeddings but the model would
// not load. Reports whether it wrote anything.
//
// It exists because the fallback used to be silent apart from a log line, and
// the only visible text was the keyword-only hint claiming the vault had no
// embeddings and prescribing a re-embed — which fails for the same reason
// (live, 2026-09-23: a stale ONNX runtime next to the binary).
func WriteEmbedderDownNotice(w io.Writer, err error) bool {
	if err == nil {
		return false
	}
	_, _ = fmt.Fprintf(w, "⚠ Semantic search is DOWN — keyword-only results. This vault has embeddings, "+
		"but the embedding model failed to load: %v\n", err)
	_, _ = fmt.Fprintln(w, "  Run `vaultmind doctor` to see which runtime was loaded and how to fix it.")
	return true
}

// WriteSparseFormatStaleNotice says the stored sparse vectors still count
// special tokens, and names the one command that rewrites them. The caller
// prints it before the results, on the output the reader sees.
//
// A hook caller is silent. UserPromptSubmit echoes this stdout into the
// agent on every turn. doctor, a plain index, and the SessionStart health
// hook print the same words on their own paths.
func WriteSparseFormatStaleNotice(w io.Writer) {
	if index.HookCaller() {
		return
	}
	_, _ = fmt.Fprintf(w, "⚠ %s\n", index.SparseFormatStaleNotice)
	_, _ = fmt.Fprintf(w, "  run: %s\n", index.SparseFormatStaleRemedy)
}

// WriteEmbedTextStaleNotice says stored vectors predate title embedding, and
// names the one command that rewrites them. The caller prints it before the
// results, on the output the reader sees.
//
// A hook caller is silent. UserPromptSubmit echoes this stdout into the
// agent on every turn. doctor, a plain index, and the SessionStart health
// hook print the same words on their own paths.
func WriteEmbedTextStaleNotice(w io.Writer) {
	if index.HookCaller() {
		return
	}
	_, _ = fmt.Fprintf(w, "⚠ %s\n", index.EmbedTextStaleNotice)
	_, _ = fmt.Fprintf(w, "  run: %s\n", index.EmbedTextStaleRemedy)
}
