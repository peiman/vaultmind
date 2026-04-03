package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
)

var resolveCmd = MustNewCommand(commands.ResolveMetadata, runResolve)

func init() {
	MustAddToRoot(resolveCmd)
}

func runResolve(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind resolve <id-or-title-or-alias>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppResolveVault)
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppResolveJson)

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

	resolver := graph.NewResolver(db)
	result, err := resolver.Resolve(args[0])
	if err != nil {
		return fmt.Errorf("resolving: %w", err)
	}

	if jsonOut {
		env := envelope.OK("resolve", result)
		if result.Ambiguous {
			env.Status = "error"
			env.Errors = append(env.Errors, envelope.Issue{
				Code: "ambiguous_resolution", Message: "multiple matches",
			})
		}
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	if !result.Resolved {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "No match for %q\n", args[0])
		return err
	}
	for _, m := range result.Matches {
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s  (%s)\n", m.ID, m.Type, m.Title, m.Path)
		if err != nil {
			return err
		}
	}
	return nil
}
