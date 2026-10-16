package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
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
