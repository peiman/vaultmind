package cmd

import (
	"context"
	"path/filepath"
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
		DraftsDir: getConfigValueWithFlags[string](cmd, "drafts-dir", config.KeyAppNoteproposeDraftsDir),
		GapID:     getConfigValueWithFlags[string](cmd, "gap", config.KeyAppNoteproposeGap),
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
	req.GapID = strings.TrimSpace(req.GapID)
	// A missing finding, or a missing question with no gap, is the caller's
	// error. Opening a guessed vault first would create an index in the
	// wrong directory. --gap with an empty question still opens the vault:
	// the stored question is on the gap note.
	if command == "note propose" && req.Finding == "" {
		return propose.Vault{}, req, noop, nil
	}
	if command == "note propose" && req.Question == "" && req.GapID == "" {
		return propose.Vault{}, req, noop, nil
	}
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, command)
	if err != nil {
		return propose.Vault{}, req, noop, err
	}
	cfg := vdb.Config
	// One tracker for every embed this command runs. Propose writes the draft
	// and then the rank keys; accept closes a gap and then the target. The
	// second pass still embeds (the hash changed) but does not repeat the line.
	track := &embedPrint{written: map[string]struct{}{}}
	req.Embed = func(path, rel string) {
		track.written[filepath.ToSlash(rel)] = struct{}{}
		embedAfterWriteUpTo(cmd, path, cfg, embedOnWriteLimit, track)
	}
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
	result, err := askRank(a.cmd, ctx, vaultPath, db, text)
	if err != nil || result == nil || len(result.TopHits) == 0 {
		return propose.Hit{}, false, err
	}
	hit := result.TopHits[0]
	return propose.Hit{ID: hit.ID, Path: hit.Path, Type: hit.Type, Title: hit.Title, Band: query.RelevanceBand(result)}, true, nil
}
