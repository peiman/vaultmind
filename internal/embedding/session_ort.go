//go:build cgo && ORT

package embedding

import (
	"os"
	"runtime"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/options"
)

// newBGEM3Session creates an ORT-accelerated hugot session for BGE-M3.
// This file is compiled when building with -tags ORT.
// Requires libonnxruntime installed on the system.
func newBGEM3Session() (*hugot.Session, error) {
	var opts []options.WithOption
	if libDir := detectORTLibDir(); libDir != "" {
		opts = append(opts, options.WithOnnxLibraryPath(libDir))
	}
	return hugot.NewORTSession(opts...)
}

// detectORTLibDir finds the ONNX Runtime library directory.
func detectORTLibDir() string {
	// Environment variable takes priority
	if dir := os.Getenv("ORT_LIB_DIR"); dir != "" {
		return dir
	}
	// Common locations by platform
	candidates := []string{"/usr/local/lib", "/usr/lib"}
	if runtime.GOOS == "darwin" {
		candidates = append([]string{"/opt/homebrew/lib"}, candidates...)
	}
	for _, dir := range candidates {
		lib := "libonnxruntime.so"
		if runtime.GOOS == "darwin" {
			lib = "libonnxruntime.dylib"
		}
		if _, err := os.Stat(dir + "/" + lib); err == nil {
			return dir
		}
	}
	return ""
}
