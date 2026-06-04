package cmd

import (
	"errors"
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/query"
	"github.com/spf13/cobra"
)

var noteGetCmd = MustNewCommand(commands.NoteGetMetadata, runNoteGet)

func init() {
	noteCmd.AddCommand(noteGetCmd)
	setupCommandConfig(noteGetCmd)
}

func runNoteGet(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vaultmind note get <id-or-path>")
	}
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppNoteVault)
	vdb, err := cmdutil.OpenVaultDBOrWriteErr(cmd, vaultPath, "note get")
	if err != nil {
		if errors.Is(err, cmdutil.ErrAlreadyWritten) {
			return nil
		}
		return err
	}
	defer vdb.Close()

	// Log note access for experiment outcome linkage (non-blocking)
	if session := experiment.FromContext(cmd.Context()); session != nil {
		session.SetVaultPath(vaultPath)
		_, _ = session.LogNoteAccessEvent(args[0], "note_get")
	}

	return query.RunNoteGet(vdb.DB, query.NoteGetConfig{
		Input:           args[0],
		FrontmatterOnly: getConfigValueWithFlags[bool](cmd, "frontmatter-only", config.KeyAppNoteFrontmatterOnly),
		JSONOutput:      getConfigValueWithFlags[bool](cmd, "json", config.KeyAppNoteJson),
		VaultPath:       vaultPath,
	}, cmd.OutOrStdout())
}
