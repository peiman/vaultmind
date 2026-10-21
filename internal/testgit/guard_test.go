package testgit

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Scan source, including tests excluded by build tags and hidden framework
// directories. Git hooks export GIT_DIR and can redirect a test's git init
// into the developer's repository even when cmd.Dir points at a temp directory.
func TestGitCommandsAreHermetic(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, "go.mod"))
	var found []string
	scanned := 0
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			// A nested module is not part of this module's test suite.
			if path != root {
				if _, statErr := os.Stat(filepath.Join(path, "go.mod")); statErr == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		scanned++
		for _, line := range unguardedGitCalls(fset, file) {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			found = append(found, fmt.Sprintf("%s:%d", rel, line))
		}
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, scanned, "no test files scanned — this guard proved nothing")
	require.Empty(t, found, "%s: use internal/testgit.Command or Run (or explicit GIT_ handling); hooks export GIT_DIR and can redirect git into the real repository", strings.Join(found, ", "))
}

// This is a file-level guard, not data-flow analysis: existing tests may keep
// their explicit GIT_ sanitizers. Only actual calls count, not comments or
// source snippets used as fixtures in other source-scanning tests.
func unguardedGitCalls(fset *token.FileSet, file *ast.File) []int {
	execName := ""
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if path == "github.com/peiman/vaultmind/internal/testgit" {
			return nil
		}
		if path == "os/exec" {
			execName = "exec"
			if imp.Name != nil {
				execName = imp.Name.Name
			}
		}
	}
	var lines []int
	handlesGitEnv := false
	ast.Inspect(file, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			value, _ := strconv.Unquote(lit.Value)
			if strings.HasPrefix(value, "GIT_") {
				handlesGitEnv = true
			}
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != execName {
			return true
		}
		arg := 0
		switch sel.Sel.Name {
		case "Command":
		case "CommandContext":
			arg = 1
		default:
			return true
		}
		if len(call.Args) > arg {
			if lit, ok := call.Args[arg].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if value, _ := strconv.Unquote(lit.Value); value == "git" {
					lines = append(lines, fset.Position(call.Pos()).Line)
				}
			}
		}
		return true
	})
	if handlesGitEnv {
		return nil
	}
	return lines
}
