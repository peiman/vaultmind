//go:build dev

// cmd/check.go

package cmd

import (
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/check"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
)

var checkCmd = MustNewCommand(commands.CheckMetadata, runCheck)

func init() {
	MustAddToRoot(checkCmd)
}

func runCheck(cmd *cobra.Command, args []string) error {
	// Parse categories from comma-separated string
	categoryStr := getConfigValueWithFlags[string](cmd, "category", config.KeyAppCheckCategory)
	var categories []string
	if categoryStr != "" {
		for _, c := range strings.Split(categoryStr, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				categories = append(categories, c)
			}
		}
	}

	// Validate categories if specified
	if len(categories) > 0 {
		if err := check.ValidateCategories(categories); err != nil {
			return err
		}
	}

	cfg := check.Config{
		FailFast:   getConfigValueWithFlags[bool](cmd, "fail-fast", config.KeyAppCheckFailFast),
		Verbose:    getConfigValueWithFlags[bool](cmd, "verbose", config.KeyAppCheckVerbose),
		Parallel:   getConfigValueWithFlags[bool](cmd, "parallel", config.KeyAppCheckParallel),
		Categories: categories,
		ShowTiming: getConfigValueWithFlags[bool](cmd, "timing", config.KeyAppCheckTiming),
	}

	return check.NewExecutor(cfg, cmd.OutOrStdout()).Execute(cmd.Context())
}
