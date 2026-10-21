package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/peiman/vaultmind/internal/projectdefaults"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// app.log_level defaults to "warn", so a normal command prints its result,
// not info-level lines on stderr. It is a project default
// (internal/projectdefaults), outside .ckeletin/, so framework updates keep it.
func TestProjectDefaults_LogLevelIsWarn(t *testing.T) {
	config.SetDefaults()
	require.NoError(t, config.ApplyProjectDefaultsTOML(projectdefaults.TOML))
	assert.Equal(t, "warn", viper.GetString("app.log_level"),
		"internal/projectdefaults/ckeletin-project.toml must set app.log_level = \"warn\"")

	reg, err := config.EffectiveRegistry(projectdefaults.TOML)
	require.NoError(t, err)
	var published any
	for _, opt := range reg {
		if opt.Key == "app.log_level" {
			published = opt.DefaultValue
		}
	}
	assert.Equal(t, "warn", published, "generated docs must publish the default the process uses")
}

// The golangci-lint the framework installs must read this module's Go
// version: Go 1.27.x (.go-version) writes V5 export data, read from v2.14.0
// on. A project override (CKELETIN_TOOL_GOLANGCI_LINT_VERSION) would win; none
// is set, so the framework pin is the one CI installs.
func TestToolPins_GolangciLintReadsThisGoVersion(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", ".ckeletin", "Taskfile.yml"))
	require.NoError(t, err)
	m := regexp.MustCompile(`TOOL_GOLANGCI_LINT_VERSION:[^\n]*?v(\d+)\.(\d+)\.(\d+)`).FindStringSubmatch(string(raw))
	require.NotNil(t, m, "TOOL_GOLANGCI_LINT_VERSION not found in .ckeletin/Taskfile.yml")
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	assert.True(t, major > 2 || (major == 2 && minor >= 14),
		"golangci-lint v%s.%s.%s cannot read Go 1.27's export data; set CKELETIN_TOOL_GOLANGCI_LINT_VERSION in Taskfile.yml", m[1], m[2], m[3])
}
