//go:build windows

package memguard

// PeakRSS is not measured on Windows; 0 leaves the memory cap off there.
func PeakRSS() int64 { return 0 }
