package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/graph"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var askCmd = MustNewCommand(commands.AskMetadata, runAsk)

func init() {
	MustAddToRoot(askCmd)
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

	resolver := graph.NewResolver(vdb.DB)
	result, err := query.Ask(&query.FTSRetriever{DB: vdb.DB}, resolver, vdb.DB, query.AskConfig{
		Query:       args[0],
		Budget:      getConfigValueWithFlags[int](cmd, "budget", config.KeyAppAskBudget),
		MaxItems:    getConfigValueWithFlags[int](cmd, "max-items", config.KeyAppAskMaxItems),
		SearchLimit: getConfigValueWithFlags[int](cmd, "search-limit", config.KeyAppAskSearchLimit),
	})
	if err != nil {
		return fmt.Errorf("ask: %w", err)
	}
	if !getConfigValueWithFlags[bool](cmd, "json", config.KeyAppAskJson) {
		return formatAsk(result, cmd.OutOrStdout())
	}
	env := envelope.OK("ask", result)
	env.Meta.VaultPath = vaultPath
	env.Meta.IndexHash = vdb.GetIndexHash()
	return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
}

func formatAsk(result *query.AskResult, w io.Writer) error {
	if _, err := fmt.Fprintf(w, "Search: %q (%d hits)\n", result.Query, len(result.TopHits)); err != nil {
		return err
	}
	for _, h := range result.TopHits {
		if _, err := fmt.Fprintf(w, "  %.2f  %-40s  %s\n", h.Score, h.ID, h.Title); err != nil {
			return err
		}
	}
	if result.Context == nil {
		return nil
	}
	if _, err := fmt.Fprintf(w, "\nContext from: %s (%d items, %d/%d tokens)\n",
		result.Context.TargetID, len(result.Context.Context),
		result.Context.UsedTokens, result.Context.BudgetTokens); err != nil {
		return err
	}
	if result.Context.Target != nil {
		noteType := ""
		if t, ok := result.Context.Target.Frontmatter["type"].(string); ok {
			noteType = t
		}
		title := ""
		if t, ok := result.Context.Target.Frontmatter["title"].(string); ok {
			title = t
		}
		if _, err := fmt.Fprintf(w, "  [%s] %s\n", noteType, title); err != nil {
			return err
		}
		if result.Context.Target.Body != "" {
			if _, err := fmt.Fprintf(w, "    %s\n", truncate(result.Context.Target.Body, 120)); err != nil {
				return err
			}
		}
	}
	for _, item := range result.Context.Context {
		noteType := ""
		if t, ok := item.Frontmatter["type"].(string); ok {
			noteType = t
		}
		title := ""
		if t, ok := item.Frontmatter["title"].(string); ok {
			title = t
		}
		if _, err := fmt.Fprintf(w, "  [%s] %s\n", noteType, title); err != nil {
			return err
		}
		if item.BodyIncluded && item.Body != "" {
			if _, err := fmt.Fprintf(w, "    %s\n", truncate(item.Body, 120)); err != nil {
				return err
			}
		}
	}
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
