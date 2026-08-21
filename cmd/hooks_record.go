package cmd

import (
	"fmt"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/cmdutil"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/peiman/vaultmind/internal/hooks"
	"github.com/spf13/cobra"
)

var hooksRecordCmd = func() *cobra.Command {
	c := MustNewCommand(commands.HooksRecordMetadata, runHooksRecord)
	c.Args = cobra.ExactArgs(1)
	return c
}()

func init() {
	hooksCmd.AddCommand(hooksRecordCmd)
	setupCommandConfig(hooksRecordCmd)
}

// runHooksRecord writes one allowlisted hook event.
//
// Failure policy: an unknown event name is a hard error, because it means a hook
// is trying to write evidence under a name nobody reviewed. Everything else —
// telemetry off, no experiment session, a closed DB — is a silent success. A
// hook that fails the operation it was attached to, in order to record that it
// happened, is worse than not recording.
func runHooksRecord(cmd *cobra.Command, args []string) error {
	event := args[0]
	jsonOut := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppHooksrecordJson)
	vaultPath := getConfigValueWithFlags[string](cmd, "vault", config.KeyAppHooksrecordVault)

	if err := hooks.ValidateRecordable(event); err != nil {
		if jsonOut {
			return cmdutil.WriteJSONError(cmd.OutOrStdout(), "hooks record", "unknown_event", err.Error())
		}
		return err
	}

	recorded := false
	if experimentSession != nil && experimentSession.DB != nil {
		if _, err := experimentSession.DB.LogEvent(experiment.Event{
			SessionID: experimentSession.ID,
			Type:      event,
			VaultPath: vaultPath,
			Data:      map[string]any{"fired": true},
		}); err == nil {
			recorded = true
		}
	}

	if jsonOut {
		return cmdutil.WriteJSON(cmd.OutOrStdout(), "hooks record",
			map[string]any{"event": event, "recorded": recorded}, "", "")
	}
	if !recorded {
		// Named, not silent: "recorded" and "the log is switched off" are
		// different facts, and conflating them is the defect this command exists
		// to remove.
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s not recorded (usage log unavailable or telemetry off)\n", event)
		return err
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s recorded\n", event)
	return err
}
