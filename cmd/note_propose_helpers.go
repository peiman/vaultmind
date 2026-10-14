package cmd

import (
	"context"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/propose"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

// These references are the namespace declaration: this file owns both
// commands, and it does not read app.ask. Ranking bounds stay in ask_helpers.go.
var (
	_ = commands.NoteProposeMetadata
	_ = commands.NoteAcceptMetadata
)

func runPropose(cmd *cobra.Command) error {
	v, req, cleanup, err := proposeRequest(cmd)
	if err != nil {
		return err
	}
	defer cleanup()
	if v.DB != nil {
		req.Ranker = newAskRanker(cmd, v.DB, v.Path)
	}
	return propose.Run(cmd.Context(), v, req, newAskClosest(cmd))
}

func runAccept(cmd *cobra.Command, args []string) error {
	v, req, cleanup, err := acceptRequest(cmd, args)
	if err != nil {
		return err
	}
	defer cleanup()
	return propose.Accept(cmd.Context(), v, req)
}

func proposeRequest(cmd *cobra.Command) (propose.Vault, propose.Request, func(), error) {
	req := propose.Request{
		Question:  getConfigValueWithFlags[string](cmd, "question", config.KeyAppNoteproposeQuestion),
		Finding:   getConfigValueWithFlags[string](cmd, "finding", config.KeyAppNoteproposeFinding),
		CloseBand: getConfigValueWithFlags[string](cmd, "close-band", config.KeyAppNoteproposeCloseBand),
		JSON:      getConfigValueWithFlags[bool](cmd, "json", config.KeyAppNoteproposeJson),
		Stdout:    cmd.OutOrStdout(),
		Stderr:    cmd.ErrOrStderr(),
	}
	// repeatedValues, not GetStringSlice: resetting a StringSlice flag to its
	// default "[]" parses that default as a source named "[]".
	req.Sources = repeatedValues(cmd, "source")
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppNoteproposeVault)
	return openProposeVault(cmd, vaultPath, "note propose", req)
}

func acceptRequest(cmd *cobra.Command, args []string) (propose.Vault, propose.Request, func(), error) {
	req := propose.Request{
		JSON:   getConfigValueWithFlags[bool](cmd, "json", config.KeyAppNoteacceptJson),
		Stdout: cmd.OutOrStdout(),
		Stderr: cmd.ErrOrStderr(),
	}
	if len(args) > 0 {
		req.ID = args[0]
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppNoteacceptVault)
	return openProposeVault(cmd, vaultPath, "note accept", req)
}

func openProposeVault(cmd *cobra.Command, vaultPath, command string, req propose.Request) (propose.Vault, propose.Request, func(), error) {
	noop := func() {}
	req.Question = strings.TrimSpace(req.Question)
	req.Finding = strings.TrimSpace(req.Finding)
	// A missing question or finding is the caller's error. Opening a guessed
	// vault first would create an index in the wrong directory.
	if command == "note propose" && (req.Question == "" || req.Finding == "") {
		return propose.Vault{}, req, noop, nil
	}
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, command)
	if err != nil {
		return propose.Vault{}, req, noop, err
	}
	cfg := vdb.Config
	req.Embed = func(path string) { embedAfterWrite(cmd, path, cfg) }
	v := propose.Vault{Path: vaultPath, DB: vdb.DB, Config: cfg, Reg: vdb.Reg}
	if req.JSON {
		v.IndexHash = vdb.GetIndexHash
	}
	return v, req, vdb.Close, nil
}

type askClosest struct{ cmd *cobra.Command }

func newAskClosest(cmd *cobra.Command) propose.Retriever {
	return askClosest{cmd: cmd}
}

func (a askClosest) Closest(ctx context.Context, vaultPath string, db *index.DB, text string) (propose.Hit, bool, error) {
	ret := query.BuildAutoRetrieverFull(ctx, db)
	// Cleanup leaves the shared ONNX session loaded. The draft is embedded
	// on write, and that pass cannot start a session while this one is held.
	defer query.CloseSharedEmbedder()
	defer ret.Cleanup()
	limit, forward, salient := askRankingBounds(a.cmd)
	noise, sigma, has, low := 0.0, 0.0, false, false
	if ret.Embedder != nil {
		noise, sigma, low = resolveNoiseFloor(ctx, vaultPath, ret.Embedder.Dims())
		has = true
	}
	result, err := query.Rank(ctx, ret.Retriever, db, query.AskConfig{
		Query:              text,
		SearchLimit:        limit,
		Embedder:           ret.Embedder,
		NoiseFloor:         noise,
		NoiseFloorSigma:    sigma,
		HasNoiseFloor:      has,
		VaultLowContrast:   low,
		QueryForwardTokens: forward,
		QuerySalientTerms:  salient,
	})
	if err != nil || result == nil || len(result.TopHits) == 0 {
		return propose.Hit{}, false, err
	}
	hit := result.TopHits[0]
	return propose.Hit{ID: hit.ID, Path: hit.Path, Type: hit.Type, Title: hit.Title, Band: query.RelevanceBand(result)}, true, nil
}
