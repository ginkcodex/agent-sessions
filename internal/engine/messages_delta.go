package engine

import (
	"context"
	"fmt"

	"github.com/ginkcode/agent-sessions/internal/model"
)

// MessagesSinceRequest asks for a page as GetMessages does, given a window
// of messages the client already holds from page revision BaseRevision.
type MessagesSinceRequest struct {
	Ref model.SessionRef `json:"ref"`
	// BaseRevision is the revision of the client's window; "" for none.
	BaseRevision string `json:"baseRevision"`
	// The client's window: messages [HaveOffset, HaveOffset+HaveCount).
	HaveOffset int `json:"haveOffset"`
	HaveCount  int `json:"haveCount"`
	// Offset and Limit select the page, as in GetMessages: -1 is the latest.
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// MessagesDelta is the requested page without its first Reused messages,
// which equal the client's copies: the page's messages are the client's
// [Offset-HaveOffset, Offset-HaveOffset+Reused) followed by Messages. Reused
// is 0 when the base revision is unknown or older than the retained history,
// which makes the delta a full page.
type MessagesDelta struct {
	MessagesPage
	Reused int `json:"reused"`
}

// TranscriptDeltaBackend is optional so older host backends remain compatible.
type TranscriptDeltaBackend interface {
	GetMessagesSince(ctx context.Context, req MessagesSinceRequest) (MessagesDelta, error)
}

var _ TranscriptDeltaBackend = (*Engine)(nil)

// GetMessagesSince returns a page of a session transcript, omitting its
// leading messages the client's window already holds unchanged.
func (e *Engine) GetMessagesSince(ctx context.Context, req MessagesSinceRequest) (MessagesDelta, error) {
	return e.svc.GetMessagesSince(ctx, req)
}

// GetMessagesSince returns a page of a session transcript, omitting its
// leading messages the client's window already holds unchanged.
func (s *Service) GetMessagesSince(ctx context.Context, req MessagesSinceRequest) (MessagesDelta, error) {
	if req.Offset < -1 || req.Limit <= 0 || req.HaveOffset < 0 || req.HaveCount < 0 {
		return MessagesDelta{}, fmt.Errorf("invalid page offset=%d limit=%d haveOffset=%d haveCount=%d", req.Offset, req.Limit, req.HaveOffset, req.HaveCount)
	}
	all, rev, err := s.loadTranscriptRevision(ctx, req.Ref)
	if err != nil {
		return MessagesDelta{}, err
	}
	page, start := pageOf(all, rev, req.Offset, req.Limit)
	reused := 0
	if start >= req.HaveOffset {
		if from, ok := s.transcripts.changedSince(req.Ref.Key(), req.BaseRevision, rev); ok {
			// Messages before from are unchanged since the base, and the
			// client holds those inside its window.
			reused = max(0, min(from, req.HaveOffset+req.HaveCount, start+len(page.Messages))-start)
		}
	}
	page.Messages = page.Messages[reused:]
	return MessagesDelta{MessagesPage: page, Reused: reused}, nil
}
