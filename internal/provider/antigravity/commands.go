package antigravity

import (
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

var (
	_ provider.Provider    = (*Provider)(nil)
	_ provider.ACPLauncher = (*Provider)(nil)
)

// NewSessionCommand starts agy in dir.
func (p *Provider) NewSessionCommand(dir string) provider.Command {
	return provider.Command{Argv: []string{"agy"}, Dir: dir}
}

// ACPAdapter runs agy's built-in ACP server.
func (p *Provider) ACPAdapter() provider.ACPAdapter {
	return provider.ACPAdapter{Executable: "agy", Args: []string{"acp"}}
}

// ResumeCommand reopens session m in its working directory.
func (p *Provider) ResumeCommand(m model.SessionMeta) provider.Command {
	return provider.Command{Argv: []string{"agy", "--conversation", m.Ref.ID}, Dir: m.CWD}
}
