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
func runAskRead(cmd *cobra.Command, queryStr, readArg string, ret query.AutoRetrieverResult, vdb *cmdutil.VaultDB, fed federatedRead) error {
	hits, err := query.AskHits(cmd.Context(), ret.Retriever,
		queryStr,
		getConfigValueWithFlags[int](cmd, "search-limit", config.KeyAppAskSearchLimit),
	)
	if err != nil {
		return fmt.Errorf("ask --read: %w", err)
	}
	// Under federation the menu the agent was shown IS the cross-vault
	// ranking, so that is the list --read must index into. Re-searching the
	// owning vault here made rank N mean two different notes: the menu said
	// one thing and --read delivered another, from another vault, silently.
	if len(fed.hits) > 0 {
		hits = fed.asAskResult(queryStr, hits.RetrievalMode)
	}
	chosen, err := resolveAskReadTarget(hits.TopHits, readArg)
	if err != nil {
		return err
	}
	// The chosen note may live in a vault other than the one that delivered
	// the top hit, so its body and its access record belong to ITS vault.
	readDB := vdb.DB
	if ownerPath, ok := fed.vaultPathFor(chosen.ID); ok && ownerPath != fed.ownerPath {
		owned, openErr := cmdutil.OpenVaultDB(ownerPath)
		if openErr != nil {
			return fmt.Errorf("ask --read: opening vault %s for %q: %w", ownerPath, chosen.ID, openErr)
		}
		defer owned.Close()
		readDB = owned.DB
	}
	note, err := readDB.QueryFullNote(chosen.ID)
	if err != nil {
		return fmt.Errorf("ask --read: querying %q: %w", chosen.ID, err)
	}
	if note == nil {
		return fmt.Errorf("ask --read: note %q resolved from search but missing from index", chosen.ID)
	}
	hits.RetrievalMode = retrievalModeLabel(ret)
	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppAskJson) {
		// JSON consumers get the same envelope as default ask, with the
		// chosen note attached as an extra field for the read-target.
		// Keep simple: emit the AskResult; downstream callers can fetch
		// the body via note get if they want JSON-structured body.
		return errAskReadJSONNotYetSupported
	}

	// Fire access AFTER the JSON guard, and record the delivery explicitly.
	// It used to fire above it, so `ask --read --json` — which returns an error
	// and prints nothing — still logged an access; and it used the nil-delivery
	// recorder, so the row landed NULL and was counted as delivered by the
	// legacy caller='agent' fallback. That fallback exists for rows written
	// before delivery was tracked, not for new ones. --read renders the body a
	// few lines below, so true is the measured answer, not an assumption.
	if recErr := index.RecordNoteAccessDelivered(readDB, note.ID, index.CallerAgent, true); recErr != nil {
		log.Debug().Err(recErr).Str("note_id", note.ID).Msg("recording ask --read access failed (non-fatal)")
	}
	// --read + --explain compose: the menu shows per-lane RRF math
	// under each hit (so the agent can see why their chosen rank
	// ranked where it did), then the chosen body renders below.
	// Round-3 evaluator flagged the silent-drop pre-fix as the worst
	// of three options; explicit composition is the fix.
	explain := getConfigValueWithFlags[bool](cmd, "explain", config.KeyAppAskExplain)
	return query.FormatAskReadWithOptions(hits, note, cmd.OutOrStdout(), explain)
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

// federatedRead carries the cross-vault ranking into the --read path.
//
// It exists because --read used to re-search the owning vault and index into
// THAT list while the agent had been shown the federated one, so rank N named
// two different notes. Passing the shown ranking explicitly makes the menu and
// the read target the same list by construction rather than by coincidence.
type federatedRead struct {
	hits   []query.FederatedHit
	vaults []query.FederatedVaultStatus
	// paths are the vaults searched, used to map a hit's display name back to
	// the directory its body must be read from.
	paths []string
	// ownerPath is the vault already open for delivery; a hit owned by that
	// vault needs no second handle.
	ownerPath string
}

// asAskResult presents the cross-vault ranking as the menu, carrying the
// federation block so --read still reports what was searched.
func (f federatedRead) asAskResult(queryStr, retrievalMode string) *query.AskResult {
	top := make([]retrieval.ScoredResult, 0, len(f.hits))
	for _, h := range f.hits {
		top = append(top, h.ScoredResult)
	}
	return &query.AskResult{
		Query:           queryStr,
		TopHits:         top,
		RetrievalMode:   retrievalMode,
		Federated:       f.hits,
		FederatedVaults: f.vaults,
	}
}

// vaultPathFor returns the directory owning a note id in the merged ranking.
func (f federatedRead) vaultPathFor(id string) (string, bool) {
	for _, h := range f.hits {
		if h.ID != id {
			continue
		}
		for _, p := range f.paths {
			if vaultDisplayName(p) == h.Vault {
				return p, true
			}
		}
		return "", false
	}
	return "", false
}
