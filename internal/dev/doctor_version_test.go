package dev

import "testing"

// The floor comparison must be NUMERIC, not lexicographic.
//
// It was `version < "1.25"` on a `\d+\.\d+` string. That is right for every
// version anyone runs today and wrong the moment a minor reaches three digits:
// "1.100" sorts BELOW "1.25" because '1' < '2', so a future Go would be
// reported as too old by a check whose whole job is to report the truth about
// the toolchain. Found by a version-pin sweep, not by a test — it had none.
func TestGoVersionMeetsFloor_ComparesNumericallyNotLexicographically(t *testing.T) {
	for _, tc := range []struct {
		version string
		floor   string
		want    bool
		why     string
	}{
		{"1.27", "1.26", true, "a newer minor satisfies an older floor"},
		{"1.26", "1.26", true, "the floor itself satisfies the floor"},
		{"1.25", "1.26", false, "an older minor does not"},
		{"1.100", "1.26", true, "THE BUG: lexicographic compare calls 1.100 older than 1.26"},
		{"2.0", "1.26", true, "a major bump satisfies an older floor"},
		{"1.9", "1.26", false, "single-digit minor is genuinely older, and must not pass by string luck"},
	} {
		if got := goVersionMeetsFloor(tc.version, tc.floor); got != tc.want {
			t.Errorf("goVersionMeetsFloor(%q, %q) = %v, want %v — %s",
				tc.version, tc.floor, got, tc.want, tc.why)
		}
	}
}
