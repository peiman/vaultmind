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
		if path == filepath.Join(root, "internal", "testgit", "git.go") {
			// This is the narrowly-scoped implementation exception: testgit owns
			// the raw exec calls that construct its hermetic commands.
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
	execName := guardImports(file)
	lines, handlesGitEnv := scanGuardCalls(fset, file, execName)
	if handlesGitEnv {
		return nil
	}
	return filterShellExemptions(fset, file, lines)
}

func guardImports(file *ast.File) string {
	execName := ""
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if path == "os/exec" {
			execName = "exec"
			if imp.Name != nil {
				execName = imp.Name.Name
			}
		}
	}
	return execName
}

func scanGuardCalls(fset *token.FileSet, file *ast.File, execName string) ([]int, bool) {
	var lines []int
	handlesGitEnv := false
	ast.Inspect(file, func(n ast.Node) bool {
		if hasGitEnvLiteral(n) {
			handlesGitEnv = true
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if line, flagged := guardCallLine(fset, call, execName); flagged {
			lines = append(lines, line)
		}
		return true
	})
	return lines, handlesGitEnv
}

func hasGitEnvLiteral(node ast.Node) bool {
	lit, ok := node.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	value, _ := strconv.Unquote(lit.Value)
	return strings.HasPrefix(value, "GIT_")
}

func guardCallLine(fset *token.FileSet, call *ast.CallExpr, execName string) (int, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Command" && sel.Sel.Name != "CommandContext" {
		return 0, false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != execName {
		return 0, false
	}
	arg := 0
	if sel.Sel.Name == "CommandContext" {
		arg = 1
	}
	if len(call.Args) <= arg {
		return 0, false
	}
	value, literal := stringLiteral(call.Args[arg])
	return fset.Position(call.Pos()).Line, literal && value == "git" || isShellCall(call)
}

func filterShellExemptions(fset *token.FileSet, file *ast.File, lines []int) []int {
	exempt := shellEnvExemptions(fset, file)
	filtered := make([]int, 0, len(lines))
	for _, line := range lines {
		if !exempt[line] {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

func shellEnvExemptions(fset *token.FileSet, file *ast.File) map[int]bool {
	exempt := map[int]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if ok {
			collectShellEnvExemptions(fset, fn.Body, exempt)
			return true
		}
		lit, ok := n.(*ast.FuncLit)
		if ok {
			collectShellEnvExemptions(fset, lit.Body, exempt)
			return false
		}
		return true
	})
	return exempt
}

func collectShellEnvExemptions(fset *token.FileSet, body *ast.BlockStmt, exempt map[int]bool) {
	if body == nil {
		return
	}
	var names map[string][]*ast.CallExpr
	names = shellAssignments(body)
	ast.Inspect(body, func(n ast.Node) bool {
		if _, nested := n.(*ast.FuncLit); nested {
			return false
		}
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		lhs, ok := assign.Lhs[0].(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, idOK := lhs.X.(*ast.Ident)
		if !idOK || lhs.Sel.Name != "Env" || names[id.Name] == nil {
			return true
		}
		if !containsOsEnviron(assign.Rhs[0]) {
			for _, shellCall := range names[id.Name] {
				exempt[fset.Position(shellCall.Pos()).Line] = true
			}
		}
		return true
	})
}

func shellAssignments(body *ast.BlockStmt) map[string][]*ast.CallExpr {
	names := map[string][]*ast.CallExpr{}
	ast.Inspect(body, func(n ast.Node) bool {
		if _, nested := n.(*ast.FuncLit); nested {
			return false
		}
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		id, ok := assign.Lhs[0].(*ast.Ident)
		call, callOK := assign.Rhs[0].(*ast.CallExpr)
		if ok && callOK && isShellCall(call) {
			names[id.Name] = append(names[id.Name], call)
		}
		return true
	})
	return names
}

func containsOsEnviron(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if ok {
			pkg, pkgOK := sel.X.(*ast.Ident)
			found = pkgOK && pkg.Name == "os" && sel.Sel.Name == "Environ"
		}
		return !found
	})
	return found
}

func isShellCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Command" && sel.Sel.Name != "CommandContext") {
		return false
	}
	arg := 0
	if sel.Sel.Name == "CommandContext" {
		arg = 1
	}
	if len(call.Args) <= arg+1 {
		return false
	}
	program, programOK := stringLiteral(call.Args[arg])
	if !programOK || (program != "bash" && program != "sh" && program != "/bin/bash" && program != "/bin/sh") {
		return false
	}
	for _, expr := range call.Args[arg+1:] {
		flag, flagOK := stringLiteral(expr)
		if !flagOK || len(flag) < 2 || flag[0] != '-' || flag[1] == '-' {
			return false
		}
		if flag == "-c" || strings.Contains(flag[1:], "c") {
			return true
		}
	}
	return false
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}
