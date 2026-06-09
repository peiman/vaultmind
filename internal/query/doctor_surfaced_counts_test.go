package query

import "testing"

// TestSurfacedIssueCounts pins the severity classification of the SURFACED
// doctor issue set — the single source of truth for the "Issues: E errors, W
// warnings" text rollup. It must count the items doctor actually renders, never
// the schema-validation aggregate (which the text report does not surface).
func TestSurfacedIssueCounts(t *testing.T) {
	tests := []struct {
		name         string
		issues       DoctorIssues
		wantErrors   int
		wantWarnings int
	}{
		{
			name:         "empty => zero",
			issues:       DoctorIssues{},
			wantErrors:   0,
			wantWarnings: 0,
		},
		{
			name: "integrity violations count as errors",
			issues: DoctorIssues{
				DuplicateIDs:          1,
				MissingRequiredFields: 2,
				MalformedMarkers:      3,
				NotesMissingIDOrType:  4,
				PathPseudoIDLinks:     5,
			},
			wantErrors:   15,
			wantWarnings: 0,
		},
		{
			name: "advisories count as warnings",
			issues: DoctorIssues{
				UnresolvedLinks:           1,
				BrokenReferences:          2,
				ObsidianIncompatibleLinks: 3,
				StaleIndex:                4,
				HookDrift:                 5,
				LegacyHooksJSON:           true,
			},
			wantErrors:   0,
			wantWarnings: 16,
		},
		{
			name: "legacy hooks json adds exactly one warning",
			issues: DoctorIssues{
				LegacyHooksJSON: true,
			},
			wantErrors:   0,
			wantWarnings: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotErrors, gotWarnings := SurfacedIssueCounts(tc.issues)
			if gotErrors != tc.wantErrors {
				t.Errorf("errors = %d, want %d", gotErrors, tc.wantErrors)
			}
			if gotWarnings != tc.wantWarnings {
				t.Errorf("warnings = %d, want %d", gotWarnings, tc.wantWarnings)
			}
		})
	}
}
