package engine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/ginkcode/agent-sessions/internal/acp"
	"github.com/ginkcode/agent-sessions/internal/launch"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/all"
)

var (
	// ErrChatUnsupported means the agent has no ACP adapter.
	ErrChatUnsupported = errors.New("the agent cannot be used in chat")
	// ErrAdapterNotFound means the agent's ACP adapter is not installed, or
	// its configured path is unusable.
	ErrAdapterNotFound = errors.New("chat adapter not found")
)

// ChatAdapters maps agents to adapter paths that replace the lookup, from
// the config file's [chat.adapters] table:
//
//	[chat.adapters]
//	claude-code = "/opt/acp/bin/claude-agent-acp"
//	codex = "~/.npm-global/bin/codex-acp"
//	opencode = "/usr/local/bin/opencode"
type ChatAdapters map[model.AgentID]string

// LoadChatAdapters reads the adapter overrides from config.toml in dir. A
// missing file or table means none.
func LoadChatAdapters(dir string) (ChatAdapters, error) {
	if dir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeChatAdapters(data)
}

func decodeChatAdapters(data []byte) (ChatAdapters, error) {
	var cfg struct {
		Chat struct {
			Adapters map[string]string `toml:"adapters"`
		} `toml:"chat"`
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("chat: parse config: %w", err)
	}
	out := ChatAdapters{}
	for id, path := range cfg.Chat.Adapters {
		if !slices.Contains(all.IDs(), id) {
			return nil, fmt.Errorf("chat: [chat.adapters] names an unknown agent %q", id)
		}
		if path = strings.TrimSpace(path); path != "" {
			out[model.AgentID(id)] = path
		}
	}
	return out, nil
}

// ChatAgent is an agent's ACP adapter as a chat on this machine would start
// it.
type ChatAgent struct {
	Agent     model.AgentID `json:"agent"`
	Name      string        `json:"name"`
	Available bool          `json:"available"`
	// Path is the adapter that would run.
	Path string `json:"path,omitempty"`
	// Override is true when the path comes from the config file.
	Override bool `json:"override,omitempty"`
	// Install is the command that installs the adapter.
	Install string `json:"install"`
	// Problem says why the adapter is not available.
	Problem string `json:"problem,omitempty"`
}

// ChatAgents reports, for each provider with an ACP adapter, whether the
// adapter is installed, found the way ChatLaunch finds it. Nothing is run.
func (s *Service) ChatAgents(overrides ChatAdapters) []ChatAgent {
	out := []ChatAgent{}
	for _, prov := range s.providers {
		l, ok := prov.(provider.ACPLauncher)
		if !ok {
			continue
		}
		a := l.ACPAdapter()
		st := ChatAgent{Agent: prov.ID(), Name: prov.DisplayName(), Install: a.Install, Override: overrides[prov.ID()] != ""}
		if exe, err := findAdapter(a, overrides[prov.ID()]); err != nil {
			st.Problem = err.Error()
		} else {
			st.Available, st.Path = true, exe.Path
		}
		out = append(out, st)
	}
	return out
}

// ChatLaunch returns the command that starts agent's ACP adapter in dir.
// Like NewSessionLaunch, it only starts in directories the catalog knows.
func (s *Service) ChatLaunch(agent model.AgentID, dir string, overrides ChatAdapters) (acp.Command, error) {
	prov, ok := s.providers.Get(agent)
	if !ok {
		return acp.Command{}, fmt.Errorf("%w: %s", ErrUnknownProvider, agent)
	}
	l, ok := prov.(provider.ACPLauncher)
	if !ok {
		return acp.Command{}, fmt.Errorf("%w: %s", ErrChatUnsupported, prov.DisplayName())
	}
	if err := s.checkLaunchDir(dir); err != nil {
		return acp.Command{}, err
	}
	a := l.ACPAdapter()
	exe, err := findAdapter(a, overrides[agent])
	if err != nil {
		return acp.Command{}, fmt.Errorf("%w: %s. Install it with: %s", ErrAdapterNotFound, err.Error(), a.Install)
	}
	cmd := acp.Command{Argv: append([]string{exe.Path}, a.Args...), Dir: dir}

	// An app started from a desktop menu may lack the PATH entries that hold
	// the adapter and its interpreter (an npm package run by node, which nvm
	// keeps beside it), so their directories go first.
	var dirs []string
	if !exe.OnPath {
		dirs = append(dirs, filepath.Dir(exe.Path))
	}
	if a.CLIEnv != "" && os.Getenv(a.CLIEnv) == "" {
		if argv := prov.NewSessionCommand(dir).Argv; len(argv) > 0 {
			if cli, err := findAgent(argv[0]); err == nil {
				cmd.Env = append(cmd.Env, a.CLIEnv+"="+cli.Path)
				if !cli.OnPath && !slices.Contains(dirs, filepath.Dir(cli.Path)) {
					dirs = append(dirs, filepath.Dir(cli.Path))
				}
			}
		}
	}
	if len(dirs) > 0 {
		if p := os.Getenv("PATH"); p != "" {
			dirs = append(dirs, p)
		}
		cmd.Env = append(cmd.Env, "PATH="+strings.Join(dirs, string(os.PathListSeparator)))
	}
	return cmd, nil
}

// findAdapter returns the configured adapter when there is one, else looks
// it up like the agent CLIs.
func findAdapter(a provider.ACPAdapter, override string) (launch.Executable, error) {
	if override == "" {
		exe, err := findAgent(a.Executable)
		if err != nil {
			hint := strings.TrimPrefix(err.Error(), launch.ErrAgentNotFound.Error()+": ")
			if !strings.Contains(hint, "Windows program") {
				hint = a.Executable + " was not found on PATH or in its usual install locations"
			}
			return launch.Executable{}, errors.New(hint)
		}
		return launch.Executable{Path: exe.Path, OnPath: exe.OnPath}, nil
	}
	path := override
	if rest, ok := strings.CutPrefix(path, "~"); ok && (rest == "" || rest[0] == '/' || rest[0] == filepath.Separator) {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, rest)
		}
	}
	if !filepath.IsAbs(path) {
		return launch.Executable{}, fmt.Errorf("the configured adapter path %q is not absolute", override)
	}
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return launch.Executable{}, fmt.Errorf("the configured adapter %s does not exist", path)
	case !info.Mode().IsRegular():
		return launch.Executable{}, fmt.Errorf("the configured adapter %s is not a file", path)
	case runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0:
		return launch.Executable{}, fmt.Errorf("the configured adapter %s is not executable", path)
	}
	return launch.Executable{Path: path}, nil
}
