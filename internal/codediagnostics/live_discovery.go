package codediagnostics

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// LiveRegistration is metadata from a concrete adapter, not an execution adapter.
// Available servers join the same exact attachment-wide proposal as saved checkers.
// Resolve assigns one synchronization owner; it never starts a process.
type LiveRegistration struct {
	State      ProviderState
	Server     ServerConfig // legacy single-workspace registration
	Resolve    func(string) (language string, owned bool)
	Workspaces []LiveWorkspace
	Validate   func(path, text string) error // legacy single-workspace validation
}

// LiveWorkspace keeps a server rooted in one contained project directory while
// approval and attachment authority remain with the parent AuthorizedRoot.
type LiveWorkspace struct {
	Server  ServerConfig
	Resolve func(string) (language string, owned bool)
	// Covers is dependency coverage, independent of exclusive document ownership.
	Covers func(string) bool
	// Validate checks saved/displayed build headers without selecting new flags.
	Validate func(path, text string) error
}

const maxLiveWorkspaces = 16

func liveWorkspaces(l *LiveRegistration) []LiveWorkspace {
	if len(l.Workspaces) != 0 {
		return l.Workspaces
	}
	return []LiveWorkspace{{Server: l.Server, Resolve: l.Resolve, Validate: l.Validate}}
}

func liveServers(l *LiveRegistration) []ServerConfig {
	var configs []ServerConfig
	for _, w := range liveWorkspaces(l) {
		configs = append(configs, cloneServer(w.Server))
	}
	return configs
}

func liveTools(l *LiveRegistration) []ToolInput {
	var tools []ToolInput
	for _, cfg := range liveServers(l) {
		tools = append(tools, cfg.Tools()...)
	}
	return tools
}

// LiveEnvironment contains reviewed host-local paths only. Discovery does not
// search parents, private editor caches, install tools, or evaluate project code.
type LiveEnvironment struct {
	Path   []string
	Go     GoplsEnvironment
	TS     TSEnvironment
	Svelte SvelteLSEnvironment
}

// DiscoverLive supplies production adapter metadata without activating a route.
// Invalid host metadata/root fails closed; missing/unsupported tools remain
// independent registrations. Existing adapter walk/hash budgets remain in force.
func DiscoverLive(root AuthorizedRoot, env LiveEnvironment) ([]Registration, error) {
	if len(env.Path) > 64 {
		return nil, failure("limitExceeded", "Reviewed executable search budget exceeded")
	}
	for _, dir := range env.Path {
		if !bounded(dir, 4096) || !filepath.IsAbs(dir) || hasParent(dir) {
			return nil, failure("invalidArgument", "Invalid reviewed executable directory")
		}
	}
	goPlan, err := DiscoverGopls(root, env.Path, env.Go)
	if err != nil {
		return nil, err
	}
	ts, err := DiscoverTypeScript(root, env.Path, env.TS)
	if err != nil {
		return nil, err
	}
	sv, err := DiscoverSvelteLS(root, env.Path, env.Svelte)
	if err != nil {
		return nil, err
	}
	tsConfig, _ := ts.Server()
	return []Registration{
		goPlan.Registration(),
		liveRegistration(ts.Provider(), tsConfig, func(file string) (string, bool) {
			lang, ok := TSLanguageID(file)
			return lang, ok && ts.covers(file)
		}),
		svelteLiveRegistration(sv),
	}, nil
}

func svelteLiveRegistration(sv *SvelteLSPlan) Registration {
	cfg, _ := sv.Server()
	return liveRegistration(sv.Provider(), cfg, func(file string) (string, bool) {
		owner, lang := DocumentOwner(file)
		return lang, owner == svelteLSProviderID && coversPackage(sv.packages, file)
	})
}

// LocalLiveEnvironment is the production local route's reviewed host metadata.
type LocalLiveEnvironment struct {
	GoPath  []string // gopls search, the selected toolchain's bin first
	Path    []string // reviewed executable directories for vtsls, node and ruff
	Go      GoplsEnvironment
	TS      TSEnvironment
	Ruff    RuffEnvironment
	Pyright PyrightEnvironment
	Rust    RustEnvironment
	Svelte  SvelteLSEnvironment // Home defaults to Go.Home
	// Disabled languages are not discovered at all: no tool lookup, no
	// registration.
	Disabled DisabledLanguages
}

