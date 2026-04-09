package experiment

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Time partitioning tests
func TestPartitionTime_AllActive(t *testing.T) {
	start := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 9, 10, 30, 0, 0, time.UTC)
	windows := []SessionWindow{{
		Start: time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 4, 9, 11, 0, 0, 0, time.UTC),
	}}
	active, idle := PartitionTime(start, end, windows)
	assert.Equal(t, 30*time.Minute, active)
	assert.Equal(t, time.Duration(0), idle)
}

func TestPartitionTime_AllIdle(t *testing.T) {
	start := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 9, 14, 0, 0, 0, time.UTC)
	windows := []SessionWindow{{
		Start: time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 4, 9, 11, 0, 0, 0, time.UTC),
	}}
	active, idle := PartitionTime(start, end, windows)
	assert.Equal(t, time.Duration(0), active)
	assert.Equal(t, 2*time.Hour, idle)
}

func TestPartitionTime_Mixed(t *testing.T) {
	start := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	windows := []SessionWindow{{
		Start: time.Date(2026, 4, 9, 10, 30, 0, 0, time.UTC),
		End:   time.Date(2026, 4, 9, 11, 0, 0, 0, time.UTC),
	}}
	active, idle := PartitionTime(start, end, windows)
	assert.Equal(t, 30*time.Minute, active)
	assert.Equal(t, 90*time.Minute, idle)
}

func TestPartitionTime_MultipleSessions(t *testing.T) {
	start := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 9, 15, 0, 0, 0, time.UTC)
	windows := []SessionWindow{
		{Start: time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC), End: time.Date(2026, 4, 9, 11, 0, 0, 0, time.UTC)},
		{Start: time.Date(2026, 4, 9, 13, 0, 0, 0, time.UTC), End: time.Date(2026, 4, 9, 14, 0, 0, 0, time.UTC)},
	}
	active, idle := PartitionTime(start, end, windows)
	assert.Equal(t, 2*time.Hour, active)
	assert.Equal(t, 3*time.Hour, idle)
}

func TestPartitionTime_NoSessions(t *testing.T) {
	start := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	active, idle := PartitionTime(start, end, nil)
	assert.Equal(t, time.Duration(0), active)
	assert.Equal(t, 2*time.Hour, idle)
}

func TestPartitionTime_ZeroDuration(t *testing.T) {
	now := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	active, idle := PartitionTime(now, now, nil)
	assert.Equal(t, time.Duration(0), active)
	assert.Equal(t, time.Duration(0), idle)
}

func TestPartitionTime_OverlappingWindows(t *testing.T) {
	start := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	windows := []SessionWindow{
		{Start: time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC), End: time.Date(2026, 4, 9, 11, 0, 0, 0, time.UTC)},
		{Start: time.Date(2026, 4, 9, 10, 30, 0, 0, time.UTC), End: time.Date(2026, 4, 9, 11, 30, 0, 0, time.UTC)},
	}
	active, idle := PartitionTime(start, end, windows)
	assert.Equal(t, 90*time.Minute, active)
	assert.Equal(t, 30*time.Minute, idle)
}

// CompressedElapsed tests
func TestCompressedElapsed_Active(t *testing.T) {
	d := CompressedElapsed(30*time.Minute, 0, 0.2)
	assert.InDelta(t, 30.0, d.Minutes(), 0.01)
}

func TestCompressedElapsed_Idle(t *testing.T) {
	d := CompressedElapsed(0, 60*time.Minute, 0.2)
	assert.InDelta(t, 12.0, d.Minutes(), 0.01)
}

func TestCompressedElapsed_Mixed(t *testing.T) {
	d := CompressedElapsed(30*time.Minute, 60*time.Minute, 0.2)
	assert.InDelta(t, 42.0, d.Minutes(), 0.01)
}

func TestCompressedElapsed_WallClock(t *testing.T) {
	d := CompressedElapsed(30*time.Minute, 60*time.Minute, 1.0)
	assert.InDelta(t, 90.0, d.Minutes(), 0.01)
}

// ComputeRetrieval tests
func TestComputeRetrieval_SingleAccess(t *testing.T) {
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	accessTimes := []time.Time{time.Date(2026, 4, 9, 11, 0, 0, 0, time.UTC)}
	// All idle, gamma=0.2: effective = 0.2*1h = 0.2h
	// retrieval = ln(0.2^(-0.5)) = ln(1/sqrt(0.2)) ~ 0.805
	score := ComputeRetrieval(accessTimes, now, nil, 0.2, 0.5)
	assert.InDelta(t, 0.805, score, 0.05)
}

func TestComputeRetrieval_MultipleAccesses(t *testing.T) {
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	accessTimes := []time.Time{
		time.Date(2026, 4, 9, 11, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC),
	}
	// gamma=1.0 (wall-clock): t1=1h, t2=2h
	// sum = 1^(-0.5) + 2^(-0.5) = 1.0 + 0.707 = 1.707
	// retrieval = ln(1.707) ~ 0.535
	score := ComputeRetrieval(accessTimes, now, nil, 1.0, 0.5)
	assert.InDelta(t, 0.535, score, 0.05)
}

func TestComputeRetrieval_NoAccesses(t *testing.T) {
	score := ComputeRetrieval(nil, time.Now(), nil, 0.2, 0.5)
	assert.Equal(t, 0.0, score)
}

// Storage and combined tests
func TestComputeStorage(t *testing.T) {
	assert.InDelta(t, 0.0, ComputeStorage(0), 0.001)
	assert.InDelta(t, 0.693, ComputeStorage(1), 0.01)
	assert.InDelta(t, 1.099, ComputeStorage(2), 0.01)
	assert.InDelta(t, 2.398, ComputeStorage(10), 0.01)
}

func TestComputeStorage_Negative(t *testing.T) {
	assert.Equal(t, 0.0, ComputeStorage(-1))
	assert.Equal(t, 0.0, ComputeStorage(-100))
}

func TestCombinedScore(t *testing.T) {
	// Without similarity (delta=0): alpha*retrieval + beta*storage
	score := CombinedScore(1.0, 2.0, 0.0, 0.6, 0.4, 0.0)
	assert.InDelta(t, 1.4, score, 0.001)
}

func TestCombinedScore_WithSimilarity(t *testing.T) {
	// alpha*retrieval + beta*storage + delta*similarity
	// 0.5*1.0 + 0.3*2.0 + 0.2*0.8 = 0.5 + 0.6 + 0.16 = 1.26
	score := CombinedScore(1.0, 2.0, 0.8, 0.5, 0.3, 0.2)
	assert.InDelta(t, 1.26, score, 0.001)
}

func TestCombinedScore_SimilarityOnly(t *testing.T) {
	// No access history, pure similarity
	score := CombinedScore(0.0, 0.0, 0.95, 0.0, 0.0, 1.0)
	assert.InDelta(t, 0.95, score, 0.001)
}
