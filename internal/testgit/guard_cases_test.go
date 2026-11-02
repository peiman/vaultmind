package testgit

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnguardedGitCalls(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		count  int
	}{
		{"command", `package p; import "os/exec"; func f() { exec.Command("git", "init") }`, 1},
		{"context", `package p; import "os/exec"; func f() { exec.CommandContext(ctx, "git", "init") }`, 1},
		{"multiline context", "package p\nimport \"os/exec\"\nfunc f() { exec.CommandContext(\nctx,\n\"git\", \"init\") }", 1},
		{"alias", `package p; import process "os/exec"; func f() { process.Command("git", "init") }`, 1},
		{"two calls", `package p; import "os/exec"; func f() { exec.Command("git"); exec.CommandContext(ctx, "git") }`, 2},
		{"explicit sanitizer", `package p; import "os/exec"; func f() { exec.Command("git"); strings.HasPrefix(kv, "GIT_") }`, 0},
		{"explicit keys", `package p; import "os/exec"; func f() { exec.Command("git"); delete(env, "GIT_DIR") }`, 0},
		{"shared helper", `package p; import ("os/exec"; "github.com/peiman/vaultmind/internal/testgit"); func f() { exec.Command("git"); testgit.Run(t, dir, "init") }`, 1},
		{"testgit import plus raw git", `package p; import ("os/exec"; "github.com/peiman/vaultmind/internal/testgit"); func f() { testgit.Run(t, dir, "init"); exec.Command("git", "init") }`, 1},
		{"testgit import plus raw shell", `package p; import ("os/exec"; "github.com/peiman/vaultmind/internal/testgit"); func f() { testgit.Run(t, dir, "init"); exec.Command("bash", "-c", "git init") }`, 1},
		{"comment is not handling", "package p\nimport \"os/exec\"\n// GIT_DIR is dangerous\nfunc f() { exec.Command(\"git\") }", 1},
		{"source snippet", "package p; var fixture = `exec.Command(\"git\", \"init\")`", 0},
		{"other executable", `package p; import "os/exec"; func f() { exec.Command("go"); exec.CommandContext(ctx, "go") }`, 0},
		{"other package", `package p; import "other/exec"; func f() { exec.Command("git") }`, 0},
		{"other method", `package p; import "os/exec"; func f() { exec.LookPath("git") }`, 0},
		{"not a selector", `package p; func f() { Command("git") }`, 0},
		{"no argument", `package p; import "os/exec"; func f() { exec.Command() }`, 0},
		{"context only", `package p; import "os/exec"; func f() { exec.CommandContext(ctx) }`, 0},
		{"issue 380 shape", `package p; import "os/exec"; func f() { cmd := exec.Command("bash", "-c", command); cmd.Dir = sub; cmd.Run() } // from commit 75c8b4b's codex_portable_test.go before the GIT_ env strip fix`, 1},
		{"sh shell", `package p; import "os/exec"; func f() { exec.Command("sh", "-c", command) }`, 1},
		{"bin bash shell", `package p; import "os/exec"; func f() { exec.Command("/bin/bash", "-c", command) }`, 1},
		{"context shell", `package p; import "os/exec"; func f() { exec.CommandContext(ctx, "bash", "-c", command) }`, 1},
		{"shell options separate c", `package p; import "os/exec"; func f() { exec.Command("/bin/bash", "-e", "-c", command) }`, 1},
		{"shell options combined ec", `package p; import "os/exec"; func f() { exec.Command("/bin/bash", "-ec", command) }`, 1},
		{"shell options combined lc", `package p; import "os/exec"; func f() { exec.Command("/bin/bash", "-lc", command) }`, 1},
		{"shell option without c", `package p; import "os/exec"; func f() { exec.Command("/bin/bash", "-e", "script.sh") }`, 0},
		{"script argument", `package p; import "os/exec"; func f() { exec.Command("bash", "script.sh") }`, 0},
		{"full env same function", `package p; import ("os/exec"; "os"); func f() { cmd := exec.Command("bash", "-c", command); cmd.Env = []string{"PATH=" + p}; _ = os.Args }`, 0},
		{"inherited env same function", `package p; import ("os/exec"; "os"); func f() { cmd := exec.Command("bash", "-c", command); cmd.Env = append(os.Environ(), "X=1") }`, 1},
		{"testgit shell helper", `package p; import ("os/exec"; "github.com/peiman/vaultmind/internal/testgit"); func f() { testgit.ShellCommand(t, dir, command); exec.Command("bash", "-c", command) }`, 1},
		{"env in different function", `package p; import "os/exec"; func makeCmd() *exec.Cmd { return exec.Command("bash", "-c", command) }; func f() { cmd := makeCmd(); cmd.Env = []string{"PATH=" + p} }`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "fixture_test.go", tc.source, 0)
			require.NoError(t, err)
			lines := unguardedGitCalls(fset, file)
			require.Len(t, lines, tc.count)
			if tc.count > 0 {
				assert.Positive(t, lines[0], "diagnostics must include the source line")
			}
		})
	}
}
