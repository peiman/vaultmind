package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/memory"
	"github.com/spf13/cobra"
)

var memorySummarizeCmd = MustNewCommand(commands.MemorySummarizeMetadata, runMemorySummarize)

func init() {
	memoryCmd.AddCommand(memorySummarizeCmd)
	setupCommandConfig(memorySummarizeCmd)
}

func runMemorySummarize(cmd *cobra.Command, args []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppMemorysummarizeVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "memory summarize")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()
	ids := collectSummarizeIDs(cmd, args)
	if len(ids) == 0 {
		return fmt.Errorf("provide note IDs as arguments or via --ids")
	}
	result, err := memory.Summarize(vdb.DB, memory.SummarizeConfig{
		NoteIDs:     ids,
		IncludeBody: getConfigValueWithFlags[bool](cmd, "include-body", config.KeyAppMemorysummarizeIncludeBody),
		MaxBodyLen:  getConfigValueWithFlags[int](cmd, "max-body-len", config.KeyAppMemorysummarizeMaxBodyLen),
	})
	if err != nil {
		return fmt.Errorf("summarize: %w", err)
	}
	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppMemorysummarizeJson) {
		env := envelope.OK("memory summarize", result)
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexHash = vdb.GetIndexHash()
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}
	return formatSummarize(result, cmd.OutOrStdout())
}

func collectSummarizeIDs(cmd *cobra.Command, args []string) []string {
	if len(args) > 0 {
		return args
	}
	idsFlag := getConfigValueWithFlags[string](cmd, "ids", config.KeyAppMemorysummarizeIds)
	if idsFlag == "" {
		return nil
	}
	var ids []string
	for _, p := range strings.Split(idsFlag, ",") {
		if p = strings.TrimSpace(p); p != "" {
			ids = append(ids, p)
		}
	}
	return ids
}

func formatSummarize(result *memory.SummarizeResult, w io.Writer) error {
	for _, src := range result.Sources {
		if _, err := fmt.Fprintf(w, "%s  [%s]  %s\n", src.ID, src.Type, src.Title); err != nil {
			return err
		}
	}
	for _, nf := range result.NotFound {
		if _, err := fmt.Fprintf(w, "NOT FOUND: %s\n", nf); err != nil {
			return err
		}
	}
	return nil
}
