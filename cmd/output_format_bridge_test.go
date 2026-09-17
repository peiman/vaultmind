package cmd

import (
	"strings"
	"testing"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/.ckeletin/pkg/output"
	"github.com/peiman/vaultmind/internal/config/commands"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The global --output-format json must actually produce JSON.
//
// It is advertised in every command's help text and was obeyed by almost none
// of them: `doctor --json` returns an envelope, `doctor --output-format json`
// printed plain text and exited 0. A script asking for JSON through the
// documented global flag got prose and no way to detect it — the silent-wrong
// -answer shape, in the interface contract.
//
// The bridge is one place because 37 of the 38 JSON call sites already funnel
// through getConfigValueWithFlags(cmd, "json", ...).
func TestOutputFormatJSON_IsHonouredByTheJSONFlagLookup(t *testing.T) {
	t.Cleanup(func() { output.SetOutputMode("text") })

	cmd := MustNewCommand(commands.DoctorMetadata, func(*cobra.Command, []string) error { return nil })
	require.False(t, cmd.Flags().Changed("json"), "precondition: --json not passed")

	output.SetOutputMode("json")
	got := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppDoctorJson)

	assert.True(t, got,
		"--output-format json must reach commands that read the per-command --json flag")
}

// Text mode must stay text — the bridge must not force JSON on everyone.
func TestOutputFormatText_LeavesTheJSONFlagAlone(t *testing.T) {
	t.Cleanup(func() { output.SetOutputMode("text") })
	output.SetOutputMode("text")

	cmd := MustNewCommand(commands.DoctorMetadata, func(*cobra.Command, []string) error { return nil })
	got := getConfigValueWithFlags[bool](cmd, "json", config.KeyAppDoctorJson)

	assert.False(t, got, "text mode must not silently turn on JSON")
}

// The bridge is scoped to the json flag only — it must not alter other bools.
func TestOutputFormatJSON_DoesNotAffectOtherBoolFlags(t *testing.T) {
	t.Cleanup(func() { output.SetOutputMode("text") })
	output.SetOutputMode("json")

	cmd := MustNewCommand(commands.AskMetadata, func(*cobra.Command, []string) error { return nil })
	got := getConfigValueWithFlags[bool](cmd, "explain", config.KeyAppAskExplain)

	assert.False(t, got, "only the json flag is bridged; --explain must be untouched")
	_ = strings.TrimSpace("")
}
