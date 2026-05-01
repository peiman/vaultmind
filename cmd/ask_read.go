// ckeletin:allow-custom-command
//
// This file is not a command — it's a helper for ask.go's --read flag.
// The ultra-thin-command validator (ADR-001) flags it because of its
// location (`cmd/`); the whitelist comment opts out of that check. The
// logic is small and ask-specific (resolving --read rank/id and
// composing search + body-fetch + render), so keeping it next to ask.go
// rather than splitting into a new internal/ package is the leaner shape.
package cmd

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/retrieval"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// runAskRead implements `vaultmind ask <query> --read <N|id>`. The
// search runs (so the agent sees the menu they chose from), the named
// hit's body is fetched and printed inline, and access tracking fires
// on the chosen note only. Skips the full Ask context-pack assembly
// because the agent's intent is "read this specific hit", not "give me
// context around the top hit." Round-2 inter-agent review surfaced the
// missing third workflow shape between probe (--pointers-only) and
// full context-pack — this is it.
func runAskRead(cmd *cobra.Command, queryStr, readArg string, ret query.AutoRetrieverResult, vdb *cmdutil.VaultDB) error {
	hits, err := query.AskHits(cmd.Context(), ret.Retriever,
		queryStr,
		getConfigValueWithFlags[int](cmd, "search-limit", config.KeyAppAskSearchLimit),
	)
	if err != nil {
		return fmt.Errorf("ask --read: %w", err)
	}
	chosen, err := resolveAskReadTarget(hits.TopHits, readArg)
	if err != nil {
		return err
	}
	note, err := vdb.DB.QueryFullNote(chosen.ID)
	if err != nil {
		return fmt.Errorf("ask --read: querying %q: %w", chosen.ID, err)
	}
	if note == nil {
		return fmt.Errorf("ask --read: note %q resolved from search but missing from index", chosen.ID)
	}
	// Fire access on the deliberately-read note. CallerAgent because
	// --read is the explicit "I want this body" path — it's a
	// note-get-equivalent in terms of intent, just composed with a
	// preceding search.
	if recErr := index.RecordNoteAccessAs(vdb.DB, note.ID, index.CallerAgent); recErr != nil {
		log.Debug().Err(recErr).Str("note_id", note.ID).Msg("recording ask --read access failed (non-fatal)")
	}
	hits.RetrievalMode = retrievalModeLabel(ret)
	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppAskJson) {
		// JSON consumers get the same envelope as default ask, with the
		// chosen note attached as an extra field for the read-target.
		// Keep simple: emit the AskResult; downstream callers can fetch
		// the body via note get if they want JSON-structured body.
		return errAskReadJSONNotYetSupported
	}
	return query.FormatAskRead(hits, note, cmd.OutOrStdout())
}

// resolveAskReadTarget maps a --read argument (a 1-indexed rank or an
// exact id) to one of the search hits. Errors when the rank is out of
// range or the id isn't in the returned set — both are likely mistakes
// (the agent assumed something the menu didn't actually surface). The
// error message points at the recovery: re-run without --read to see
// the menu, or use `note get` for direct id lookup.
func resolveAskReadTarget(hits []retrieval.ScoredResult, arg string) (*retrieval.ScoredResult, error) {
	if len(hits) == 0 {
		return nil, fmt.Errorf("--read %q: no search hits to read from", arg)
	}
	if n, err := strconv.Atoi(arg); err == nil {
		if n < 1 || n > len(hits) {
			return nil, fmt.Errorf("--read %d: only %d hit(s) available (use 1-%d)", n, len(hits), len(hits))
		}
		return &hits[n-1], nil
	}
	for i := range hits {
		if hits[i].ID == arg {
			return &hits[i], nil
		}
	}
	return nil, fmt.Errorf("--read %q: id not in returned hits — re-run without --read to see the menu, or use `note get %s` for direct lookup", arg, arg)
}

// errAskReadJSONNotYetSupported is a sentinel for the case where
// --read is combined with --json. Default ask's JSON envelope shape
// doesn't naturally carry a "the user chose this rank" signal, and the
// JSON consumers we have are scripts/hooks that don't use --read. Punt
// until a real consumer asks; meanwhile fail loudly rather than emit a
// confusing partial envelope.
var errAskReadJSONNotYetSupported = errors.New("ask --read does not yet support --json output; use either --json (no --read) for the menu or omit --json for the inline-body view")
