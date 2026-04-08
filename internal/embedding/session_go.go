//go:build !cgo || !ORT

package embedding

import "github.com/knights-analytics/hugot"

// newBGEM3Session creates a pure Go hugot session for BGE-M3.
// This file is compiled when building WITHOUT -tags ORT.
// Warning: pure Go backend is very slow for BGE-M3 indexing (hours for 130 notes).
// It is acceptable for query-time embedding of short texts (~1s).
func newBGEM3Session() (*hugot.Session, error) {
	return hugot.NewGoSession()
}
