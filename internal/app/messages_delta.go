package app

import (
	"errors"

	"github.com/ginkcode/agent-sessions/internal/engine"
)

// MessagesSinceReply is the result of GetMessagesSince. Supported is false
// when the selected backend has no delta method (an older host, or a backend
// without engine.TranscriptDeltaBackend); the frontend then uses GetMessages
// with that connection. Host and Generation identify the route that answered,
// as on DiscoveryStatusReply.
type MessagesSinceReply struct {
	Supported  bool                  `json:"supported"`
	Delta      *engine.MessagesDelta `json:"delta,omitempty"`
	Host       string                `json:"host"`
	Generation uint64                `json:"generation"`
}

// GetMessagesSince returns a transcript page without the leading messages the
// caller's window already holds unchanged. An offline host keeps its
// disconnected error.
func (a *App) GetMessagesSince(req MessagesSinceRequest) (MessagesSinceReply, error) {
	r := a.route()
	reply := MessagesSinceReply{Host: r.host}
	if r.host != "" {
		reply.Generation = r.generation
	}
	d, ok := r.backend.(engine.TranscriptDeltaBackend)
	if !ok {
		return reply, nil
	}
	delta, err := d.GetMessagesSince(a.appCtx(), req)
	if errors.Is(err, engine.ErrUnsupportedAction) {
		return reply, nil
	}
	if err != nil {
		return MessagesSinceReply{}, err
	}
	reply.Supported, reply.Delta = true, &delta
	return reply, nil
}
