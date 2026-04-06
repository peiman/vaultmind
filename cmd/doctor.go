package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/envelope"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var doctorCmd = MustNewCommand(commands.DoctorMetadata, runDoctor)

func init() {
	MustAddToRoot(doctorCmd)
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppDoctorVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "doctor")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	result, err := query.Doctor(vdb.DB, vaultPath)
	if err != nil {
		return fmt.Errorf("running doctor: %w", err)
	}

	if getConfigValueWithFlags[bool](cmd, "json", config.KeyAppDoctorJson) {
		env := envelope.OK("doctor", result)
		env.Meta.VaultPath = vaultPath
		env.Meta.IndexHash = vdb.GetIndexHash()
		return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
	}
	w := cmd.OutOrStdout()
	if _, err = fmt.Fprintf(w,
		"Vault: %s\nNotes: %d (%d domain, %d unstructured)\nUnresolved links: %d\n",
		result.VaultPath, result.TotalFiles, result.DomainNotes,
		result.UnstructuredNotes, result.Issues.UnresolvedLinks); err != nil {
		return err
	}
	if result.Issues.ObsidianIncompatibleLinks > 0 {
		if _, err = fmt.Fprintf(w, "Obsidian-incompatible links: %d\n", result.Issues.ObsidianIncompatibleLinks); err != nil {
			return err
		}
		for _, il := range result.Issues.IncompatibleLinkDetails {
			if _, err = fmt.Fprintf(w, "  %s: [[%s]] → [[%s|%s]]\n",
				il.SourcePath, il.TargetRaw, il.SuggestedFix, il.TargetRaw); err != nil {
				return err
			}
		}
	}
	return nil
}
