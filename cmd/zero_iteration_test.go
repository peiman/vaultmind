package cmd

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A test whose every assertion sits inside a range loop passes when that loop
// runs zero times. It reports green having checked nothing.
//
// Not hypothetical. TestAsk_ItemsWithoutTextAreNotDeliveries was written and
// merged in exactly that shape — `require.NoError` and `require.NotNil` outside
// the loop, and the only assertion that mattered inside it. When the pack
// delivered no items the loop did not run and the test passed. Checked against
// the pre-fix commit (da0e6a2): this rule flags it.
//
// The rule is narrow, and every narrowing was measured rather than guessed:
//
//	asserting range loops in the suite ............. 335
//	minus loops over a non-empty composite literal . 208   table tests; cannot be empty
//	minus loops with a length or emptiness guard ...  46
//	minus loops in functions that assert elsewhere .  44
//	                                                 ---
//	remaining                                        37
//
// Without the second narrowing it flagged 235 of 335 — 70% of the suite, which
// is a rule nobody can act on. Without the fourth it flagged tests that still
// check something when the loop is skipped; those are weaker, not hollow.
//
// "Asserts elsewhere" excludes NoError/NotNil and friends on purpose. Those
// establish that the loop *can* run; they are not the behaviour under test.
// Counting them as substance is precisely what let the original defect through.
//
// The rule earned itself on its first run: it flagged TestSearchFTS_FilterByTag,
// which asserted only that each result carried a non-empty ID — true of every
// row the query could return, under any filter or none. Deleting the tag clause
// from the production SQL left it green. Fixed in the same change.
func TestNoAssertionsOnlyInsideALoopThatMayNotRun(t *testing.T) {
	var found []string
	var scanned int

	for _, root := range []string{filepath.Join("..", "cmd"), filepath.Join("..", "internal")} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
				return walkErr
			}
			fset := token.NewFileSet()
			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return nil // an unparseable file is not this test's business
			}
			base := filepath.Base(path)

			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				scanned++
				if grandfathered[site{base, fn.Name.Name}] || assertsOutsideAnyRange(fn.Body) {
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					rng, ok := n.(*ast.RangeStmt)
					if !ok || !bodyAsserts(rng.Body) || cannotBeEmpty(fn.Body, rng) {
						return true
					}
					found = append(found, fmt.Sprintf("%s:%d %s (range %s)",
						base, fset.Position(rng.Pos()).Line, fn.Name.Name, exprText(fset, rng.X)))
					return true
				})
			}
			return nil
		})
		require.NoError(t, err)
	}

	// Vacuity guard. Every assertion above lives inside a directory walk; if the
	// walk found no test files, this test would pass having examined nothing —
	// which is the defect it exists to reject, one level up.
	require.Greater(t, scanned, 500,
		"expected to scan thousands of test functions, scanned %d — this run proves nothing", scanned)

	require.Empty(t, found,
		"every assertion in these functions sits inside a range loop with no guarantee of "+
			"running.\nIf the collection comes back empty the test passes having checked nothing.\n"+
			"Add require.NotEmpty on the collection, or assert something outside the loop:\n  %s",
		strings.Join(found, "\n  "))
}

type site struct{ file, fn string }

// grandfathered lists the sites that predate this rule. They are KNOWN, not
// endorsed — each may assert nothing if its collection comes back empty.
//
// The list only shrinks. A new entry means a new hollow test, and that is what
// the diff should show.
//
// Worth attention first, because these range over the system's own output and
// so would pass silently if it produced nothing: catalog_test.go over
// c.Commands(), related_test.go over result.Related, contextpack_test.go over
// result.Context. fts_filter_test.go was in this group until it was fixed —
// and it was genuinely broken, not merely at risk.
var grandfathered = map[site]bool{}

