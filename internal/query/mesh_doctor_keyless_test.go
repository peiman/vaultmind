package query

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestKeylessInvariant_NoPrivateKeyRead is the M3 keyless guard: it parses the
// mesh-doctor source and asserts NO os.ReadFile / os.Open / os.OpenFile /
// ioutil.ReadFile call exists anywhere in the file. Tier-1 custody is STAT-ONLY
// (os.Lstat); the "I hold my binding key" proof is a keyless proof-of-possession
// via the signer. The private key file must NEVER be opened for read. If a
// future edit introduces a key read, this test fails loud.
//
// This is a structural guard, not a behavioural one: it forbids the family of
// read primitives in the file outright. The legitimate file-read needs
// (registry file, agents.yaml) live in the cmd layer and read DIFFERENT paths,
// not the private key — so this package-scoped guard can be absolute.
func TestKeylessInvariant_NoPrivateKeyRead(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "mesh_doctor.go", nil, parser.AllErrors)
	require.NoError(t, err)

	forbidden := map[string]map[string]bool{
		"os":     {"ReadFile": true, "Open": true, "OpenFile": true},
		"ioutil": {"ReadFile": true},
	}

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if methods, ok := forbidden[pkg.Name]; ok && methods[sel.Sel.Name] {
			t.Fatalf("KEYLESS INVARIANT VIOLATED: mesh_doctor.go calls %s.%s — "+
				"the private key file must NEVER be read; tier-1 is stat-only "+
				"(os.Lstat) and the binding-key check is a keyless proof-of-possession",
				pkg.Name, sel.Sel.Name)
		}
		return true
	})

	// Belt-and-suspenders: assert os.Lstat IS present (the stat-only custody
	// check) so a refactor that drops custody checking is also caught. Reading
	// the SOURCE file in a test is unrelated to the keyless invariant — that
	// invariant forbids PRODUCTION code reading the PRIVATE KEY, asserted above.
	raw, err := os.ReadFile("mesh_doctor.go")
	require.NoError(t, err)
	require.Contains(t, string(raw), "os.Lstat", "tier-1 custody must use os.Lstat (stat-only)")
}
