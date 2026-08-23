package xdg

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConfigBase_DarwinUsesDotConfig pins the config directory to ~/.config on
// macOS, matching Linux.
//
// It used to return ~/Library/Application Support on darwin — the OS-native
// convention for application *data*. Two things made that wrong here rather than
// merely debatable:
//
//   - This codebase already disagreed with itself. cmd/root.go resolves the
//     user config dir with its own resolveXDGConfigDir(), which returns
//     ~/.config/<app> on every platform, and that is the DEFAULT mode. So the
//     config file was read from ~/.config while xdg.ConfigFile() pointed
//     somewhere else, and any caller reaching for the helper got the path the
//     rest of the tool does not use.
//   - It broke a real check silently. cmd/doctor_mesh.go asked
//     xdg.ConfigFile() for the wake-watcher heartbeat, while every watcher
//     script writes ${XDG_CONFIG_HOME:-$HOME/.config}/vaultmind. Verified
//     2026-08-23: zero heartbeat files in the Library path, two in ~/.config.
//     doctor could not see the file on this platform, ever — so a watcher dead
//     for seven days reported as "not found (wake-on-idle not confirmed)",
//     which reads as a verdict about the watcher and actually meant the check
//     had looked in a directory nothing writes to.
//
// A peer tool settles it: chat-mcp writes agents.yaml to ~/.config/vaultmind on
// macOS. Interop with what is already on disk beats platform convention.
//
// Windows keeps %AppData% — there is no ~/.config idiom there, so the same
// argument does not carry.
func TestConfigBase_DarwinUsesDotConfig(t *testing.T) {
	orig := osName
	t.Cleanup(func() { osName = orig })

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	osName = "darwin"
	require.Equal(t, filepath.Join(home, ".config"), configBase(),
		"darwin must use ~/.config, not Library/Application Support")

	osName = "linux"
	require.Equal(t, filepath.Join(home, ".config"), configBase(),
		"linux is unchanged, and darwin must now agree with it")
}

// TestConfigBase_DarwinHonoursXDGConfigHome — test isolation and deliberate user
// intent both depend on the env var winning, exactly as dataBase already does.
func TestConfigBase_DarwinHonoursXDGConfigHome(t *testing.T) {
	orig := osName
	t.Cleanup(func() { osName = orig })

	explicit := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", explicit)

	osName = "darwin"
	require.Equal(t, explicit, configBase(),
		"an explicit XDG_CONFIG_HOME is deliberate intent and must win on darwin too")
}

// TestConfigBase_WindowsUnchanged guards the half this change must NOT touch.
func TestConfigBase_WindowsUnchanged(t *testing.T) {
	orig := osName
	t.Cleanup(func() { osName = orig })

	t.Setenv("AppData", `C:\Users\test\AppData\Roaming`)
	osName = "windows"
	require.Equal(t, `C:\Users\test\AppData\Roaming`, configBase())
}
