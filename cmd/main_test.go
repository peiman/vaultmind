package cmd

import (
	"fmt"
	"os"
	"testing"
)

// TestMain points the whole package at an EMPTY config directory.
//
// Without it the suite read the developer's own ~/.config/vaultmind/config.yaml:
// the first test to initialise config loaded it into the global viper, and
// every later test saw it. Outcomes depended on whose machine ran them — a
// root pin set on mine turned six doctor tests red (2026-09-23), and they
// would have passed on CI, which has no such file. Tests that need a config
// value set it themselves.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "vm-cmd-test-config-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "cmd tests: temp config dir:", err)
		os.Exit(1)
	}
	if err := os.Setenv("XDG_CONFIG_HOME", dir); err != nil {
		fmt.Fprintln(os.Stderr, "cmd tests: set XDG_CONFIG_HOME:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
