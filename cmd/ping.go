// cmd/ping.go

package cmd

import (
	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/ping"
	"github.com/peiman/vaultmind/internal/ui"
	"github.com/spf13/cobra"
)

var pingCmd = MustNewCommand(commands.PingMetadata, runPing)

func init() {
	MustAddToRoot(pingCmd)
}

func runPing(cmd *cobra.Command, args []string) error {
	return runPingWithUIRunner(cmd, args, ui.NewDefaultUIRunner())
}

// runPingWithUIRunner is the internal implementation that allows dependency injection for testing
func runPingWithUIRunner(cmd *cobra.Command, args []string, uiRunner ui.UIRunner) error {
	cfg := ping.Config{
		Message: getConfigValueWithFlags[string](cmd, "message", config.KeyAppPingOutputMessage),
		Color:   getConfigValueWithFlags[string](cmd, "color", config.KeyAppPingOutputColor),
		UI:      getConfigValueWithFlags[bool](cmd, "ui", config.KeyAppPingUi),
	}
	return ping.NewExecutor(cfg, uiRunner, cmd.OutOrStdout()).Execute()
}
