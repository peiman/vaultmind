package index_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRebuild_DatesStoredAsISO8601(t *testing.T) {
	db := rebuildTestIndex(t)

	// ACT-R has created: 2026-04-03
	var created string
	require.NoError(t, db.QueryRow("SELECT created FROM notes WHERE id = ?", "concept-act-r").Scan(&created))

	// Must be ISO 8601 date, NOT Go time.Time string
	assert.Equal(t, "2026-04-03", created, "dates must be stored as ISO 8601, not time.Time.String()")
	assert.NotContains(t, created, "00:00:00", "must not contain time component")
	assert.NotContains(t, created, "UTC", "must not contain timezone")
}
