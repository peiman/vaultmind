package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The stale-sparse line is printed, so the Issues rollup must count it.
// A report that warns and then says "0 warnings" denies its own output.
func TestResultSurfacedIssueCounts_CountsStaleSparse(t *testing.T) {
	_, warns := ResultSurfacedIssueCounts(&DoctorResult{
		Embeddings: &DoctorEmbeddings{SparseFormatStale: true},
	})
	assert.Equal(t, 1, warns, "the stale-sparse warning is printed; it must be counted")

	_, fresh := ResultSurfacedIssueCounts(&DoctorResult{
		Embeddings: &DoctorEmbeddings{},
	})
	assert.Equal(t, 0, fresh, "a current sparse format adds no warning")

	_, none := ResultSurfacedIssueCounts(&DoctorResult{})
	assert.Equal(t, 0, none, "no embeddings section adds no warning")
}
