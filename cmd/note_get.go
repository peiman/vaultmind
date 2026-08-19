package cmd

import (
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
		return err
	}
	defer vdb.Close()

	outcome, runErr := query.RunNoteGet(vdb.DB, query.NoteGetConfig{
		Input:           args[0],
		FrontmatterOnly: getConfigValueWithFlags[bool](cmd, "frontmatter-only", config.KeyAppNoteFrontmatterOnly),
		JSONOutput:      getConfigValueWithFlags[bool](cmd, "json", config.KeyAppNoteJson),
		VaultPath:       vaultPath,
	}, cmd.OutOrStdout())

	// Log AFTER the read, keyed on the RESOLVED id and on what was actually
	// rendered. Logging before it ran, with args[0] and a hardcoded true,
	// produced three wrong things at once: a key no lookup can match when the
	// input was a title or path, a delivered read for --frontmatter-only which
	// renders no body, and a delivered read for a note that does not exist.
	// note_get is trusted unconditionally by the activation gate, so each of
	// those entered the ranking. Best-effort — telemetry never fails the read.
	if session := experiment.FromContext(cmd.Context()); session != nil && outcome.NoteID != "" {
		session.SetVaultPath(vaultPath)
		_, _ = session.LogNoteAccessEvent(outcome.NoteID, experiment.AccessSourceRead, outcome.BodyDelivered)
	}
	return runErr
}
