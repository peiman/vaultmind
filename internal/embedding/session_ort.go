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
//
// CoreML execution provider is wired but disabled by default — see
// shouldEnableCoreML(). The CoreML EP path is incompatible with BGE-M3 in
// the current hugot v0.7.0 + onnxruntime 1.25 combination: session
// creation fails with "ReadExternalDataForTensor Failed to get file size"
// because CoreML's external-data resolver can't locate the 2.27GB
// model.onnx_data companion file (the canonical ONNX-with-external-data
// layout BGE-M3 ships in). All CoreML options tried — MLComputeUnits =
// ALL/CPUAndGPU/CPUAndNeuralEngine, ModelFormat = NeuralNetwork/MLProgram —
// reproduce the same error. The fix is upstream: either hugot exposing a
// session-init callback that lets us pre-flatten the model, or a
// model-conversion step that merges external data into a single ONNX.
// Tracked as future work; the Go-side wiring stays in place so the fix
// lands cheaply once the upstream gap closes.
//
// To opt in for experimentation: set VAULTMIND_ENABLE_COREML=1.
func newBGEM3Session() (*hugot.Session, error) {
	var opts []options.WithOption
	if libDir := detectORTLibDir(); libDir != "" {
		opts = append(opts, options.WithOnnxLibraryPath(libDir))
	}
	if shouldEnableCoreML() {
		// Kept for future investigation. CPUAndGPU avoids the Neural Engine
		// model-conversion path (which is brittlest); MLProgram handles
		// external data better than NeuralNetwork in principle. Neither
		// rescues BGE-M3 today.
		opts = append(opts, options.WithCoreML(map[string]string{
			"MLComputeUnits": "CPUAndGPU",
			"ModelFormat":    "MLProgram",
		}))
	}
	return hugot.NewORTSession(opts...)
}

// shouldEnableCoreML reports whether the CoreML execution provider should
// be enabled for this binary. Default off pending the hugot/ORT upstream
// fix for BGE-M3 + external-data. Opt in with VAULTMIND_ENABLE_COREML=1
// for experimentation or to retest after upstream changes.
func shouldEnableCoreML() bool {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return false
	}
	return os.Getenv("VAULTMIND_ENABLE_COREML") != ""
}

// BackendName identifies which hugot backend the binary was built against.
// Mirror file in session_go.go returns "go". Callers use this to decide
// whether to warn about BGE-M3 indexing being about to run on the slow path.
// Stays "ort" regardless of CoreML state so the slow-backend guard works as
// a binary check; CoreML status is surfaced separately via Acceleration().
func BackendName() string { return "ort" }

// Acceleration reports the active execution-provider stack for this binary
// at runtime. Useful for the doctor command and index summary so the
// operator can see whether BGE-M3 work is hitting CoreML or staying on
// CPU. Mirror in session_go.go returns "cpu" since the pure-Go backend
// has no GPU path.
func Acceleration() string {
	if shouldEnableCoreML() {
		return "ort+coreml"
	}
	return "ort+cpu"
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
