// Package antigravity reads Antigravity CLI (agy) sessions: the read-only
// conversation_summaries.db index and the brain/<id> transcripts. It never
// reads the credential file, history.jsonl, conversations/*.db or presence/,
// and never writes to the store.
package antigravity

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/pathutil"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

// summariesDB is agy's session index at the store root.
const summariesDB = "conversation_summaries.db"

// Provider reads the Antigravity CLI store, ~/.gemini/antigravity-cli.
type Provider struct {
	root string
	git  *pathutil.GitResolver
	now  func() time.Time // the clock the turn's recency is judged by
}

// New constructs an Antigravity provider for root.
func New(root string, git *pathutil.GitResolver) *Provider {
	if git == nil {
		git = pathutil.NewGitResolver()
	}
	// A relative root is made absolute so source paths always are.
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return &Provider{root: pathutil.NormalizeDir(root), git: git, now: time.Now}
}

// ID returns the model.AgentAntigravity identifier.
func (p *Provider) ID() model.AgentID {
	return model.AgentAntigravity
}

// DisplayName returns the human-readable provider name.
func (p *Provider) DisplayName() string {
	return "Antigravity"
}

func (p *Provider) dbPath() string   { return filepath.Join(p.root, summariesDB) }
func (p *Provider) brainDir() string { return filepath.Join(p.root, "brain") }

// Detect reports whether the store holds a session index or transcripts. It
// only stats files and runs no program.
func (p *Provider) Detect(ctx context.Context) (provider.Detection, error) {
	if err := ctx.Err(); err != nil {
		return provider.Detection{}, err
	}
	det := provider.Detection{Roots: []string{p.root}}
	if info, err := os.Stat(p.dbPath()); err == nil && info.Mode().IsRegular() {
		det.Present = true
	}
	if info, err := os.Stat(p.brainDir()); err == nil && info.IsDir() {
		det.Present = true
	}
	return det, nil
}

// WatchPaths returns the transcript tree and the session index.
func (p *Provider) WatchPaths() []string {
	return []string{p.brainDir(), p.dbPath(), p.dbPath() + "-wal"}
}
