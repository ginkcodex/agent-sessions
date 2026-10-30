package engine

import (
	"context"
	"strings"

	"github.com/ginkcode/agent-sessions/internal/index"
	"github.com/ginkcode/agent-sessions/internal/model"
)

// SessionSearchRequest asks which messages of one session contain every
// term of a query.
type SessionSearchRequest struct {
	Ref   model.SessionRef `json:"ref"`
	Query string           `json:"query"`
	// IncludeHidden also searches meta and system messages, which only the
	// transcript's "all" display level shows.
	IncludeHidden bool `json:"includeHidden,omitempty"`
	// Limit clamps to 1..sessionSearchMaxLimit (default the maximum).
	Limit int `json:"limit,omitempty"`
}

// SessionSearchHit is one matching message.
type SessionSearchHit struct {
	MessageIndex int `json:"messageIndex"`
	// Parts indexes the message's parts containing at least one term.
	Parts []int `json:"parts"`
}

// SessionSearchResult lists matching messages in transcript order.
type SessionSearchResult struct {
	Hits []SessionSearchHit `json:"hits"`
	// Truncated reports more matches past Limit.
	Truncated bool `json:"truncated,omitempty"`
	// Revision is the transcript snapshot searched, as in MessagesPage.
	Revision string `json:"revision,omitempty"`
}

// SessionSearchBackend is optional so older host backends remain compatible.
type SessionSearchBackend interface {
	SearchSession(ctx context.Context, req SessionSearchRequest) (SessionSearchResult, error)
}

var _ SessionSearchBackend = (*Engine)(nil)

const (
	sessionSearchMaxLimit = 5000
	// sessionSearchCacheSize bounds how many sessions keep their folded text.
	sessionSearchCacheSize = 4
	// sessionSearchCheckEvery is how many messages pass between checks for
	// a cancelled search.
	sessionSearchCheckEvery = 256
)

// SearchSession finds the messages of one session containing every term of
// a query, ignoring case and runs of whitespace.
func (e *Engine) SearchSession(ctx context.Context, req SessionSearchRequest) (SessionSearchResult, error) {
	return e.svc.SearchSession(ctx, req)
}

// SearchSession finds the messages of one session containing every term of
// a query, ignoring case and runs of whitespace. Terms follow the global
// search grammar but match anywhere in a word. The full current transcript
// is searched, including tool output, so new and long messages are found.
func (s *Service) SearchSession(ctx context.Context, req SessionSearchRequest) (SessionSearchResult, error) {
	terms, err := index.SearchTerms(req.Query)
	if err != nil {
		return SessionSearchResult{}, err
	}
	for i, term := range terms {
		terms[i] = foldSearchText(term)
	}
	limit := req.Limit
	if limit <= 0 || limit > sessionSearchMaxLimit {
		limit = sessionSearchMaxLimit
	}

	tr, rev, err := s.loadTranscriptRevision(ctx, req.Ref)
	if err != nil {
		return SessionSearchResult{}, err
	}
	texts, err := s.sessionSearchTexts(ctx, req.Ref.Key(), tr, rev)
	if err != nil {
		return SessionSearchResult{}, err
	}

	out := SessionSearchResult{Hits: []SessionSearchHit{}, Revision: rev}
	for i, msg := range texts {
		if i%sessionSearchCheckEvery == 0 {
			if err := ctx.Err(); err != nil {
				return SessionSearchResult{}, err
			}
		}
		if msg.hidden && !req.IncludeHidden {
			continue
		}
		parts, ok := matchSearchMessage(msg.parts, terms)
		if !ok {
			continue
		}
		if len(out.Hits) == limit {
			out.Truncated = true
			break
		}
		out.Hits = append(out.Hits, SessionSearchHit{MessageIndex: i, Parts: parts})
	}
	return out, nil
}

// matchSearchMessage reports whether every term occurs in some part, and
// which parts hold any term.
func matchSearchMessage(parts []string, terms []string) ([]int, bool) {
	for _, term := range terms {
		found := false
		for _, text := range parts {
			if strings.Contains(text, term) {
				found = true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	var hit []int
	for i, text := range parts {
		for _, term := range terms {
			if strings.Contains(text, term) {
				hit = append(hit, i)
				break
			}
		}
	}
	return hit, true
}

// searchMessageText is a message's searchable text, folded part by part.
type searchMessageText struct {
	hidden bool
	parts  []string // indexed like Message.Parts; "" for a part without text
}

// sessionSearchText is a session's folded text at one transcript revision.
type sessionSearchText struct {
	revision string
	messages []searchMessageText
}

// sessionSearchTexts returns the folded text of a transcript, reusing the
// text of messages unchanged since the session's last search, so a live
// session only folds what it appended. A snapshot without a revision is
// folded but not kept.
func (s *Service) sessionSearchTexts(ctx context.Context, key string, tr *model.Transcript, rev string) ([]searchMessageText, error) {
	reuse := 0
	var prev *sessionSearchText
	if rev != "" {
		if cached, ok := s.searchTexts.Get(key); ok {
			if cached.revision == rev {
				return cached.messages, nil
			}
			if from, ok := s.transcripts.changedSince(key, cached.revision, rev); ok {
				prev = cached
				reuse = min(from, len(cached.messages), len(tr.Messages))
			}
		}
	}

	messages := make([]searchMessageText, len(tr.Messages))
	if prev != nil {
		copy(messages, prev.messages[:reuse])
	}
	for i := reuse; i < len(tr.Messages); i++ {
		if (i-reuse)%sessionSearchCheckEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		messages[i] = foldSearchMessage(&tr.Messages[i])
	}
	if rev != "" {
		s.searchTexts.Put(key, &sessionSearchText{revision: rev, messages: messages})
	}
	return messages, nil
}

// foldSearchMessage folds the text a transcript shows for each part: text,
// reasoning, compaction and notice text, a tool's name, input values and
// inline output, and an attachment's name.
func foldSearchMessage(msg *model.Message) searchMessageText {
	out := searchMessageText{
		hidden: msg.IsMeta || msg.Role == model.RoleSystem,
		parts:  make([]string, len(msg.Parts)),
	}
	for i := range msg.Parts {
		part := &msg.Parts[i]
		var text string
		switch part.Kind {
		case model.PartText, model.PartReasoning, model.PartCompaction, model.PartNotice, model.PartPatch:
			text = part.Text
		case model.PartTool:
			if tool := part.Tool; tool != nil {
				// Plain values are never longer than the JSON holding them.
				text = strings.Join([]string{tool.Name, model.JSONPlainValues(tool.Input, len(tool.Input)), tool.Output}, " ")
			}
		case model.PartFile:
			if part.File != nil {
				text = part.File.Name
			}
		}
		out.parts[i] = foldSearchText(text)
	}
	return out
}

// foldSearchText lowercases text and collapses its whitespace runs, so
// terms match regardless of case and line breaks.
func foldSearchText(s string) string {
	return strings.ToLower(model.OneLine(s))
}
