package experiment

import (
	"reflect"
	"strings"
	"testing"
)

// The federated-export structs (CalibrationSnapshot, Rollup) MUST stay
// content-free: they leave the user's machine in the Paper #2 aggregation, so
// no field may carry a note id, content, query text, or a path. Prose comments
// say this; this test ENFORCES it (Principle 9) so a future field like
// `TopNoteID string` or `SampleQuery string` trips a compile-gated guard
// instead of silently exfiltrating content.
//
// Two rules per struct:
//  1. No field name contains a content-shaped substring (with a tiny allowlist
//     for legitimate non-content uses like the random CalibrationID token).
//  2. Every field is a scalar (string/int/float/bool) — no maps/slices/nested
//     structs that could smuggle a content payload. (TypeDistribution on Rollup
//     is the one allowed map: it carries type *names* + counts, already exported
//     today and shown in the export preview; allowlisted explicitly.)
func TestFederatedExportStructsAreContentFree(t *testing.T) {
	forbidden := []string{
		"note", "content", "body", "query", "text", "path",
		"title", "alias", "snippet", "name",
	}
	// Field names that contain a forbidden substring but are verified non-content
	// (counts and identifiers, not payloads). Pinned explicitly so a genuinely
	// content-shaped field (e.g. NoteContent, NoteBody, TopNoteID) still trips:
	// only these EXACT names pass.
	allowedNames := map[string]bool{
		"CalibrationID":    true, // random hex token, not a note id
		"NoteCount":        true, // an integer count, not note content
		"AliasCount":       true, // an integer count, not an alias string
		"TypeDistribution": true, // type names + counts (public, in export preview)
	}
	// Non-scalar fields verified to carry only labels + numbers, not content.
	allowedNonScalar := map[string]bool{
		"TypeDistribution": true, // map[string]int: type name → count
		"VariantStats":     true, // map keyed by internal variant label → numeric stats
	}
	scalarKinds := map[reflect.Kind]bool{
		reflect.String: true, reflect.Int: true, reflect.Int64: true,
		reflect.Float64: true, reflect.Bool: true,
	}

	check := func(t *testing.T, v any) {
		typ := reflect.TypeOf(v)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			// The invariant guards the SERIALIZATION surface: a `json:"-"` field
			// never leaves the machine via the JSON export, so it can carry a
			// storage-only discriminator (e.g. VaultPath) without leaking. Skip
			// it. (If a non-JSON export path is ever added — a raw SQL dump — it
			// must re-establish this guard for itself.)
			if name := strings.Split(f.Tag.Get("json"), ",")[0]; name == "-" {
				continue
			}
			lower := strings.ToLower(f.Name)
			if !allowedNames[f.Name] {
				for _, bad := range forbidden {
					if strings.Contains(lower, bad) {
						t.Errorf("%s.%s: field name contains content-shaped substring %q — "+
							"federated-export structs must be content-free (add to allowlist only if verified non-content)",
							typ.Name(), f.Name, bad)
					}
				}
			}
			if allowedNonScalar[f.Name] {
				continue // verified non-content map (labels + numbers)
			}
			if !scalarKinds[f.Type.Kind()] {
				t.Errorf("%s.%s: kind %s is not a scalar — a non-scalar field could carry a content payload",
					typ.Name(), f.Name, f.Type.Kind())
			}
		}
	}

	t.Run("CalibrationSnapshot", func(t *testing.T) { check(t, CalibrationSnapshot{}) })
	t.Run("Rollup", func(t *testing.T) { check(t, Rollup{}) })
}
