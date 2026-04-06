package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var noteMgetCmd = &cobra.Command{
	Use:   "mget",
	Short: "Batch read multiple notes by ID",
	Long: `Batch read multiple notes in a single call. Returns found notes and lists not-found IDs separately.

Provide IDs via --ids (comma-separated) or --stdin (one ID per line).
Use --include-body to include full note body text in the response.

With --json, returns: {"result": {"notes": [...], "not_found": ["id1", ...]}}`,
	RunE: runNoteMget,
}

func init() {
	noteCmd.AddCommand(noteMgetCmd)
	noteMgetCmd.Flags().String("vault", ".", "Path to vault root")
	noteMgetCmd.Flags().Bool("json", false, "Output in JSON format")
	noteMgetCmd.Flags().String("ids", "", "Comma-separated note IDs")
	noteMgetCmd.Flags().Bool("stdin", false, "Read IDs from stdin")
	noteMgetCmd.Flags().Bool("include-body", false, "Include body text")
}

func runNoteMget(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppNoteVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "note mget")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	ids, err := collectMgetIDs(cmd)
	if err != nil {
		return err
	}
	includeBody, _ := cmd.Flags().GetBool("include-body")
	result, err := query.Mget(vdb.DB, ids, !includeBody)
	if err != nil {
		return fmt.Errorf("batch read: %w", err)
	}

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		env := envelope.OK("note mget", result)
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexHash = vdb.GetIndexHash()
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
	if idsFlag != "" {
		var ids []string
		for _, p := range strings.Split(idsFlag, ",") {
			if p = strings.TrimSpace(p); p != "" {
				ids = append(ids, p)
			}
		}
		return ids, nil
	}
	useStdin, _ := cmd.Flags().GetBool("stdin")
	if useStdin {
		var ids []string
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if line := strings.TrimSpace(scanner.Text()); line != "" {
				ids = append(ids, line)
			}
		}
		return ids, scanner.Err()
	}
	return nil, fmt.Errorf("provide --ids or --stdin")
}
