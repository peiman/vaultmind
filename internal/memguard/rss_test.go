package memguard_test

import (
	"runtime"
	"testing"

	"github.com/peiman/vaultmind/internal/memguard"
	"github.com/stretchr/testify/assert"
)

func TestPeakRSS_ReportsThisProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		assert.Zero(t, memguard.PeakRSS())
		return
	}
	got := memguard.PeakRSS()
	assert.Greater(t, got, int64(1<<20), "a running test binary holds more than 1 MiB")
	assert.Less(t, got, int64(1<<40), "units are bytes, not a raw kilobyte count misread")
}
