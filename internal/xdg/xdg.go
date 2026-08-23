// Package xdg provides XDG Base Directory Specification compliant paths.
//
// On Linux/Unix, it follows the XDG spec:
//   - Config: $XDG_CONFIG_HOME or ~/.config
//   - Data:   $XDG_DATA_HOME or ~/.local/share
//   - Cache:  $XDG_CACHE_HOME or ~/.cache
//   - State:  $XDG_STATE_HOME or ~/.local/state
//
// On macOS, it uses Apple conventions for data/cache/state, but ~/.config for
// config — see configBase for why, and NativeConfigDir for the Apple location
// that the explicit --config-path-mode native still resolves to:
//   - Config: $XDG_CONFIG_HOME or ~/.config
//   - Data:   ~/Library/Application Support
//   - Cache:  ~/Library/Caches
//   - State:  ~/Library/Application Support
//
// On Windows, it uses standard Windows paths:
//   - Config: %AppData%
//   - Data:   %AppData%
//   - Cache:  %LocalAppData%
//   - State:  %AppData%
//
// Usage:
//
//	// At startup, set the app name from your single source of truth
//	xdg.SetAppName(binaryName)
//
//	// Then use the package functions
//	configDir, _ := xdg.ConfigDir()
//	cacheFile, _ := xdg.CacheFile("timings.json")
package xdg

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	appName string
	mu      sync.RWMutex

	// osName holds the operating system name. Defaults to runtime.GOOS.
	// Overridden in tests to cover platform-specific branches.
	osName = runtime.GOOS
)

// SetAppName sets the application name used for XDG directories.
// This should be called once at startup from your single source of truth.
// The name should be lowercase and without spaces (e.g., "ckeletin-go").
func SetAppName(name string) {
	mu.Lock()
	defer mu.Unlock()
	appName = name
}

// GetAppName returns the configured application name.
func GetAppName() string {
	mu.RLock()
	defer mu.RUnlock()
	return appName
}

func getAppName() (string, error) {
	mu.RLock()
	defer mu.RUnlock()
	if appName == "" {
		return "", fmt.Errorf("xdg: app name not set, call xdg.SetAppName() first")
	}
	return appName, nil
}

