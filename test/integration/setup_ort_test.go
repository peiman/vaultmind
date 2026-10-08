package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestORTTokenizerDetection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash toolchain setup targets macOS and Linux")
	}
	script, err := os.ReadFile("../../.claude/scripts/setup-ort.sh")
	require.NoError(t, err)
	for _, tt := range []struct {
		name, module, version, want string
		fail                        bool
	}{
		{"hugot Go tokenizer", "github.com/gomlx/go-huggingface", "v0.4.13", "github.com/gomlx/go-huggingface v0.4.13 (Go tokenizer; no libtokenizers.a required)", false},
		{"legacy native tokenizer", "github.com/daulet/tokenizers", "v1.27.0", "github.com/daulet/tokenizers v1.27.0", false},
		{"legacy with both dependencies", "github.com/gomlx/go-huggingface v0.4.12\n\tgithub.com/daulet/tokenizers", "v1.27.0", "github.com/daulet/tokenizers v1.27.0", false},
		{"unknown tokenizer", "example.com/unknown", "v1.0.0", "could not identify tokenizer dependency", true},
		{"unresolved version", "github.com/gomlx/go-huggingface", "", "could not resolve tokenizer version", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			scripts := filepath.Join(root, ".claude", "scripts")
			require.NoError(t, os.MkdirAll(scripts, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(scripts, "setup-ort.sh"), script, 0o644))
			gomod := filepath.Join(root, "hugot.mod")
			require.NoError(t, os.WriteFile(gomod, []byte("module github.com/knights-analytics/hugot\nrequire (\n\t"+tt.module+" v0.4.13\n)\n"), 0o644))
			bin := filepath.Join(root, "bin")
			require.NoError(t, os.Mkdir(bin, 0o755))
			// Inject the Go module lookup; version-only detection must not probe system
			// libraries or download native assets, even on a cold CI runner.
			require.NoError(t, os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/bash\ncase \"$*\" in\n*'{{.GoMod}}'*) printf '%s\\n' \"$TEST_HUGOT_GOMOD\" ;;\n*'{{.Version}}'*) printf '%s\\n' \"$TEST_TOKENIZER_VERSION\" ;;\n*) exit 99 ;;\nesac\n"), 0o755))
			cmd := exec.Command("bash", filepath.Join(scripts, "setup-ort.sh"), "--tokenizers-version")
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "TEST_HUGOT_GOMOD="+gomod, "TEST_TOKENIZER_VERSION="+tt.version)
			out, err := cmd.CombinedOutput()
			if tt.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err, "%s", out)
			}
			require.Contains(t, string(out), tt.want)
			_, err = os.Stat(filepath.Join(root, "lib"))
			require.True(t, os.IsNotExist(err), "version-only detection must not install anything")
		})
	}
}
