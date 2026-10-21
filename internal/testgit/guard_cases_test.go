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
		{"shared helper", `package p; import ("os/exec"; "github.com/peiman/vaultmind/internal/testgit"); func f() { exec.Command("git"); testgit.Run(t, dir, "init") }`, 0},
		{"comment is not handling", "package p\nimport \"os/exec\"\n// GIT_DIR is dangerous\nfunc f() { exec.Command(\"git\") }", 1},
		{"source snippet", "package p; var fixture = `exec.Command(\"git\", \"init\")`", 0},
		{"other executable", `package p; import "os/exec"; func f() { exec.Command("go"); exec.CommandContext(ctx, "go") }`, 0},
		{"other package", `package p; import "other/exec"; func f() { exec.Command("git") }`, 0},
		{"other method", `package p; import "os/exec"; func f() { exec.LookPath("git") }`, 0},
		{"not a selector", `package p; func f() { Command("git") }`, 0},
		{"no argument", `package p; import "os/exec"; func f() { exec.Command() }`, 0},
		{"context only", `package p; import "os/exec"; func f() { exec.CommandContext(ctx) }`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "fixture_test.go", tc.source, 0)
			require.NoError(t, err)
			lines := unguardedGitCalls(fset, file)
			assert.Len(t, lines, tc.count)
			if tc.count > 0 {
				assert.Positive(t, lines[0], "diagnostics must include the source line")
			}
		})
	}
}
