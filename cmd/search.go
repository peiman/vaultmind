package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
)

var searchCmd = MustNewCommand(commands.SearchMetadata, runSearch)

func init() {
	MustAddToRoot(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind search <query>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppSearchVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppSearchJson)
	limit := getConfigValueWithFlags[int](cmd, "limit", config.KeyAppSearchLimit)
	offset := getConfigValueWithFlags[int](cmd, "offset", config.KeyAppSearchOffset)

	cfg, err := vault.LoadConfig(vaultPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	dbPath := filepath.Join(vaultPath, cfg.Index.DBPath)
	db, err := index.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening index: %w", err)
	}
	defer func() { _ = db.Close() }()

	results, err := index.SearchFTS(db, args[0], limit, offset)
	if err != nil {
		return fmt.Errorf("searching: %w", err)
	}

	if jsonOut {
		type searchResult struct {
			Query  string            `json:"query"`
			Offset int               `json:"offset"`
			Limit  int               `json:"limit"`
			Hits   []index.FTSResult `json:"hits"`
			Total  int               `json:"total"`
		}
		env := envelope.OK("search", searchResult{
			Query: args[0], Offset: offset, Limit: limit,
			Hits: results, Total: len(results),
		})
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	for _, r := range results {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", r.NoteID, r.Title); err != nil {
			return err
		}
	}
	return nil
}