// DiscoverLocalLive is the production local live route's discovery: gopls,
// vtsls (CD-23), the Svelte language server (CD-24), Ruff and Pyright for
// Python and rust-analyzer, each with independent availability. Metadata only.
func DiscoverLocalLive(root AuthorizedRoot, env LocalLiveEnvironment) ([]Registration, error) {
	if len(env.GoPath) > 64 || len(env.Path) > 64 {
		return nil, failure("limitExceeded", "Reviewed executable search budget exceeded")
	}
	off := env.Disabled.Disables
	byProvider := map[string]Registration{}
	if !off(goplsProviderID) {
		goPlan, err := DiscoverGopls(root, env.GoPath, env.Go)
		if err != nil {
			return nil, err
		}
		byProvider[goplsProviderID] = goPlan.Registration()
	}
	if !off(rustAnalyzerProviderID) {
		rustEnv := env.Rust
		if rustEnv.Home == "" {
			rustEnv.Home = env.Go.Home
		}
		rust, err := DiscoverRust(root, env.Path, rustEnv)
		if err != nil {
			return nil, err
		}
		// Pinned Rust live servers contribute the same exact composite approval.
		byProvider[rustAnalyzerProviderID] = rustLiveRegistration(rust)
	}
	if !off(svelteLSProviderID) {
		svelteEnv := env.Svelte
		if svelteEnv.Home == "" {
			svelteEnv.Home = env.Go.Home
		}
		// One Svelte server at the root covers every contained Svelte package, in
		// flat and nested workspaces alike.
		sv, err := DiscoverSvelteLS(root, env.Path, svelteEnv)
		if err != nil {
			return nil, err
		}
		byProvider[svelteLSProviderID] = svelteLiveRegistration(sv)
	}
	// TypeScript and Python come from the nested-workspace scan when the project
	// has nested projects, else from one server for the whole root.
	nested, used, err := discoverWorkspaceLive(root, env)
	if err != nil {
		return nil, err
	}
	for _, r := range nested {
		byProvider[r.ProviderID] = r
	}
	if !used {
		for _, p := range workspaceProviders {
			if off(p.id) {
				continue
			}
			r, err := p.register(root, env)
			if err != nil {
				return nil, err
			}
			byProvider[p.id] = r
		}
	}
	var registrations []Registration
	for _, id := range LiveProviderIDs() {
		if r, ok := byProvider[id]; ok {
			registrations = append(registrations, r)
		}
	}
	return registrations, nil
}

func liveRegistration(state ProviderState, cfg ServerConfig, resolve func(string) (string, bool)) Registration {
	return Registration{
		ProviderID: state.ProviderID, Availability: state.Availability,
		Live: &LiveRegistration{State: state, Server: cfg, Resolve: resolve},
	}
}

