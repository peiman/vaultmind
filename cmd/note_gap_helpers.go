package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/gap"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/spf13/cobra"
)

// This reference is the namespace declaration: this file owns note gap.
// Ranking bounds stay in ask_helpers.go, so this file does not read app.ask.
var _ = commands.NoteGapMetadata

func runGap(cmd *cobra.Command, args []string) error {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("a question is required")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppNotegapVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppNotegapJson)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "note gap")
	if err != nil {
		return err
	}
	defer vdb.Close()
	v := gap.Vault{Path: vaultPath, DB: vdb.DB, Config: vdb.Config, Reg: vdb.Reg}
	if jsonOut {
		v.IndexHash = vdb.GetIndexHash
	}
	req := gap.Request{
		Question: args[0],
		Session:  os.Getenv(experiment.EnvUserSessionID),
		JSON:     jsonOut,
		Stdout:   cmd.OutOrStdout(),
		Stderr:   cmd.ErrOrStderr(),
	}
	return gap.Open(cmd.Context(), v, req, gapRankHits{cmd: cmd, db: vdb.DB, vault: vaultPath})
}

// gapRankHits is the top hit ids ask would return for the question now.
// Drafts and gaps are excluded by that ranking. The library does not rank.
type gapRankHits struct {
	cmd   *cobra.Command
	db    *index.DB
	vault string
}

func (g gapRankHits) IDs(ctx context.Context, question string) ([]string, error) {
	result, err := askRank(g.cmd, ctx, g.vault, g.db, question)
	if err != nil || result == nil {
		return nil, err
	}
	ids := make([]string, 0, len(result.TopHits))
	for _, hit := range result.TopHits {
		ids = append(ids, hit.ID)
	}
	return ids, nil
}
