package memguard_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/peiman/vaultmind/internal/memguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A watched process that grows past its limit is reported once, with the
// size that crossed it.
func TestWatch_ReportsOnceWhenMemoryPassesTheLimit(t *testing.T) {
	var size, reported, calls atomic.Int64
	size.Store(100)
	stop := memguard.Watch(1000, time.Millisecond, size.Load, func(used int64) {
		reported.Store(used)
		calls.Add(1)
	})
	defer stop()

	size.Store(5000)
	require.Eventually(t, func() bool { return calls.Load() > 0 }, 2*time.Second, time.Millisecond,
		"memory passed the limit and was not reported")
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, int64(5000), reported.Load())
	assert.Equal(t, int64(1), calls.Load(), "the crossing is reported once")
}

func TestWatch_UnderTheLimitIsNeverReported(t *testing.T) {
	var calls atomic.Int32
	stop := memguard.Watch(1000, time.Millisecond, func() int64 { calls.Add(1); return 999 }, func(int64) {
		t.Error("reported a size under the limit")
	})
	require.Eventually(t, func() bool { return calls.Load() >= 5 }, 2*time.Second, time.Millisecond, "the watch never sampled")
	stop()
}

// A zero limit, or no way to read memory, turns the watch off.
func TestWatch_OffWithoutALimitOrAReader(t *testing.T) {
	never := func(int64) { t.Error("an off watch reported") }
	memguard.Watch(0, time.Millisecond, func() int64 { return 1 << 40 }, never)()
	memguard.Watch(1000, time.Millisecond, nil, never)()
	time.Sleep(10 * time.Millisecond)
}

func TestWatch_StopEndsSampling(t *testing.T) {
	var calls atomic.Int32
	stop := memguard.Watch(1000, time.Millisecond, func() int64 { calls.Add(1); return 1 }, func(int64) {})
	require.Eventually(t, func() bool { return calls.Load() > 0 }, 2*time.Second, time.Millisecond)
	stop()
	after := calls.Load()
	time.Sleep(20 * time.Millisecond)
	assert.LessOrEqual(t, calls.Load(), after+1, "sampling continued after stop")
}

// A sample that is under way when the work finishes does not report: the
// pass is done, so stopping it now would only turn a success into an error.
func TestWatch_NoReportAfterStop(t *testing.T) {
	sampling, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var reported atomic.Bool
	stop := memguard.Watch(1000, time.Millisecond, func() int64 {
		once.Do(func() { close(sampling) })
		<-release
		return 5000
	}, func(int64) { reported.Store(true) })

	<-sampling
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	time.Sleep(10 * time.Millisecond) // stop has closed its done channel and is waiting
	close(release)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not return")
	}
	assert.False(t, reported.Load(), "a crossing read after stop was reported")
}