func init() {
	for _, s := range []site{
		{"activation_reranker_test.go", "TestActivationReranker_ActivationCannotIntroduceCandidates"},
		{"bank_rate_test.go", "bankVault"},
		{"catalog_test.go", "TestBuild_CommandsSortedByPathWithinGroup"},
		{"catalog_test.go", "TestBuild_OmitsGroupWithNoCommands"},
		{"catalog_test.go", "TestCommandCatalog_HiddenAliasesExcluded"},
		{"catalog_test.go", "TestCommandCatalog_WhenComposedIntoLong"},
		{"checks_test.go", "TestFilterTestOutput"},
		{"completion_test.go", "TestCompletionCommandExecution"},
		{"config_namespace_test.go", "parseConfigPrefixes"},
		{"console_handler_test.go", "TestConsoleHandler"},
		{"context_header_invariant_test.go", "atoi"},
		{"contextpack_excerpt_test.go", "TestContextPack_ExcerptCapsItemsThatWouldOtherwiseFit"},
		{"contextpack_test.go", "TestContextPack_BodyBackfillConsistency"},
		{"contextpack_test.go", "TestContextPack_EdgePriority_ExplicitEmbed"},
		{"crosslang_fixture_test.go", "TestCrossLanguageFixture_MessageCasesLoadThenRejectAtMessage"},
		{"doctor_heal_test.go", "TestTopLevelLint_HiddenFromRootListing"},
		{"doctor_summary_test.go", "TestVaultParent_HiddenFromRootListing"},
		{"doctor_test.go", "TestFormatResults"},
		{"edges_test.go", "TestLinksOut_FilterByEdgeType"},
		{"executor_test.go", "TestBuildCategories_CheckMetadata"},
		{"heads_test.go", "TestL2Normalize_ZeroVector"},
		{"hooks_status_test.go", "writeCanonical"},
		{"index_config_test.go", "TestIndexOptions_JSONDefaultFalse"},
		{"index_truth_test.go", "indexedVault"},
		{"initvault_test.go", "TestInit_WikilinksAreObsidianCompatible"},
		{"log_handler_test.go", "TestLogHandler_OnProgress_OptionalFields"},
		{"memory_taxonomy_test.go", "TestTopLevelLinks_HiddenFromRootListing"},
		{"note_create_test.go", "setupNoteCreateVault"},
		{"related_test.go", "TestRelated_ExplicitOnly"},
		{"related_test.go", "TestRelated_InferredOnly"},
		{"relevance_test.go", "TestRelevance_ProbeFixture"},
		{"renderer_test.go", "TestRenderError"},
		{"renderer_test.go", "TestRenderSuccess"},
		{"scanner_test.go", "TestScan_ExcludesPatterns"},
		{"status_nilguard_test.go", "TestCollectTypeBreakdown_NilStatusesNormalisedToEmpty"},
		{"strict_verify_fixture_test.go", "TestStrictVerify_CrossLangFixture"},
		{"summary_test.go", "TestPrintFinalSummary"},
	} {
		grandfathered[s] = true
	}
}

// cannotBeEmpty reports whether the ranged collection is known to have at least
// one element, so skipping the loop is impossible.
func cannotBeEmpty(body *ast.BlockStmt, rng *ast.RangeStmt) bool {
	// `for _, tc := range []T{a, b}` — the literal is right there.
	if lit, ok := rng.X.(*ast.CompositeLit); ok && len(lit.Elts) > 0 {
		return true
	}
	// `for i := range 4` — an integer range, not a collection.
	if lit, ok := rng.X.(*ast.BasicLit); ok && lit.Kind == token.INT {
		return true
	}
	// The dominant table-test shape assigns the literal to a local first:
	// `tests := []struct{...}{...}` then `for _, tt := range tests`.
	if ident, ok := rng.X.(*ast.Ident); ok && assignedNonEmptyLiteral(body, ident.Name) {
		return true
	}
	// An explicit guard on the collection: require.NotEmpty(t, xs), Len(t, xs, n),
	// or any len(xs) test. Matched on the rendered expression so that field and
	// call expressions (result.Context, c.Commands()) work as well as identifiers.
	expr := exprTextOf(rng.X)
	for _, guard := range []string{"len(" + expr + ")", "NotEmpty(t, " + expr, "Len(t, " + expr} {
		if strings.Contains(exprTextOf(body), guard) {
			return true
		}
	}
	return false
}

// assignedNonEmptyLiteral reports whether name is assigned a composite literal
// with at least one element anywhere in the function.
func assignedNonEmptyLiteral(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if !ok || ident.Name != name || i >= len(assign.Rhs) {
				continue
			}
			if lit, ok := assign.Rhs[i].(*ast.CompositeLit); ok && len(lit.Elts) > 0 {
				found = true
			}
		}
		return true
	})
	return found
}

// bodyAsserts reports whether the block makes any assert/require call.
func bodyAsserts(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if name, ok := assertionName(n); ok && name != "" {
			found = true
		}
		return true
	})
	return found
}

// assertsOutsideAnyRange reports whether the function makes a SUBSTANTIVE
// assertion outside every range loop — one that is not merely establishing that
// the loop can run.
func assertsOutsideAnyRange(body *ast.BlockStmt) bool {
	insideRange := map[ast.Node]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if rng, ok := n.(*ast.RangeStmt); ok {
			ast.Inspect(rng.Body, func(m ast.Node) bool {
				insideRange[m] = true
				return true
			})
		}
		return true
	})

	substantive := false
	ast.Inspect(body, func(n ast.Node) bool {
		if insideRange[n] {
			return true
		}
		if name, ok := assertionName(n); ok && !precondition[name] {
			substantive = true
		}
		return true
	})
	return substantive
}

// assertionName returns the method name of an assert.X / require.X call.
func assertionName(n ast.Node) (string, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || (pkg.Name != "assert" && pkg.Name != "require") {
		return "", false
	}
	return sel.Sel.Name, true
}

// precondition names the assertions that establish the loop can run at all
// rather than checking the behaviour under test. The hollow test this rule
// exists for had require.NoError and require.NotNil outside its loop; a rule
// counting those as substance would have passed it.
var precondition = map[string]bool{
	"NoError": true, "NoErrorf": true,
	"Error": true, "Errorf": true, "ErrorIs": true, "ErrorAs": true,
	"NotNil": true, "NotNilf": true, "Nil": true, "Nilf": true,
}

func exprText(fset *token.FileSet, node ast.Node) string {
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, fset, node)
	return buf.String()
}

// exprTextOf renders without position info, for substring matching within a
// function body.
func exprTextOf(node ast.Node) string {
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, token.NewFileSet(), node)
	return buf.String()
}
