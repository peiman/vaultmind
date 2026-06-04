package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/noisefloor"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var askCmd = MustNewCommand(commands.AskMetadata, runAsk)

func init() {
	MustAddToRoot(askCmd)
}

// retrievalModeLabel reports the retriever kind for event logging. Ask uses an
// auto-selected retriever — hybrid when embeddings are available, keyword
// otherwise. Embedder presence is the signal.
func retrievalModeLabel(r query.AutoRetrieverResult) string {
	if r.Embedder != nil {
		return "hybrid"
	}
	return "keyword"
}

// resolveNoiseFloor picks the noise floor N and dispersion σ for the relevance
// label: the vault's measured calibration when it exists and clears the
// provisional gate (enough notes + pairs to be a trustworthy estimate), else the
// shipped per-embedder defaults. σ is always clamped so z stays finite.
//
// This is where the live label uses the vault's OWN measured floor instead of
// the shipped default. The clamp in noisefloor.Relevance keeps a tight vault's
// high measured N from silencing its own weak-but-real queries, which is what
// made driving the label off measured N safe. A nil session (telemetry off / no
// experiment DB) falls back to defaults — calibration is an enhancement, never a
// requirement for ask to run.
func resolveNoiseFloor(ctx context.Context, vaultPath string, dims int) (noiseFloor, sigma float64, lowContrast bool) {
	noiseFloor = noisefloor.DefaultNoiseFloor(dims)
	sigma = noisefloor.DefaultDispersion(dims)
	if session := experiment.FromContext(ctx); session != nil {
		key := canonicalVaultKey(vaultPath)
		snap, err := session.DB.LatestCalibrationForVault(key)
		switch {
		case err != nil:
			// Best-effort: a lookup failure (locked / corrupt DB) must never
			// break ask. Breadcrumb so "why isn't my calibration used?" is
			// diagnosable, not indistinguishable from "no calibration yet".
			log.Debug().Err(err).Str("vault", key).Msg("calibration lookup failed; using default noise floor")
		case snap != nil && snap.NoteCount >= noisefloor.MinCalibNotes && snap.NTNSampleCount >= noisefloor.MinCalibPairs:
			noiseFloor = snap.NoiseFloor
			sigma = snap.NTNCosineSigma
			// Tight vault (high note-to-note μ): even correct hits read "weak".
			// Surfaced so the formatter can explain a weak label rather than let
			// the agent misread it as "nothing relevant".
			lowContrast = noisefloor.IsTightVault(snap.NTNCosineMu)
		}
	}
	return noiseFloor, noisefloor.ClampSigma(sigma), lowContrast
}

// writeZeroHitDiagnostics emits user-facing hints when ask returns no hits.
// Non-fatal: a database error fetching titles is logged at debug and the
// function proceeds. The keyword-only hint always fires first when
// applicable; the title-suggestions block follows when matches exist.
func writeZeroHitDiagnostics(w io.Writer, db *index.DB, queryText, mode string, hitCount int) {
	query.WriteKeywordOnlyHint(w, mode, hitCount)
	if hitCount > 0 {
		return
	}
	titles, err := db.AllNoteTitles()
	if err != nil {
		// silent-failure-ok: fuzzy-title suggestions are an enhancement on
		// an already-reported zero-hit result. The keyword-only hint above
		// remains visible; users just don't get the optional "did you mean"
		// list. Nothing is lost.
		log.Debug().Err(err).Msg("could not load titles for zero-hit fallback")
		return
	}
	query.WriteTitleSuggestions(w, query.FuzzyTitleMatches(queryText, titles, 3))
}

