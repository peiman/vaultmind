package index_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/index"
	"github.com/stretchr/testify/require"
)

func TestSearchFTS_SpecialCharsDoNotCrash(t *testing.T) {
	db := rebuildTestIndex(t)

	tests := []string{
		`"unclosed quote`,
		`(unclosed paren`,
		`colon:in:query`,
		`dash-in-query`,
		`AND OR NOT`,
		`*wildcard`,
		`query with "quotes"`,
	}

	for _, q := range tests {
		t.Run(q, func(t *testing.T) {
			results, err := index.SearchFTS(db, q, 10, 0)
			require.NoError(t, err, "query %q must not crash", q)
			_ = results // may be empty, that's fine
		})
	}
}
