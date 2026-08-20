package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// notATestFile keeps parser.ParseDir to production sources. Test files would
// otherwise contribute their own key reads and metadata references, and a test
// fixture is not a command.
func notATestFile(fi fs.FileInfo) bool {
	return !strings.HasSuffix(fi.Name(), "_test.go")
}

// A command may only read config keys inside its OWN namespace.
//
// The bug this exists for: `memory pack` read `config.KeyAppMemorycontextpackExcerpt`
// — the excerpt key belonging to a DIFFERENT command. The compiler caught it, but
// only by luck: that constant happened not to exist. Name a constant that *does*
// exist under another command's prefix and it compiles, runs, and silently ignores
// the flag in every config-file and environment-variable path. The flag still works,
// so a manual test passes; only config and env are broken, and nothing says so.
//
// That is a whole class the type system cannot see, because every key constant has
// the same type: string.
//
// The check is also an SSOT guard (Principle 7). `ConfigPrefix` in the command's
// metadata is the single declaration of which namespace a command owns; this asserts
// the command's code agrees with it rather than re-deriving the namespace by hand at
// each call site.
//
// Measured before it was written: 55 of 55 command files resolve to an owner, and
// zero read outside it. The invariant already held; this stops it from quietly
// stopping.
func TestConfigNamespace_CommandsReadOnlyTheirOwnKeys(t *testing.T) {
	keyValues := parseKeyConstants(t, filepath.Join("..", ".ckeletin", "pkg", "config", "keys_generated.go"))
	require.NotEmpty(t, keyValues, "precondition: key constants must be readable")

	prefixes := parseConfigPrefixes(t,
		filepath.Join("..", "internal", "config", "commands"),
		filepath.Join("..", ".ckeletin", "pkg", "config", "commands"),
	)
	require.NotEmpty(t, prefixes, "precondition: command metadata must be readable")

	known := map[string]bool{}
	for _, p := range prefixes {
		known[p] = true
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", notATestFile, 0)
	require.NoError(t, err)

	var checked int
	for _, pkg := range pkgs {
		names := make([]string, 0, len(pkg.Files))
		for name := range pkg.Files {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			file := pkg.Files[name]
			base := filepath.Base(name)

			keys := keysReadBy(file)
			if len(keys) == 0 {
				continue
			}

			owned := ownedPrefixes(file, base, prefixes, known)
			// Not a skip. A file nobody can attribute to a command is exactly
			// where a cross-namespace read would hide, so an unattributable file
			// fails rather than passing quietly.
			require.NotEmpty(t, owned,
				"%s reads config keys but no owning command could be determined.\n"+
					"Reference its commands.XMetadata in this file, or name the file after "+
					"the command it belongs to (ask_read.go -> ask).", base)
			checked++

			for _, key := range keys {
				value, ok := keyValues[key]
				require.True(t, ok,
					"%s reads config.%s, which is not a generated key constant", base, key)
				require.True(t, underAny(value, owned),
					"%s reads %q (config.%s) but owns %v.\n"+
						"A command reading another command's namespace compiles and runs, "+
						"and silently ignores the config file and environment variable.",
					base, value, key, owned)
			}
		}
	}

	// The vacuity guard. Every assertion above lives inside two loops; if the
	// parse produced nothing, or the key-detection stopped matching, the test
	// would pass having checked nothing at all — which is the shape of defect
	// this whole file exists to reject.
	require.GreaterOrEqual(t, checked, 40,
		"expected to check dozens of command files, checked %d — "+
			"the scan found almost nothing, so this run proves nothing", checked)
}

// globalOwners records the files that legitimately own a process-wide namespace
// rather than a command's. Logging and output format are configured once for
// every command, so the root command reads them by design.
//
// Deliberately a closed list: adding a file here is a decision to exempt it, and
// it should be visible in a diff.
var globalOwners = map[string][]string{
	"root.go": {"app.log", "app.log_level", "app.output_format"},
}

// ownedPrefixes resolves which namespaces a command file may read from.
//
// Preference order matters. A metadata reference is a declaration, so it wins.
// The filename fallback exists for helper files that carry part of a command's
// implementation without declaring it (ask_read.go, doctor_mesh.go, note_mget.go)
// — it only ever resolves to a prefix some command actually declared, so it can
// widen coverage but never invent a namespace.
func ownedPrefixes(file *ast.File, base string, prefixes map[string]string, known map[string]bool) []string {
	set := map[string]bool{}

	for _, meta := range metadataReferencedBy(file) {
		if p, ok := prefixes[meta]; ok {
			set[p] = true
		}
	}
	for _, p := range globalOwners[base] {
		set[p] = true
	}

	if len(set) == 0 {
		segments := strings.Split(strings.TrimSuffix(base, ".go"), "_")
		for n := len(segments); n > 0 && len(set) == 0; n-- {
			head := segments[:n]
			for _, candidate := range []string{
				"app." + strings.Join(head, ""),
				"app." + strings.Join(head, "_"),
			} {
				if known[candidate] {
					set[candidate] = true
				}
			}
		}
	}

	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// underAny reports whether value sits inside one of the owned namespaces.
// Compared segment-wise: "app.note" must not match "app.notemget", which a
// plain string prefix would happily accept.
func underAny(value string, owned []string) bool {
	for _, p := range owned {
		if value == p || strings.HasPrefix(value, p+".") {
			return true
		}
	}
	return false
}

// keysReadBy returns the KeyApp* constant names the file reads as config.KeyAppX.
func keysReadBy(file *ast.File) []string {
	set := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "config" {
			return true
		}
		if strings.HasPrefix(sel.Sel.Name, "KeyApp") {
			set[sel.Sel.Name] = true
		}
		return true
	})
	return sortedKeys(set)
}