func runAsk(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind ask <query>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppAskVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "ask")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	ret := query.BuildAutoRetrieverFull(vdb.DB)
	defer ret.Cleanup()

	resolver := graph.NewResolver(vdb.DB)
	delta := getConfigValueWithFlags[float64](cmd, "activation-delta", config.KeyExperimentsActivationDelta)

	// --read short-circuits the full Ask path. When set, we run the
	// search-only AskHits (which doesn't fire access tracking on the
	// top hit + neighbors), resolve the chosen rank/id, fetch its body,
	// and render the menu plus the chosen body. The deliberate-read
	// access fires on the chosen note specifically, avoiding the
	// mis-attribution that would happen if Ask packed context around
	// the top hit when the agent intended to read a different rank.
	if readArg := getConfigValueWithFlags[string](cmd, "read", config.KeyAppAskRead); readArg != "" {
		return runAskRead(cmd, args[0], readArg, ret, vdb)
	}

	activationScores := computeActivationScores(cmd.Context(), nil, delta)

	// Noise-floor relevance: when embeddings exist, derive the top-hit confidence
	// honestly from the vault's measured noise floor N and dispersion σ as
	// z = (top_cosine − N)/σ (see internal/noisefloor). resolveNoiseFloor uses the
	// vault's calibration when it's trustworthy, else the shipped per-embedder
	// default. Keyword-only mode (no embedder) keeps the RRF-gap fallback in Ask.
	noiseFloor, noiseFloorSigma, hasNoiseFloor := 0.0, 0.0, false
	lowContrast := false
	if ret.Embedder != nil {
		noiseFloor, noiseFloorSigma, lowContrast = resolveNoiseFloor(cmd.Context(), vaultPath, ret.Embedder.Dims())
		hasNoiseFloor = true
	}
	quietOnNoMatch := getConfigValueWithFlags[bool](cmd, "quiet-on-no-match", config.KeyAppAskQuietOnNomatch)

	result, err := query.Ask(cmd.Context(), ret.Retriever, resolver, vdb.DB, query.AskConfig{
		Query:             args[0],
		Budget:            getConfigValueWithFlags[int](cmd, "budget", config.KeyAppAskBudget),
		MaxItems:          getConfigValueWithFlags[int](cmd, "max-items", config.KeyAppAskMaxItems),
		SearchLimit:       getConfigValueWithFlags[int](cmd, "search-limit", config.KeyAppAskSearchLimit),
		ActivationScores:  activationScores,
		Embedder:          ret.Embedder,
		NoiseFloor:        noiseFloor,
		NoiseFloorSigma:   noiseFloorSigma,
		HasNoiseFloor:     hasNoiseFloor,
		VaultLowContrast:  lowContrast,
		SuppressOnNoMatch: quietOnNoMatch,
		ActivationFunc: func(sims map[string]float64) map[string]float64 {
			return computeActivationScores(cmd.Context(), sims, delta)
		},
	})

	mode := retrievalModeLabel(ret)
	if result != nil {
		result.RetrievalMode = mode
	}

	// Recall floor: nothing relevant (honest noise-floor no_match) and the
	// caller asked to be quiet. `suppressed` gates BOTH the reinforcement
	// telemetry (don't record access on the irrelevant top hit) and the text
	// output (inject silence). Gated on NoiseFloorApplied so an RRF-gap "tied"
	// no_match — which means "no clear winner", not "nothing relevant" — never
	// triggers it.
	suppressed := quietOnNoMatch && result != nil && result.NoiseFloorApplied &&
		result.TopHitConfidence == query.ConfidenceNoMatch

	logAskExperiment(cmd, args[0], vaultPath, mode, result, err, suppressed)

	if err != nil {
		return fmt.Errorf("ask: %w", err)
	}

	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppAskJson)
	// Suppress only the human-readable recall noise. A --json consumer still
	// gets the envelope (with no_match + nil context) so the signal isn't lost.
	if suppressed && !jsonOut {
		return nil
	}

	if !jsonOut {
		formatter := query.FormatAsk
		// Flags are mutually exclusive in semantics. Precedence (strongest
		// constraint first):
		//   --pointers-only  — strips all bodies; forces the ask-to-read loop
		//   --preview        — adds a 1-line snippet under each ranked hit
		//                      (bridges titles ↔ full body)
		//   --explain        — shows lane math under each hit
		// pointers-only wins over preview when both are set (pointers-only
		// is the stricter "no body content at all" promise).
		switch {
		case getConfigValueWithFlags[bool](cmd, "pointers-only", config.KeyAppAskPointersOnly):
			formatter = query.FormatAskPointersOnly
		case getConfigValueWithFlags[bool](cmd, "preview", config.KeyAppAskPreview):
			formatter = query.FormatAskPreview
		case getConfigValueWithFlags[bool](cmd, "explain", config.KeyAppAskExplain):
			formatter = query.FormatAskExplain
		}
		if err := formatter(result, cmd.OutOrStdout()); err != nil {
			return err
		}
		writeZeroHitDiagnostics(cmd.OutOrStdout(), vdb.DB, args[0], mode, len(result.TopHits))
		return nil
	}
	env := envelope.OK("ask", result)
	env.Meta.VaultPath = vaultPath
	env.Meta.IndexHash = vdb.GetIndexHash()
	return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
}
