package app

import (
	"errors"

	"github.com/ginkcode/agent-sessions/internal/engine"
)

// SessionSearchReply is the result of SearchSession. Supported is false when
// the selected backend cannot search a session (an older host, or a backend
// without engine.SessionSearchBackend).
type SessionSearchReply struct {
	Supported bool                        `json:"supported"`
	Result    *engine.SessionSearchResult `json:"result,omitempty"`
}

// SearchSession finds the messages of one session containing every term of
// a query. An offline host keeps its disconnected error.
func (a *App) SearchSession(req SessionSearchRequest) (SessionSearchReply, error) {
	s, ok := a.route().backend.(engine.SessionSearchBackend)
	if !ok {
		return SessionSearchReply{}, nil
	}
	result, err := s.SearchSession(a.appCtx(), req)
	if errors.Is(err, engine.ErrUnsupportedAction) {
		return SessionSearchReply{}, nil
	}
	if err != nil {
		return SessionSearchReply{}, err
	}
	return SessionSearchReply{Supported: true, Result: &result}, nil
}