// ConfigDir returns the directory for configuration files.
// Creates the directory if it doesn't exist.
func ConfigDir() (string, error) {
	name, err := getAppName()
	if err != nil {
		return "", err
	}
	base := configBase()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// ConfigFile returns the path to a config file.
func ConfigFile(filename string) (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}

// DataDir returns the directory for persistent data files.
// Creates the directory if it doesn't exist.
func DataDir() (string, error) {
	name, err := getAppName()
	if err != nil {
		return "", err
	}
	base := dataBase()
	dir := filepath.Join(base, name)
	// BEFORE MkdirAll, deliberately. A guard that refuses the path after creating
	// the directory has already done the thing it exists to prevent.
	guardTestIsolation(dir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// guardTestIsolation stops a test binary writing into the developer's real data
// directory. It panics; that is the point.
//
// Issue #121: `go test` wrote note_access events into the real experiments.db.
// 156 of 227 note_get events in the live log were fixture ids — `does-not-exist`,
// `concept-alpha`, `no-such-id` — arriving 52 in a minute, which is `task check`,
// not a person reading notes. Every measurement taken off that log was partly a
// measurement of the test suite.
//
// A guard for this already existed and worked, in internal/experiment. It covered
// one package, so `go test ./cmd/...` went straight past it. Replicating it per
// package is the same forgettable convention that let the shipped hook scripts rot
// away from their working copies. DataDir is the choke point every caller reaches
// experiments.db through, so one condition here covers the packages that exist and
// the ones nobody has written yet.
//
// Why panic rather than return an error: cmd/root.go's write path does
//
//	log.Debug().Err(expErr).Msg("Experiment DB unavailable")
//
// so an error would stop the corruption and tell no one — the highest-traffic
// caller would swallow it at debug level. A guard the caller can silence is not a
// guard. A panic cannot be debug-logged away, and testing.Testing() makes this
// branch unreachable outside a test binary, so it can never fire for a user.
//
// It checks where the path RESOLVES, not whether XDG_DATA_HOME is set: the
// variable can be set and still point at the real directory, and "the env var is
// non-empty" proves nothing about isolation.
func guardTestIsolation(dir string) {
	if !testing.Testing() {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	for _, realBase := range []string{
		filepath.Join(home, "Library", "Application Support"),
		filepath.Join(home, ".local", "share"),
		filepath.Join(home, "AppData", "Roaming"),
	} {
		if !strings.HasPrefix(abs, realBase) {
			continue
		}
		panic(fmt.Sprintf(
			"vaultmind test isolation (issue #121): this test would write to your real data "+
				"directory at %s.\n"+
				"Test runs there are recorded as agent behaviour and corrupt every measurement "+
				"taken from the usage log.\n"+
				"Fix: run `task test` (which sets XDG_DATA_HOME for you), or set it yourself:\n"+
				"    export XDG_DATA_HOME=$(mktemp -d)\n"+
				"In a single test: t.Setenv(\"XDG_DATA_HOME\", t.TempDir())", abs))
	}
}

// DataFile returns the path to a data file.
func DataFile(filename string) (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}

// CacheDir returns the directory for cache files.
// Creates the directory if it doesn't exist.
func CacheDir() (string, error) {
	name, err := getAppName()
	if err != nil {
		return "", err
	}
	base := cacheBase()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// CacheFile returns the path to a cache file.
func CacheFile(filename string) (string, error) {
	dir, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}

// StateDir returns the directory for state files (logs, history, etc.).
// Creates the directory if it doesn't exist.
func StateDir() (string, error) {
	name, err := getAppName()
	if err != nil {
		return "", err
	}
	base := stateBase()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// StateFile returns the path to a state file.
func StateFile(filename string) (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}

// NativeConfigDir returns the OS-native config directory — on darwin
// ~/Library/Application Support/<app>, elsewhere the same as ConfigDir.
//
// This exists ONLY for the explicit `--config-path-mode native|both` option.
// ConfigDir is the default and follows ~/.config on darwin (see configBase for
// why). Collapsing the two would have silently removed a mode a user can ask
// for by name, so the mode keeps its own resolver rather than the default
// keeping the wrong path.
func NativeConfigDir() (string, error) {
	name, err := getAppName()
	if err != nil {
		return "", err
	}
	base := nativeConfigBase()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// nativeConfigBase is the platform's own convention, unmodified. Only
// NativeConfigDir uses it.
func nativeConfigBase() string {
	if osName == "darwin" {
		return filepath.Join(homeDir(), "Library", "Application Support")
	}
	return configBase()
}

// configBase returns the base config directory.
//
// macOS uses ~/.config, NOT ~/Library/Application Support. That is deliberate
// and it is a correction, not a preference:
//
//   - The codebase already disagreed with itself. cmd/root.go resolves the user
//     config dir with its own resolveXDGConfigDir(), returning ~/.config/<app>
//     on every platform — and that is the DEFAULT search mode. So config was
//     read from ~/.config while this helper pointed elsewhere, and any caller
//     reaching for it got a directory the rest of the tool does not use.
//   - It broke a real check, silently. cmd/doctor_mesh.go asked ConfigFile()
//     for the wake-watcher heartbeat while every watcher script writes
//     ${XDG_CONFIG_HOME:-$HOME/.config}/vaultmind. Verified 2026-08-23: zero
//     heartbeat files in the Library path, two in ~/.config. On darwin the
//     check could never see the file, so a watcher seven days dead reported
//     "not found (wake-on-idle not confirmed)" — a verdict about the watcher
//     that actually meant the checker had looked somewhere nothing writes.
//   - A peer tool settles it. chat-mcp writes agents.yaml to
//     ~/.config/vaultmind on macOS. Agreeing with what is already on disk beats
//     agreeing with a platform convention meant for application data.
//
// Note the consequence for cmd/root.go's ConfigPathInfo: XDGDir and NativeDir
// now resolve identically on darwin, so --config-path-mode native/both are
// degenerate there. That is honest — there was only ever one config directory
// in use — but it makes the flag's macOS behaviour worth revisiting separately.
//
// Windows keeps %AppData%: there is no ~/.config idiom there, so none of the
// reasoning above carries. Data, state and cache are unaffected — they have
// their own base functions and keep their platform-native locations.
func configBase() string {
	switch osName {
	case "windows":
		if dir := os.Getenv("AppData"); dir != "" {
			return dir
		}
		return filepath.Join(homeDir(), "AppData", "Roaming")
	default: // Linux, macOS, and other Unix
		if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
			return dir
		}
		return filepath.Join(homeDir(), ".config")
	}
}

// dataBase returns the base data directory. XDG_DATA_HOME is honored on all
// platforms when explicitly set — the XDG spec is cross-platform by design,
// and an explicit env var signals deliberate user intent that should win over
// OS defaults. When unset, the platform default applies (Library/Application
// Support on macOS, %AppData% on Windows, ~/.local/share on Linux/Unix).
// This cross-platform override is also what makes test isolation possible
// on macOS — see issue #17 and Taskfile.yml 'test'/'check' tasks.
func dataBase() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return dir
	}
	switch osName {
	case "darwin":
		return filepath.Join(homeDir(), "Library", "Application Support")
	case "windows":
		if dir := os.Getenv("AppData"); dir != "" {
			return dir
		}
		return filepath.Join(homeDir(), "AppData", "Roaming")
	default: // Linux and other Unix
		return filepath.Join(homeDir(), ".local", "share")
	}
}

// cacheBase returns the base cache directory.
func cacheBase() string {
	switch osName {
	case "darwin":
		return filepath.Join(homeDir(), "Library", "Caches")
	case "windows":
		if dir := os.Getenv("LocalAppData"); dir != "" {
			return dir
		}
		return filepath.Join(homeDir(), "AppData", "Local")
	default: // Linux and other Unix
		if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
			return dir
		}
		return filepath.Join(homeDir(), ".cache")
	}
}

// stateBase returns the base state directory.
// State is for data that should persist between restarts but isn't config (logs, history).
func stateBase() string {
	switch osName {
	case "darwin":
		// macOS doesn't have a state concept, use Application Support
		return filepath.Join(homeDir(), "Library", "Application Support")
	case "windows":
		if dir := os.Getenv("AppData"); dir != "" {
			return dir
		}
		return filepath.Join(homeDir(), "AppData", "Roaming")
	default: // Linux and other Unix
		if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
			return dir
		}
		return filepath.Join(homeDir(), ".local", "state")
	}
}

// homeDir returns the user's home directory.
func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}
