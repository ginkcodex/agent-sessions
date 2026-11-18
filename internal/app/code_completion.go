package app

import (
	cd "github.com/ginkcode/agent-sessions/internal/codediagnostics"
	"github.com/ginkcode/agent-sessions/internal/engine"
)

// completionRoute is liveRoute plus the separate completion surface. A
// backend without it (an older server) fails with unsupported before anything
// is dispatched.
func (a *App) completionRoute(expectedHostID string) (engine.CodeCompletionBackend, uint64, error) {
	b, gen, err := a.liveRoute(expectedHostID)
	if err != nil {
		return nil, 0, err
	}
	complete, ok := b.(engine.CodeCompletionBackend)
	if !ok {
		return nil, 0, &cd.Error{Code: "unsupported", Message: "Code completion is unavailable on this backend"}
	}
	return complete, gen, nil
}

// CompleteCode asks the language server that holds a live document for
// completions at position, for exactly that version. requestID names the
// request for CancelCodeCompletion; a new request under the same ID cancels
// the old one, and a host switch cancels them all.
func (a *App) CompleteCode(c cd.Context, requestID, documentID string, version int64, position cd.Position, trigger cd.CompletionTrigger, expectedHostID string) (cd.CompletionResult, error) {
	b, gen, err := a.completionRoute(expectedHostID)
	if err != nil {
		return cd.CompletionResult{}, err
	}
	ctx, done := a.completions.start(a.appCtx(), requestID)
	defer done()
	out, err := b.CompleteCode(ctx, c, documentID, version, position, trigger)
	if stale := a.diagnosticsCurrent(gen); stale != nil {
		return cd.CompletionResult{}, stale
	}
	if err != nil && ctx.Err() != nil {
		// A remote client returns ctx.Err() itself once it sends the cancel.
		return cd.CompletionResult{}, &cd.Error{Code: "canceled", Message: "The completion request was canceled"}
	}
	return out, err
}

// ResolveCodeCompletion returns the detail and documentation of item index
// of the document's last completion answer, resultID. With version above 0,
// the document's current version, the item's extra edits for that version
// come too (CC-10).
func (a *App) ResolveCodeCompletion(c cd.Context, documentID, resultID string, index int, version int64, expectedHostID string) (cd.CompletionDetails, error) {
	b, gen, err := a.completionRoute(expectedHostID)
	if err != nil {
		return cd.CompletionDetails{}, err
	}
	out, err := b.ResolveCodeCompletion(a.appCtx(), c, documentID, resultID, index, version)
	if stale := a.diagnosticsCurrent(gen); stale != nil {
		return cd.CompletionDetails{}, stale
	}
	return out, err
}

// CancelCodeCompletion stops the request started under requestID: its
// CompleteCode call returns canceled. Locally the language server gets
// $/cancelRequest; over SSH and WSL the client sends $/cancelRequest and the
// server cancels in turn. It is a no-op once the request has finished.
func (a *App) CancelCodeCompletion(requestID string) {
	a.completions.cancel(requestID)
}
