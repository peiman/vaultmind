package experiment

import "context"

// contextKey is the unexported key type used to store Session in a context.
type contextKey struct{}

// Session holds the active experiment session and its associated database.
type Session struct {
	DB        *DB
	ID        string
	VaultPath string
}

// WithSession stores s in the context and returns the updated context.
func WithSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, contextKey{}, s)
}

// FromContext retrieves the Session from ctx. Returns nil if no Session is
// stored in the context.
func FromContext(ctx context.Context) *Session {
	s, _ := ctx.Value(contextKey{}).(*Session)
	return s
}
