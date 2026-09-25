package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/peiman/vaultmind/internal/mutation"
	"github.com/spf13/cobra"
)

// jsonArrayOpening and jsonStringOpening are how a JSON array or string value
// begins on the command line.
const (
	jsonArrayOpening  = "["
	jsonStringOpening = `"`
)

var frontmatterSetCmd = MustNewCommand(commands.FrontmatterSetMetadata, runFrontmatterSet)

func init() {
	frontmatterCmd.AddCommand(frontmatterSetCmd)
	setupCommandConfig(frontmatterSetCmd)
}

func runFrontmatterSet(cmd *cobra.Command, args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: frontmatter set <target> <key> <value>")
	}
	return runMutation(cmd, mutation.MutationRequest{
		Op: mutation.OpSet, Target: args[0], Key: args[1], Value: setValue(args[2]),
	}, "frontmatter set", config.KeyAppFrontmattersetVault, config.KeyAppFrontmattersetJson,
		config.KeyAppFrontmattersetDryRun, config.KeyAppFrontmattersetDiff,
		config.KeyAppFrontmattersetCommit, config.KeyAppFrontmattersetAllowExtra)
}

// setValue reads the command-line value: a JSON array becomes a list, as the
// help promises, and a JSON string is the text inside it — the way to store
// text that would otherwise parse as an array. Anything else stays the text it
// is. The array used to be written as a quoted string, turning a note's tags
// into one tag (#159).
func setValue(raw string) interface{} {
	trimmed := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(trimmed, jsonArrayOpening):
		var list []interface{}
		if json.Unmarshal([]byte(trimmed), &list) == nil {
			return list
		}
	case strings.HasPrefix(trimmed, jsonStringOpening):
		var text string
		if json.Unmarshal([]byte(trimmed), &text) == nil {
			return text
		}
	}
	return raw
}
