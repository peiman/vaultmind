package relayclient

import "time"

// SetFetchTimeoutForTest overrides the internal fetch deadline for a single test
// and returns a restore func. It exists ONLY so a deadline test can use a short
// timeout instead of waiting the production 15s; production code never calls it.
func SetFetchTimeoutForTest(d time.Duration) (restore func()) {
	prev := relayFetchTimeout
	relayFetchTimeout = d
	return func() { relayFetchTimeout = prev }
}
