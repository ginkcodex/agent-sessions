package antigravity

import (
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

// NewSessionCommand starts agy in dir.
func (p *Provider) NewSessionCommand(dir string) provider.Command {
	return provider.Command{Argv: []string{"agy"}, Dir: dir}
}

// ResumeCommand reopens session m in its working directory.
func (p *Provider) ResumeCommand(m model.SessionMeta) provider.Command {
	return provider.Command{Argv: []string{"agy", "--conversation", m.Ref.ID}, Dir: m.CWD}
}
