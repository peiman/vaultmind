package hookscripts

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// InstallStub puts a fake command called name in dir: a symlink to the one
// committed stub, which sources body from dir/name.body.
//
// A test must never write an executable of its own. macOS scans every newly
// written executable before its first run, and on a loaded machine that took
// up to four minutes per test (#191) — nine such tests were most of the
// package's seven minutes and crossed go test's 10-minute limit in a full run.
// The committed stub is scanned once; a symlink to it and a sourced body are
// not scanned at all. body is written as a shell script (a leading #! line is
// fine — it is a comment when sourced); "$@" is the command's arguments.
//
// Exported from a _test.go file so the external hookscripts_test files can
// use it; it is not part of the package's API.
func InstallStub(t *testing.T, dir, name, body string) {
	t.Helper()
	stub, err := filepath.Abs(filepath.Join("testdata", "stub", "stub"))
	require.NoError(t, err)
	link := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(link+".body", []byte(body), 0o600))
	require.NoError(t, os.Symlink(stub, link))
}

// The stub passes its arguments to the body and exits with the body's code,
// whether it is found on PATH or run by its path.
func TestInstallStub_BehavesAsTheBodySays(t *testing.T) {
	dir := t.TempDir()
	InstallStub(t, dir, "vaultmind", "#!/bin/bash\necho \"got $1 $2\"\nexit 3\n")

	for name, cmd := range map[string]*exec.Cmd{
		// Found on PATH by a shell, as the hooks run it. exec.Command("vaultmind")
		// would resolve against THIS process's PATH instead — a real binary on a
		// developer machine, nothing on CI.
		"on PATH": exec.Command("/bin/bash", "-c", `vaultmind "$@"`, "bash", "ask", "q"),
		"by path": exec.Command(filepath.Join(dir, "vaultmind"), "ask", "q"),
	} {
		cmd.Env = []string{"PATH=" + dir + ":/usr/bin:/bin"}
		out, err := cmd.Output()
		assert.Equal(t, "got ask q\n", string(out), name)
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr, name)
		assert.Equal(t, 3, exitErr.ExitCode(), name)
	}
}

// Two stubs of the same name in different dirs keep their own behaviour.
func TestInstallStub_EachDirHasItsOwnBody(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	InstallStub(t, a, "vaultmind", "echo a\n")
	InstallStub(t, b, "vaultmind", "echo b\n")

	outA, err := exec.Command(filepath.Join(a, "vaultmind")).Output()
	require.NoError(t, err)
	outB, err := exec.Command(filepath.Join(b, "vaultmind")).Output()
	require.NoError(t, err)
	assert.Equal(t, "a\n", string(outA))
	assert.Equal(t, "b\n", string(outB))
}

// The rule InstallStub exists for, as a check rather than a comment: no test
// in this package writes an executable file. Each one cost a macOS scan of up
// to four minutes before its first run (#191); a hook script a test copies
// is run as `bash script` and needs no execute bit either.
func TestNoTestWritesAnExecutable(t *testing.T) {
	files, err := filepath.Glob("*_test.go")
	require.NoError(t, err)
	require.NotEmpty(t, files, "no test files found — the check would pass vacuously")
	fset := token.NewFileSet()
	inspected := 0
	for _, f := range files {
		parsed, err := parser.ParseFile(fset, f, nil, 0)
		require.NoError(t, err)
		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "os" || (sel.Sel.Name != "WriteFile" && sel.Sel.Name != "Chmod") {
				return true
			}
			lit, ok := call.Args[len(call.Args)-1].(*ast.BasicLit)
			if !ok || lit.Kind != token.INT {
				return true
			}
			inspected++
			mode, err := strconv.ParseInt(lit.Value, 0, 64)
			require.NoError(t, err)
			assert.Zerof(t, mode&0o111, "%s: os.%s with mode %s makes an executable — use InstallStub (#191)",
				fset.Position(call.Pos()), sel.Sel.Name, lit.Value)
			return true
		})
	}
	require.Positive(t, inspected, "found no os.WriteFile or os.Chmod calls to check — is the parse wrong?")
}
