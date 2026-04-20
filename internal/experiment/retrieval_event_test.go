package experiment_test

import (
	"errors"
	"testing"

	"github.com/peiman/vaultmind/internal/experiment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRetrievalEventData_SuccessOmitsErrorField(t *testing.T) {
	variants := experiment.BuildVariantPayload("hybrid", []experiment.RetrievalHit{
		{NoteID: "n1", Rank: 1},
	})

	got := experiment.BuildRetrievalEventData(variants, 1, nil)

	assert.Equal(t, 1, got["result_count"])
	_, has := got["error"]
	assert.False(t, has, "error field should be absent on success")
	assert.Equal(t, variants, got["variants"])
}

func TestBuildRetrievalEventData_ZeroHitsIsDistinctFromError(t *testing.T) {
	variants := experiment.BuildVariantPayload("hybrid", nil)

	got := experiment.BuildRetrievalEventData(variants, 0, nil)

	assert.Equal(t, 0, got["result_count"])
	_, has := got["error"]
	assert.False(t, has, "zero hits is a successful retrieval — no error field")
}

func TestBuildRetrievalEventData_ErrorPopulatesErrorField(t *testing.T) {
	variants := experiment.BuildVariantPayload("hybrid", nil)

	got := experiment.BuildRetrievalEventData(variants, 0, errors.New("connection refused"))

	assert.Equal(t, 0, got["result_count"])
	assert.Equal(t, "connection refused", got["error"])
}

func TestBuildRetrievalEventData_ResultCountCanDifferFromVariantHits(t *testing.T) {
	// Caller may pass the variant payload and the result count independently —
	// e.g. when limit truncates the variant hits but the total count is known.
	variants := experiment.BuildVariantPayload("hybrid", []experiment.RetrievalHit{
		{NoteID: "n1", Rank: 1},
	})

	got := experiment.BuildRetrievalEventData(variants, 42, nil)

	assert.Equal(t, 42, got["result_count"])
	require.NotNil(t, got["variants"])
}