// metadataReferencedBy returns every *Metadata identifier the file mentions,
// under any package qualifier — cmd/docs.go reaches for both commands.* and
// projcommands.*, and a check that assumed one qualifier would miss the other.
func metadataReferencedBy(file *ast.File) []string {
	set := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if _, ok := sel.X.(*ast.Ident); ok && strings.HasSuffix(sel.Sel.Name, "Metadata") {
			set[sel.Sel.Name] = true
		}
		return true
	})
	return sortedKeys(set)
}

// parseKeyConstants maps KeyApp* constant names to their string values.
func parseKeyConstants(t *testing.T, path string) map[string]string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	require.NoError(t, err, "parsing generated key constants")

	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
				continue
			}
			lit, ok := value.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			unquoted, err := strconv.Unquote(lit.Value)
			if err != nil {
				continue
			}
			out[value.Names[0].Name] = unquoted
		}
	}
	return out
}

// parseConfigPrefixes maps each *Metadata variable to the ConfigPrefix it declares.
func parseConfigPrefixes(t *testing.T, dirs ...string) map[string]string {
	t.Helper()

	out := map[string]string{}
	for _, dir := range dirs {
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, dir, notATestFile, 0)
		require.NoError(t, err, "parsing command metadata in %s", dir)

		for _, pkg := range pkgs {
			for _, file := range pkg.Files {
				for _, decl := range file.Decls {
					gen, ok := decl.(*ast.GenDecl)
					if !ok || gen.Tok != token.VAR {
						continue
					}
					for _, spec := range gen.Specs {
						value, ok := spec.(*ast.ValueSpec)
						if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
							continue
						}
						name := value.Names[0].Name
						if !strings.HasSuffix(name, "Metadata") {
							continue
						}
						if prefix := configPrefixOf(value.Values[0]); prefix != "" {
							out[name] = prefix
						}
					}
				}
			}
		}
	}
	return out
}

// configPrefixOf reads the ConfigPrefix field out of a CommandMetadata literal.
func configPrefixOf(expr ast.Expr) string {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return ""
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "ConfigPrefix" {
			continue
		}
		str, ok := kv.Value.(*ast.BasicLit)
		if !ok || str.Kind != token.STRING {
			continue
		}
		if unquoted, err := strconv.Unquote(str.Value); err == nil {
			return unquoted
		}
	}
	return ""
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
