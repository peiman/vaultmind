package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/index"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/peiman/vaultmind/internal/vault"
	"github.com/spf13/cobra"
)

var noteMgetCmd = &cobra.Command{
	Use:   "mget",
	Short: "Batch read multiple notes by ID",
	Long:  "Fetch multiple notes. Pass IDs via --ids (comma-separated) or pipe newline-delimited IDs on stdin.",
	RunE:  runNoteMget,
}

func init() {
	noteCmd.AddCommand(noteMgetCmd)
	noteMgetCmd.Flags().String("vault", ".", "Path to vault root")
	noteMgetCmd.Flags().Bool("json", false, "Output in JSON format")
	noteMgetCmd.Flags().String("ids", "", "Comma-separated note IDs")
	noteMgetCmd.Flags().Bool("stdin", false, "Read IDs from stdin (newline-delimited)")
	noteMgetCmd.Flags().Bool("include-body", false, "Include body text (default: frontmatter-only)")
	setupCommandConfig(noteMgetCmd)
}

func runNoteMget(cmd *cobra.Command, _ []string) error {
	vaultPath, _ := cmd.Flags().GetString("vault")
	jsonOut, _ := cmd.Flags().GetBool("json")

	ids, err := collectMgetIDs(cmd)
	if err != nil {
		return err
	}

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

	includeBody, _ := cmd.Flags().GetBool("include-body")
	result, err := query.Mget(db, ids, !includeBody)
	if err != nil {
		return fmt.Errorf("batch read: %w", err)
	}

	if jsonOut {
		env := envelope.OK("note mget", result)
		env.Meta.VaultPath = vaultPath
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}

	for _, n := range result.Notes {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s\n", n.ID, n.Type, n.Title); err != nil {
			return err
		}
	}
	for _, nf := range result.NotFound {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "NOT FOUND: %s\n", nf); err != nil {
			return err
		}
	}
	return nil
}

func collectMgetIDs(cmd *cobra.Command) ([]string, error) {
	idsFlag, _ := cmd.Flags().GetString("ids")
	useStdin, _ := cmd.Flags().GetBool("stdin")

	if idsFlag != "" {
		parts := strings.Split(idsFlag, ",")
		var ids []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				ids = append(ids, p)
			}
		}
		return ids, nil
	}

	if useStdin {
		var ids []string
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				ids = append(ids, line)
			}
		}
		return ids, scanner.Err()
	}

	return nil, fmt.Errorf("provide --ids or --stdin")
}