// DiscoveryDocuments binds all available checker and live identities, not just
// the selected server. Trust deliberately still stores one exact fingerprint per
// project. Adding/removing/changing ANY tool requires renewed composite approval;
// this seam never turns a subset into a grant. Call with the same registrations
// supplied to Manager.Attach/CompleteDiscovery. R05 owns lifecycle wiring.
func DiscoveryDocuments(root AuthorizedRoot, registrations []Registration, sessions *SessionManager) (SessionDocuments, error) {
	if len(registrations) > 32 {
		return SessionDocuments{}, failure("limitExceeded", "Provider budget exceeded")
	}
	var tools []ToolInput
	var live []LiveRegistration
	seen := map[string]bool{}
	for _, r := range registrations {
		if seen[r.ProviderID] || !bounded(r.ProviderID, 256) || !availability(r.Availability) {
			return SessionDocuments{}, failure("invalidArgument", "Invalid provider registration")
		}
		seen[r.ProviderID] = true
		if r.Adapter != nil {
			tools = append(tools, r.Adapter.Tools()...)
		}
		if r.Live != nil {
			if err := validateLive(r); err != nil {
				return SessionDocuments{}, err
			}
			if r.Availability == "available" {
				for _, cfg := range liveServers(r.Live) {
					if _, err := commandDirectory(root, cfg.Directory); err != nil {
						return SessionDocuments{}, err
					}
				}
				tools = append(tools, liveTools(r.Live)...)
				entry := *r.Live
				entry.Workspaces = nil
				entry.Validate = nil
				for _, w := range liveWorkspaces(r.Live) {
					entry.Workspaces = append(entry.Workspaces, LiveWorkspace{Server: cloneServer(w.Server), Resolve: w.Resolve, Covers: w.Covers, Validate: w.Validate})
				}
				live = append(live, entry)
			}
		}
	}
	_, preparationTools := collectPreparations(registrations)
	tools = append(tools, preparationTools...)
	if len(tools) > 0 {
		if _, err := Fingerprint(root, tools); err != nil {
			return SessionDocuments{}, err
		}
	}
	tools = cloneTools(tools)
	resolveWorkspaces := func(current AuthorizedRoot, file string, coverage bool) []DocumentServer {
		if current.Project() != root.Project() {
			return nil
		}
		if ValidatePath(file) != nil {
			return nil
		}
		if !coverage {
			if _, err := current.ResolvePath(current.Project().HostID, file); err != nil {
				return nil
			}
		}
		var result []DocumentServer
		for _, entry := range live {
			var selected *DocumentServer
			depth := -1
			conflict := false
			for _, w := range entry.Workspaces {
				dir := w.Server.Directory
				if dir != "" && !strings.HasPrefix(file, dir+"/") {
					continue
				}
				lang, ok := w.Resolve(file)
				if coverage && w.Covers != nil {
					ok = w.Covers(file)
				}
				if !ok {
					continue
				}
				if coverage {
					cfg := cloneServer(w.Server)
					cfg.ApprovalTools = cloneTools(tools)
					result = append(result, DocumentServer{Config: cfg, LanguageID: lang, Validate: w.Validate})
					continue
				}
				d := 0
				if dir != "" {
					d = strings.Count(dir, "/") + 1
				}
				if d < depth {
					continue
				}
				if d == depth {
					conflict = true
					continue
				}
				cfg := cloneServer(w.Server)
				cfg.ApprovalTools = cloneTools(tools)
				selected = &DocumentServer{Config: cfg, LanguageID: lang, Validate: w.Validate}
				depth, conflict = d, false
			}
			if conflict {
				return nil
			}
			if selected != nil {
				result = append(result, *selected)
			}
		}
		// Only the reviewed Python lint/type pair may share a document. Arbitrary
		// overlapping registrations remain conflicts, never an order-based choice.
		if !coverage && len(result) > 1 {
			if len(result) != 2 || result[0].LanguageID != "python" || result[1].LanguageID != "python" {
				return nil
			}
			a, b := result[0].Config.ProviderID, result[1].Config.ProviderID
			if !((a == "ruff" && b == "pyright") || (a == "pyright" && b == "ruff")) {
				return nil
			}
		}
		return result
	}
	resolveAll := func(current AuthorizedRoot, file string) []DocumentServer {
		return resolveWorkspaces(current, file, false)
	}
	return SessionDocuments{Sessions: sessions, ResolveAll: resolveAll, Cover: func(current AuthorizedRoot, file string) []DocumentServer {
		return resolveWorkspaces(current, file, true)
	}, Resolve: func(current AuthorizedRoot, file string) (DocumentServer, bool) {
		all := resolveAll(current, file)
		if len(all) != 1 {
			return DocumentServer{}, false
		}
		return all[0], true
	}}, nil
}

func cloneServer(cfg ServerConfig) ServerConfig {
	cfg.Args = append([]string(nil), cfg.Args...)
	cfg.Env = append([]string(nil), cfg.Env...)
	cfg.ConfigPaths = append([]string(nil), cfg.ConfigPaths...)
	cfg.Toolchain = cloneTools(cfg.Toolchain)
	cfg.ApprovalTools = cloneTools(cfg.ApprovalTools)
	cfg.Settings = append(json.RawMessage(nil), cfg.Settings...)
	cfg.InitializationOptions = append(json.RawMessage(nil), cfg.InitializationOptions...)
	return cfg
}

func validateLive(r Registration) error {
	l := r.Live
	if r.Adapter != nil || l.State.ProviderID != r.ProviderID || l.State.Availability != r.Availability || l.State.Coverage.Kind != "openedDocuments" {
		return failure("invalidArgument", "Invalid live provider metadata")
	}
	if r.ProviderID == goplsProviderID && len(l.Workspaces) > maxGoContexts+1 {
		return failure("limitExceeded", "Go build context budget exceeded")
	}
	if len(l.Workspaces) > maxLiveWorkspaces {
		return failure("limitExceeded", "Live workspace budget exceeded")
	}
	if len(l.Workspaces) != 0 && (l.Server.ProviderID != "" || l.Resolve != nil || l.Validate != nil) {
		return failure("invalidArgument", "Mixed legacy and workspace live registration")
	}
	if r.Availability == "available" {
		seen := map[string]bool{}
		for _, w := range liveWorkspaces(l) {
			cfg := w.Server
			if cfg.ProviderID != r.ProviderID || w.Resolve == nil || cfg.ApprovalTools != nil || seen[cfg.Directory+"\x00"+cfg.BuildContext] || (cfg.BuildContext != "" && (cfg.ProviderID != goplsProviderID || !validGoContextID(cfg.BuildContext))) {
				return failure("invalidArgument", "Invalid live server identity")
			}
			if cfg.Directory != "" {
				if err := ValidatePath(cfg.Directory); err != nil {
					return err
				}
			}
			seen[cfg.Directory+"\x00"+cfg.BuildContext] = true
		}
	}
	return nil
}
