//go:build cgo && ORT

package embedding

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/options"
)

// newBGEM3Session creates an ORT-accelerated hugot session for BGE-M3.
// This file is compiled when building with -tags ORT.
// Requires libonnxruntime installed on the system.
//
// CoreML execution provider is wired but disabled by default — see
// shouldEnableCoreML(). Three independent blockers confirmed during the
// 2026-04-29 investigation prevent the CoreML path from working with
// BGE-M3 in the current hugot v0.7.0 + onnxruntime 1.25 stack:
//
//  1. CoreML EP fails at session creation with
//     "ReadExternalDataForTensor Failed to get file size ... std::filesystem
//     error: Not a directory" because its external-data resolver appears
//     to treat the model file path as a directory base. Reproduced across
//     MLComputeUnits = ALL / CPUAndGPU / CPUAndNeuralEngine and
//     ModelFormat = NeuralNetwork / MLProgram. ORT CPU path resolves the
//     same model fine.
//  2. Rewriting the model proto to use absolute external-data location
//     (the obvious workaround for blocker #1) hits ORT's path-validation
//     guard: "Absolute path not allowed for external data location" — a
//     security boundary, not a bug.
//  3. Flattening the external data into a single self-contained ONNX
//     hits protobuf's 2GB hard serialization limit (BGE-M3 weights are
//     ~2.2GB).
//
// The fix is upstream: hugot/ORT need a path that supports CoreML's
// init expectations for ONNX-with-external-data, OR a CoreML-native
// model conversion step (.mlmodelc), OR a different ML framework
// entirely. None of those are runtime knobs we can flip from Go.
//
// The wiring stays in place so opt-in testing is one env var
// (VAULTMIND_ENABLE_COREML=1) and the diagnostic catalog above stays
// next to the code that would benefit from re-trying CoreML once the
// upstream gap closes. Future-me reading this: try the env var, see
// what error message you get NOW, compare to the catalog above. If it
// matches, the gap is still open. If it's different or works, the
// upstream fix landed and you can flip the default.
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
func BackendName() string { return BackendNameORT }

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

// detectORTLibDir finds the ONNX Runtime library directory, wiring the real OS
// to resolveORTLibDir (which holds the testable discovery order).
func detectORTLibDir() string {
	libName := "libonnxruntime.so"
	if runtime.GOOS == "darwin" {
		libName = "libonnxruntime.dylib"
	}
	systemDirs := []string{"/usr/local/lib", "/usr/lib"}
	if runtime.GOOS == "darwin" {
		systemDirs = append([]string{"/opt/homebrew/lib"}, systemDirs...)
	}
	return resolveORTLibDir(os.Getenv("ORT_LIB_DIR"), executableLibDirs(), libName, systemDirs, func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	})
}

// executableLibDirs returns the candidate directories that may hold the bundled
// libonnxruntime, in priority order: the directory of the invoked executable
// path, then the directory of its symlink-resolved real path. macOS computes a
// process's executable path from the SYMLINK used to launch it — os.Executable
// does not resolve it — so a symlinked-on-PATH install
// (~/.local/bin/vaultmind → ~/.local/vaultmind/vaultmind) would otherwise look
// for the dylib next to the symlink, miss the bundle sitting next to the real
// binary, and silently degrade to MiniLM. Checking the resolved dir too makes a
// downloaded archive portable through a PATH symlink (Siavoush field report,
// 2026-06-19).
func executableLibDirs() []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	dirs := []string{filepath.Dir(exe)}
	if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
		if d := filepath.Dir(resolved); d != dirs[0] {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// resolveORTLibDir picks the directory to load libonnxruntime from, in priority
// order: an explicit ORT_LIB_DIR override, then each executable-relative
// directory (the prebuilt-release bundle layout — a libonnxruntime shipped next
// to the binary, or next to its symlink-resolved real path, is found with zero
// config so a downloaded archive just runs), then standard system locations
// (homebrew / /usr/local). Pure and injectable so the order is unit-testable
// without a real install. Returns "" when none has the library, leaving the
// runtime to surface its own load error.
func resolveORTLibDir(ortLibDirEnv string, exeDirs []string, libName string, systemDirs []string, fileExists func(string) bool) string {
	if ortLibDirEnv != "" {
		return ortLibDirEnv
	}
	for _, dir := range exeDirs {
		if dir != "" && fileExists(filepath.Join(dir, libName)) {
			return dir
		}
	}
	for _, dir := range systemDirs {
		if fileExists(filepath.Join(dir, libName)) {
			return dir
		}
	}
	return ""
}
