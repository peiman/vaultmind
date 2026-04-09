package experiment

import "context"

// contextKey is the unexported key type used to store Session in a context.
type contextKey struct{}

// Session holds the active experiment session and its associated database.
type Session struct {
	DB            *DB
	ID            string
	VaultPath     string
	OutcomeWindow int // configurable outcome window; 0 uses default (2)
}

// SetVaultPath updates the vault path on the session and persists it to the DB.
func (s *Session) SetVaultPath(vaultPath string) {
	s.VaultPath = vaultPath
	_ = s.DB.UpdateSessionVaultPath(s.ID, vaultPath)
}

// outcomeWindow returns the configured outcome window, defaulting to 2.
func (s *Session) outcomeWindow() int {
	if s.OutcomeWindow > 0 {
		return s.OutcomeWindow
	}
	return 2
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

// LogSearchEvent logs a search event from this session.
func (s *Session) LogSearchEvent(query, mode string, data map[string]any) (string, error) {
	if data == nil {
		data = map[string]any{}
	}
	return s.DB.LogEvent(Event{
		SessionID: s.ID,
		Type:      EventSearch,
		VaultPath: s.VaultPath,
		QueryText: query,
		QueryMode: mode,
		Data:      data,
	})
}

// LogAskEvent logs an ask event from this session.
func (s *Session) LogAskEvent(query string, data map[string]any) (string, error) {
	if data == nil {
		data = map[string]any{}
	}
	return s.DB.LogEvent(Event{
		SessionID: s.ID,
		Type:      EventAsk,
		VaultPath: s.VaultPath,
		QueryText: query,
		Data:      data,
	})
}

// LogContextPackEvent logs a context_pack event from this session.
func (s *Session) LogContextPackEvent(data map[string]any) (string, error) {
	if data == nil {
		data = map[string]any{}
	}
	return s.DB.LogEvent(Event{
		SessionID: s.ID,
		Type:      EventContextPack,
		VaultPath: s.VaultPath,
		Data:      data,
	})
}

// LogNoteAccessEvent logs a note_access event and triggers outcome linkage.
func (s *Session) LogNoteAccessEvent(noteID, source string) (string, error) {
	eventID, err := s.DB.LogEvent(Event{
		SessionID: s.ID,
		Type:      EventNoteAccess,
		VaultPath: s.VaultPath,
		Data:      map[string]any{"note_id": noteID, "source": source},
	})
	if err != nil {
		return "", err
	}
	_, _ = s.DB.LinkOutcomes(s.ID, noteID, s.outcomeWindow())
	return eventID, nil
}
