package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"

	"github.com/peiman/vaultmind/.ckeletin/pkg/config"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vaultmind patches two framework defaults in .ckeletin/. `task ckeletin:update`
// replaces .ckeletin/ with upstream and resets both, so these tests live in
// project code, where an update cannot remove them, and fail when it happens.
// After an update, re-apply the patch the failing test names.

// .ckeletin/pkg/config/core_options.go: app.log_level defaults to "warn", so a
// normal command prints its result, not info-level chatter on stderr.
func TestFrameworkPatch_LogLevelDefaultsToWarn(t *testing.T) {
	var def any
	for _, opt := range config.Registry() {
		if opt.Key == "app.log_level" {
			def = opt.DefaultValue
		}
	}
	assert.Equal(t, "warn", def, "re-apply the app.log_level default patch in .ckeletin/pkg/config/core_options.go")
}

// .ckeletin/pkg/config/security.go: a 0644 config file (a normal install) is
// noted at Debug, not Info. Config loads at Info level, so an Info note
// printed on every command (agents saw it on stderr in v0.10.10).
func TestFrameworkPatch_PermissiveConfigNoteIsDebug(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not checked on Windows")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("app: {}\n"), 0o644))
	require.NoError(t, os.Chmod(path, 0o644))

	var buf bytes.Buffer
	saved := log.Logger
	log.Logger = zerolog.New(&buf).Level(zerolog.InfoLevel)
	t.Cleanup(func() { log.Logger = saved })

	require.NoError(t, config.ValidateConfigFilePermissions(path))
	assert.NotContains(t, buf.String(), "permissive permissions",
		"re-apply the Debug-level permissive-permissions patch in .ckeletin/pkg/config/security.go")
}

// .ckeletin/Taskfile.yml: golangci-lint is pinned at v2.14.0 or later. Go
// 1.27.2 (.go-version) writes V5 export data; v2.12/v2.13 cannot read it and
// CI lint fails on `import "math"` (it did, on the 0.2.2 update's first push).
func TestFrameworkPatch_GolangciLintReadsThisGoVersion(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", ".ckeletin", "Taskfile.yml"))
	require.NoError(t, err)
	m := regexp.MustCompile(`(?m)^\s*TOOL_GOLANGCI_LINT_VERSION:\s*v(\d+)\.(\d+)\.(\d+)`).FindStringSubmatch(string(raw))
	require.NotNil(t, m, "TOOL_GOLANGCI_LINT_VERSION not found in .ckeletin/Taskfile.yml")
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	assert.True(t, major > 2 || (major == 2 && minor >= 14),
		"golangci-lint v%s.%s.%s cannot read Go 1.27.2's export data; re-apply the v2.14.0 pin in .ckeletin/Taskfile.yml", m[1], m[2], m[3])
}
