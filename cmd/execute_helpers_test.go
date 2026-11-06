package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A reset must replace slice values, preserve Var bindings, and make the next
// explicit occurrence replace defaults as it would in a fresh process.
func TestResetFlagsRecursive_SlicesStartFresh(t *testing.T) {
	for _, kind := range []string{"slice", "array", "tracked-array"} {
		for _, defaults := range [][]string{{}, {"a,b", `a"b`}} {
			t.Run(kind+"/"+stringSliceDefaultName(defaults), func(t *testing.T) {
				cmd := &cobra.Command{Use: "probe"}
				var values []string
				if kind == "slice" {
					cmd.Flags().StringSliceVar(&values, "items", defaults, "")
				} else {
					cmd.Flags().StringArrayVar(&values, "items", defaults, "")
				}
				if kind == "tracked-array" {
					trackRepeats(cmd, "items")
				}
				require.NoError(t, cmd.ParseFlags([]string{"--items", "before"}))
				for range 1100 {
					resetFlagsRecursive(cmd)
				}
				assert.Equal(t, defaults, values)
				assert.False(t, cmd.Flags().Changed("items"))
				assert.Empty(t, repeatedValues(cmd, "items"))
				if kind == "slice" {
					require.Error(t, cmd.Flags().Set("items", `"unterminated`))
					assert.Equal(t, defaults, values, "a rejected occurrence preserves defaults")
				}
				require.NoError(t, cmd.ParseFlags([]string{"--items", "after", "--items", "again"}))
				assert.Equal(t, []string{"after", "again"}, values)
				if kind == "tracked-array" {
					assert.Equal(t, []string{"after", "again"}, repeatedValues(cmd, "items"))
				}
			})
		}
	}
}

func stringSliceDefaultName(defaults []string) string {
	if len(defaults) == 0 {
		return "empty"
	}
	return "quoted-defaults"
}
