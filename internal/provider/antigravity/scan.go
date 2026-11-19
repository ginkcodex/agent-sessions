package antigravity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"github.com/ginkcode/agent-sessions/internal/jsonl"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/pathutil"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

const checkpointVersion = 1

// liveWindow is how long after the last write a session the index still
// calls running may count as working.
// shortcut: a tool or approval silent for longer reads as done, until agy
// records a reliable liveness signal (its presence/ locks are not read).
const liveWindow = 10 * time.Minute

// checkpoint is the running aggregate of one transcript, and what Scan last
// published for it.
type checkpoint struct {
	Version     int                 `json:"version"`
	ID          string              `json:"id"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
	Counts      model.MessageCounts `json:"counts"`
	Tokens      model.TokenUsage    `json:"tokens"`
	Model       string              `json:"model,omitempty"`
	FirstPrompt string              `json:"firstPrompt,omitempty"`
	UserSeen    bool                `json:"userSeen,omitempty"`
	// Turn is where the transcript alone leaves the turn; ToolsUntil is the
	// step index of the last result the pending tool calls expect.
	Turn       string `json:"turn,omitempty"`
	ToolsUntil int64  `json:"toolsUntil,omitempty"`
	LinesRead  int    `json:"linesRead,omitempty"`
	// Row and Published are the index row and turn of the last published meta.
	Row       *row   `json:"row,omitempty"`
	Published string `json:"published,omitempty"`

	path   string
	lineNo int
}

func (c *checkpoint) consume(line []byte, d *provider.Diagnostics) {
	if !gjson.ValidBytes(line) {
		d.ParseErrors++
		d.Warn(c.path, c.lineNo, "invalid JSON record")
		return
	}
	if stamp := gjson.GetBytes(line, "created_at").String(); stamp != "" {
		if ts, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
			ts = ts.UTC()
			if c.CreatedAt.IsZero() || ts.Before(c.CreatedAt) {
				c.CreatedAt = ts
			}
			if ts.After(c.UpdatedAt) {
				c.UpdatedAt = ts
			}
		} else {
			d.Warn(c.path, c.lineNo, "invalid created_at %q", stamp)
		}
	}
	index := gjson.GetBytes(line, "step_index").Int()
	switch kind := gjson.GetBytes(line, "type").String(); kind {
	case stepUserInput:
		c.Counts.User++
		c.UserSeen = true
		if c.FirstPrompt == "" {
			c.FirstPrompt = prompt(userRequest(gjson.GetBytes(line, "content").String()))
		}
		c.Turn = model.TurnWorking
	case stepPlanner:
		c.Counts.Assistant++
		r := gjson.GetManyBytes(line, "model", "input_tokens", "output_tokens", "cache_read_tokens", "tool_calls.#")
		if r[0].String() != "" {
			c.Model = r[0].String()
		}
		c.Tokens = c.Tokens.Add(model.TokenUsage{Input: r[1].Int(), Output: r[2].Int(), CacheRead: r[3].Int()})
		calls := r[4].Int()
		c.Counts.ToolCalls += int(calls)
		if calls > 0 {
			// The result of call k is step index+1+k.
			c.Turn, c.ToolsUntil = model.TurnTool, index+calls
		} else {
			c.Turn = model.TurnDone
		}
	case stepGeneric:
		// A denied call may write no result, so a later step counts too.
		if c.Turn == model.TurnTool && index >= c.ToolsUntil {
			c.Turn = model.TurnWorking
		}
	case stepSystemMessage:
		// A subagent's transcript opens with its task from the parent, and
		// has no USER_INPUT: that task is its prompt.
		if !c.UserSeen && c.FirstPrompt == "" {
			if msg, ok := parseSystemMessage(gjson.GetBytes(line, "content").String()); ok && msg.fromSession() {
				c.Counts.User++
				c.FirstPrompt = prompt(msg.Text)
			}
		}
		c.Turn = model.TurnWorking
	case stepCheckpoint:
		c.Turn = model.TurnWorking
	case stepErrorMessage:
		c.Turn = model.TurnDone
	default:
		d.Unknown(kind)
	}
}

func prompt(text string) string { return model.TruncateRunes(model.OneLine(text), 300) }

// turn gates the transcript's turn with the index row: the row alone is not
// reliable (left RUNNING after a clean exit, or CANCELING long after the
// process died), and neither is a transcript whose process was killed.
func (p *Provider) turn(c *checkpoint, r *row, modTime time.Time) string {
	if c.Turn == "" || c.Turn == model.TurnDone {
		return c.Turn
	}
	last := modTime
	if r != nil {
		switch strings.TrimPrefix(r.Status, "CASCADE_RUN_STATUS_") {
		case "RUNNING", "CANCELING":
		default:
			return model.TurnDone
		}
		if !r.NotFullyIdle || r.Killed {
			return model.TurnDone
		}
		if r.Modified.After(last) {
			last = r.Modified
		}
	}
	if p.now().Sub(last) > liveWindow {
		return model.TurnDone
	}
	return c.Turn
}

func (c *checkpoint) meta(src source, r *row, modTime time.Time) model.SessionMeta {
	meta := model.SessionMeta{
		Ref:         model.SessionRef{Agent: model.AgentAntigravity, ID: src.ID},
		SourcePath:  src.Path,
		FirstPrompt: c.FirstPrompt,
		Model:       c.Model,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
		Counts:      c.Counts,
		Tokens:      c.Tokens,
		Turn:        c.Published,
	}
	if meta.UpdatedAt.IsZero() {
		meta.UpdatedAt = modTime.UTC()
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = meta.UpdatedAt
	}
	var title string
	if r != nil {
		meta.ParentID = r.ParentID
		meta.AgentName = r.AgentName
		meta.CWD = pathutil.NormalizeDir(r.CWD)
		title = r.Title
		if title == "" {
			title = prompt(r.Preview)
		}
	}
	switch {
	case title != "":
		meta.Title = title
	case c.FirstPrompt != "":
		meta.Title = c.FirstPrompt
	default:
		meta.Title = "(untitled)"
	}
	return meta
}

type sourceScan struct {
	state   provider.SourceState
	meta    model.SessionMeta
	diag    provider.Diagnostics
	changed bool
	err     error
}

// Scan reads the session index and the transcripts, returning sessions whose
// transcript, index row or turn changed since prev. Only appended transcript
// bytes are parsed. Without a readable index, transcripts alone are used.
func (p *Provider) Scan(ctx context.Context, prev provider.ScanState) (provider.ScanResult, error) {
	var diag provider.Diagnostics
	sources, err := p.discover(ctx, &diag)
	if err != nil {
		return provider.ScanResult{}, err
	}
	rows, err := p.readSummaries(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return provider.ScanResult{}, ctx.Err()
		}
		diag.Warn(p.dbPath(), 0, "Antigravity session index unavailable: %v; using transcripts and the last rows read", err)
	}
	result := provider.ScanResult{State: provider.ScanState{Sources: make(map[string]provider.SourceState, len(sources))}}
	present := make(map[string]bool, len(sources))
	ids := make(map[string]bool, len(sources))
	for _, src := range sources {
		present[src.Path], ids[src.ID] = true, true
		old, hasOld := prev.Sources[src.Path]
		out := p.scanSource(ctx, src, old, hasOld, rows)
		if out.err != nil {
			if ctx.Err() != nil {
				return provider.ScanResult{}, ctx.Err()
			}
			out.diag.Warn(src.Path, 0, "scan transcript: %v", out.err)
		}
		diag.Merge(out.diag)
		if out.changed {
			result.Changed = append(result.Changed, out.meta)
			result.State.Sources[src.Path] = out.state
		} else if hasOld {
			result.State.Sources[src.Path] = old
		}
	}
	for path, old := range prev.Sources {
		if present[path] {
			continue
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			result.State.Sources[path] = old
			diag.Warn(path, 0, "previous transcript unavailable; retaining previous scan state")
			continue
		}
		// A session whose transcript moved to its other name is not removed.
		if id := sessionID(path); !ids[id] {
			result.Removed = append(result.Removed, model.SessionRef{Agent: model.AgentAntigravity, ID: id})
		}
	}
	sort.Slice(result.Removed, func(i, j int) bool { return result.Removed[i].Key() < result.Removed[j].Key() })
	result.Diag = diag
	return result, ctx.Err()
}

// scanSource parses what was appended to one transcript since old. rows is
// nil when the index is unreadable; the checkpoint's last row is used then.
func (p *Provider) scanSource(ctx context.Context, src source, old provider.SourceState, hasOld bool, rows map[string]row) sourceScan {
	var out sourceScan
	info, err := os.Stat(src.Path)
	if err != nil {
		out.diag.Warn(src.Path, 0, "stat transcript: %v", err)
		return out
	}
	size, modNs := info.Size(), info.ModTime().UnixNano()

	var cp checkpoint
	resume := hasOld && len(old.Checkpoint) != 0 && old.Offset >= 0 && old.Size <= size && old.Offset <= size
	if resume {
		if err := json.Unmarshal(old.Checkpoint, &cp); err != nil {
			out.diag.Warn(src.Path, 0, "invalid scan checkpoint: %v", err)
			resume = false
		} else if cp.Version != checkpointVersion || cp.ID != src.ID {
			resume = false
		}
	}
	unchanged := resume && old.Size == size && old.ModTimeNs == modNs
	// A same-size rewrite is not an append: rebuild.
	if resume && !unchanged && old.Size == size {
		resume = false
	}
	if !resume {
		cp = checkpoint{Version: checkpointVersion, ID: src.ID}
	}
	cp.path = src.Path

	end := old.Offset
	if !unchanged {
		if !resume {
			end = 0
		}
		if end, err = cp.read(ctx, end, &out.diag); err != nil {
			out.err = err
			return out
		}
	}

	if !p.settle(&cp, rows, info.ModTime()) && unchanged {
		return out
	}
	out.meta = p.sessionMeta(&cp, src, info.ModTime())
	data, err := json.Marshal(cp)
	if err != nil {
		out.err = fmt.Errorf("marshal scan checkpoint %s: %w", src.Path, err)
		return out
	}
	out.changed = true
	out.state = provider.SourceState{Size: size, ModTimeNs: modNs, Offset: end, Checkpoint: data}
	return out
}

// settle takes the session's index row (the last one read when rows is nil,
// the index being unreadable) and gates the turn with it. It reports whether
// either changed since the last publication.
func (p *Provider) settle(c *checkpoint, rows map[string]row, modTime time.Time) bool {
	r := c.Row
	if rows != nil {
		r = nil
		if found, ok := rows[c.ID]; ok {
			r = &found
		}
	}
	turn := p.turn(c, r, modTime)
	changed := !sameRow(c.Row, r) || turn != c.Published
	c.Row, c.Published = r, turn
	return changed
}

// sessionMeta is the published meta of a settled checkpoint.
func (p *Provider) sessionMeta(c *checkpoint, src source, modTime time.Time) model.SessionMeta {
	meta := c.meta(src, c.Row, modTime)
	if meta.CWD != "" {
		meta.CWDMissing = !pathutil.Exists(meta.CWD)
		if repo, ok := p.git.Resolve(meta.CWD); ok {
			meta.RepoRoot = repo.MainRoot
		}
	}
	return meta
}

// read consumes the transcript from offset and returns the offset of the
// first byte not yet consumed.
func (c *checkpoint) read(ctx context.Context, offset int64, d *provider.Diagnostics) (int64, error) {
	r, err := jsonl.Open(c.path, offset)
	if err != nil {
		return offset, fmt.Errorf("open transcript: %w", err)
	}
	defer func() { _ = r.Close() }()
	base := c.LinesRead
	for {
		if err := ctx.Err(); err != nil {
			return offset, err
		}
		line, _, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		c.lineNo = base + r.LineNo()
		if errors.Is(err, jsonl.ErrLineTooLong) {
			d.ParseErrors++
			d.Warn(c.path, c.lineNo, "JSONL record exceeds maximum line length")
			continue
		}
		if err != nil {
			return offset, fmt.Errorf("read transcript %s: %w", c.path, err)
		}
		c.consume(line, d)
	}
	c.LinesRead = base + r.LineNo()
	return r.Offset(), nil
}
