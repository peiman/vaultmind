//go:build !windows

package memguard

import (
	"runtime"
	"syscall"
)

// PeakRSS reports this process's peak resident memory in bytes, or 0 when it
// cannot be read. getrusage reports ru_maxrss in bytes on macOS and in
// kilobytes on Linux. A peak, not the current size: a process that has grown
// past its cap stops, which is what the cap is for.
func PeakRSS() int64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	if runtime.GOOS == "darwin" {
		return ru.Maxrss
	}
	return ru.Maxrss * 1024
}
