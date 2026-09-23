//go:build !cgo || !ORT

package embedding

import (
	"context"

	"github.com/knights-analytics/hugot"
)

// newBGEM3Session creates a pure Go hugot session for BGE-M3.
// This file is compiled when building WITHOUT -tags ORT.
// Warning: pure Go backend is very slow for BGE-M3 indexing (hours for 130 notes).
// It is acceptable for query-time embedding of short texts (~1s).
func newBGEM3Session(ctx context.Context) (*hugot.Session, error) {
	return hugot.NewGoSession(ctx)
}

// BackendName identifies which hugot backend the binary was built against.
// Consumers (e.g. the index command) use this to warn when BGE-M3 indexing
// is about to run on the slow pure-Go path so operators don't mistake
// "hours-long indexing" for a hang or OOM. Reported by the build tag.
func BackendName() string { return BackendNameGo }

// Acceleration mirrors the ORT-build's Acceleration() so callers don't
// need to special-case build tags. Pure-Go has no GPU path; "go-cpu"
// names the slow path explicitly.
func Acceleration() string { return "go-cpu" }

// CheckRuntime is a no-op on the pure-Go build: there is no native runtime to
// be the wrong version. See the ORT build's CheckRuntime.
func CheckRuntime(context.Context) error { return nil }
