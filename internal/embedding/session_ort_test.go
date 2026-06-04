//go:build cgo && ORT

package embedding

import "testing"

// resolveORTLibDir is the discovery-order heart of detectORTLibDir. It is
// ORT-tagged (runs under `CGO_ENABLED=1 CGO_LDFLAGS=-L./lib go test -tags ORT`).
// The exe-dir branch is what makes a prebuilt-release archive — binary +
// bundled libonnxruntime side by side — run with zero config.
func TestResolveORTLibDir(t *testing.T) {
	const lib = "libonnxruntime.dylib"
	has := func(present ...string) func(string) bool {
		set := map[string]bool{}
		for _, p := range present {
			set[p] = true
		}
		return func(p string) bool { return set[p] }
	}
	sys := []string{"/opt/homebrew/lib", "/usr/local/lib", "/usr/lib"}

	t.Run("ORT_LIB_DIR override wins over everything", func(t *testing.T) {
		got := resolveORTLibDir("/custom/ort", "/app/bin", lib, sys,
			has("/app/bin/"+lib, "/opt/homebrew/lib/"+lib)) // both also present
		if got != "/custom/ort" {
			t.Fatalf("explicit ORT_LIB_DIR must win, got %q", got)
		}
	})

	t.Run("bundled lib next to the executable is found (the release layout)", func(t *testing.T) {
		got := resolveORTLibDir("", "/app/bin", lib, sys, has("/app/bin/"+lib))
		if got != "/app/bin" {
			t.Fatalf("exe-dir bundle must be found, got %q", got)
		}
	})

	t.Run("exe-dir beats a system lib when both exist", func(t *testing.T) {
		got := resolveORTLibDir("", "/app/bin", lib, sys,
			has("/app/bin/"+lib, "/usr/local/lib/"+lib))
		if got != "/app/bin" {
			t.Fatalf("bundled lib must take priority over system, got %q", got)
		}
	})

	t.Run("falls back to system locations in order", func(t *testing.T) {
		got := resolveORTLibDir("", "/app/bin", lib, sys, has("/usr/local/lib/"+lib))
		if got != "/usr/local/lib" {
			t.Fatalf("system fallback failed, got %q", got)
		}
	})

	t.Run("nothing found returns empty", func(t *testing.T) {
		if got := resolveORTLibDir("", "/app/bin", lib, sys, has()); got != "" {
			t.Fatalf("no library anywhere must return empty, got %q", got)
		}
	})
}
