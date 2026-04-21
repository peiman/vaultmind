package query_test

import (
	"testing"

	"github.com/peiman/vaultmind/internal/query"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Doctor's duplicate-ID counter must detect rows that share an ID. The
// schema has a UNIQUE constraint on notes.id; the counter exists as a
// belt-and-suspenders guard in case that constraint is ever dropped by a
// migration bug. Losing the guard would let the index silently hold
// conflicting notes under the same ID.
func TestDoctor_DuplicateIDCounterExistsAndReturnsZeroForCleanVault(t *testing.T) {
	db, dir := smallIndexedVault(t)
	result, err := query.Doctor(db, dir)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Issues.DuplicateIDs,
		"clean vault must report zero duplicate IDs")
}

// Doctor.IndexStatus starts at "current" so downstream tools can rely on
// a populated value even when there's nothing special to flag. A
// zero-value string would force every consumer to branch on empty-vs-set.
func TestDoctor_IndexStatusIsAlwaysPopulated(t *testing.T) {
	db, dir := smallIndexedVault(t)
	result, err := query.Doctor(db, dir)
	require.NoError(t, err)
	assert.NotEmpty(t, result.IndexStatus,
		"IndexStatus must always have a value so JSON consumers don't branch on empty")
}

// Doctor.VaultPath must echo the path passed in — this is what consumers
// use as the anchor to associate the report with a specific vault when
// they fan out across multiple.
func TestDoctor_VaultPathEchoesInput(t *testing.T) {
	db, dir := smallIndexedVault(t)
	result, err := query.Doctor(db, dir)
	require.NoError(t, err)
	assert.Equal(t, dir, result.VaultPath)
}
