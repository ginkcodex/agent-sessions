package codediagnostics

// Safe LSP session lifecycle (CD-19).
//
// A session owns one host-local stdio server: trust gate, initialize/initialized,
// server-request policy, idle reuse and shutdown. It implements no language
// adapter, document sync, file watching or pull diagnostics, and advertises no
// live client capability: only the UTF-16 position encoding is declared.
//
// Trust is not a sandbox. An approved server and its plugins can still read
// dependencies or use the network; this file only bounds the app's own protocol
// surface. Document bodies and raw frames are never retained or logged here;
// only a failed session's bounded stderr reaches the owner's FailureLog.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Proposed limits (provisional until CD-01 finalizes them).
const (
	maxActiveProjects = 2
	idleGrace         = 2 * time.Minute
	maxRestarts       = 3

	initTimeout     = 10 * time.Second
	shutdownTimeout = 5 * time.Second
	cleanupTimeout  = 5 * time.Second
	// A failed server's exit is reaped and its stderr drained for up to this
	// long (real time) before the pipes close, so the crash log keeps its exit
	// status and the end of its output.
	crashDrainTimeout = 500 * time.Millisecond
	maxInitBytes      = 256 << 10
	maxConfigItems    = 16
	maxConfigSect     = 256
	maxServerParams   = 8 << 10
	maxScopeURI       = 4096
	maxWatchRejects   = 1 << 16

	clientName    = "agent-sessions"
	clientVersion = "stage2"
)

// Session phases. stopped, failed and disconnected are terminal for one
// generation; a restart builds a new session instead of recycling a dead one.
const (
	phaseStarting     = "starting"
	phaseInitialized  = "initialized"
	phaseIdle         = "idle"
	phaseFailed       = "failed"
	phaseDisconnected = "disconnected"
	phaseStopping     = "stopping"
	phaseStopped      = "stopped"
)

// Session errors.
var (
	ErrSessionNotRetryable = errors.New("lsp: session cannot be retried")
	ErrSessionRestart      = errors.New("lsp: restart attempts exhausted")
)

// CoverageExternalChanges is the honest file-watch answer: this lifecycle rejects
// every watcher registration, so unopened-file coverage is "unwatched" (stale
// fallback, CD-25) and never a continuous-sync claim.
const CoverageExternalChanges = "unwatched"

// ServerConfig is the fixed, adapter-owned identity of one installed server.
// Executable and argv are used exactly. Nothing here is client-decoded.
type ServerConfig struct {
	ProviderID    string
	Executable    string // absolute, resolved on the project host
	Args          []string
	Env           []string // entire environment, see Command
	Directory     string   // relative to the authorized root; empty is the root
	AdapterPolicy string
	ConfigPaths   []string // relative config/lock identities bound into approval
	// Toolchain binds additional host files the server executes or loads (for
	// gopls: the reviewed GOROOT's bin/go and VERSION) into the same proposal.
	Toolchain []ToolInput
	// ApprovalTools is the complete attachment proposal, not a subset grant.
	// Nil preserves the standalone single-server seam used by adapter tests.
	ApprovalTools []ToolInput
	// Settings is the adapter-owned, fixed answer to workspace/configuration (a
	// JSON object). Nil advertises no configuration capability and answers every
	// item null. It is bound into the policy, so changing it requires approval.
	Settings json.RawMessage
	// InitializationOptions is fixed adapter-owned JSON sent in initialize, before
	// the server can load a workspace. Like Settings, it is cloned and policy-bound.
	InitializationOptions json.RawMessage
	// ValidateStart re-enumerates adapter inputs at launch and publication/status
	// boundaries. Its behavior must be identified by AdapterPolicy; the function
	// itself is backend-owned, never serialized or treated as approval authority.
	ValidateStart func() error `json:"-"`
	// DeclareDiagnostics advertises textDocument.publishDiagnostics. vtsls
	// publishes only to a client that declares it; gopls pushes regardless and
	// keeps its unchanged initialize. Bound into the policy like Settings.
	DeclareDiagnostics bool
	// AcknowledgeWatchers advertises dynamic didChangeWatchedFiles registration
	// and acknowledges it, without delivering events: external changes stay
	// unwatched. svelte-language-server otherwise watches its workspace itself,
	// follows symlinks out of the project (a DMG staging link to /Applications),
	// and exits when macOS refuses the watch handles. Bound into the policy.
	AcknowledgeWatchers bool
	// BuildContext names an automatically inferred, immutable build context of
	// the same semantic server (for gopls: the canonical, comma-joined custom
	// build tags). The flags themselves reach the server through Settings and the
	// policy; this only keeps same-directory contexts apart in approval IDs.
	BuildContext string
}

func (c ServerConfig) command() Command {
	policy := c.AdapterPolicy
	if c.DeclareDiagnostics {
		policy += "|client:publish-diagnostics"
	}
	if c.AcknowledgeWatchers {
		policy += "|client:acknowledge-watchers"
	}
	if c.Settings != nil {
		policy += "|settings:" + string(c.Settings)
	}
	if c.InitializationOptions != nil {
		policy += "|init-options:" + string(c.InitializationOptions)
	}
	if c.ValidateStart != nil {
		// The callback is not serialized; its contract must already be fixed in
		// AdapterPolicy. This marker makes a config that enables one distinguishable.
		policy += "|validate-start"
	}
	return Command{ProviderID: c.scopedToolID(c.ProviderID), Executable: c.Executable, Args: c.Args, Env: c.Env, Directory: c.Directory, AdapterPolicy: policy, AcceptedExits: []int{0}, ValidateStart: c.ValidateStart}
}

// Policy is the ToolInput.Policy that approval must bind (CD-02 fingerprint).
func (c ServerConfig) Policy() string { return c.command().Policy() }

// Tool is the approval input for this server.
func (c ServerConfig) Tool() ToolInput {
	return ToolInput{ProviderID: c.scopedToolID(c.ProviderID), Executable: c.Executable, ConfigPaths: append([]string(nil), c.ConfigPaths...), Policy: c.Policy(), BuildContext: c.BuildContext}
}

// Tools is the server's complete approval contribution: its own identity plus
// any bound toolchain files.
func (c ServerConfig) Tools() []ToolInput {
	tools := cloneTools(c.Toolchain)
	for i := range tools {
		tools[i].ProviderID = c.scopedToolID(tools[i].ProviderID)
	}
	return append([]ToolInput{c.Tool()}, tools...)
}

// Keep semantic provider identities separate from exact proposal identities.
// Hash both components so even a 256-byte toolchain ID stays bounded.
func (c ServerConfig) scopedToolID(id string) string {
	if c.Directory == "" && c.BuildContext == "" {
		return id
	}
	scope := c.Directory
	if c.BuildContext != "" {
		scope += "\x00context:" + c.BuildContext
	}
	workspace := sha256.Sum256([]byte(scope))
	tool := sha256.Sum256([]byte(id))
	scoped := "workspace:" + hex.EncodeToString(workspace[:]) + ":" + hex.EncodeToString(tool[:])
	if c.ProviderID == rustAnalyzerProviderID {
		return rustAnalyzerProviderID + "-" + scoped
	}
	return scoped
}

func (c ServerConfig) approvalTools() []ToolInput {
	if c.ApprovalTools != nil {
		return cloneTools(c.ApprovalTools)
	}
	return c.Tools()
}

// ServerCapabilities is the negotiated subset recorded for document operations.
type ServerCapabilities struct {
	OpenClose       bool
	Change          int  // 1 full, 2 incremental
	Save            bool // didSave notifications wanted; text is never included
	PullAdvertised  bool // recorded only; pull diagnostics stay unsupported
	PositionEncoded string
	Definition      bool // textDocument/definition answered
	References      bool // textDocument/references answered
	Hover           bool // textDocument/hover answered
}

func (c ServerCapabilities) FullSync() bool        { return c.Change == 1 }
func (c ServerCapabilities) IncrementalSync() bool { return c.Change == 2 }

// SessionInfo is a bounded snapshot with no document text or raw frames.
type SessionInfo struct {
	Generation       string
	Phase            string
	Root             Project
	ProviderID       string
	Capabilities     ServerCapabilities
	Restarts         int
	RetryAvailable   bool
	ExternalChanges  string // always CoverageExternalChanges
	RejectedWatchers int
	Notice           string
	Err              error
}

// SessionStreams are the stdio ends of one launched server.
type SessionStreams struct {
	Stdin          io.WriteCloser
	Stdout, Stderr io.Reader
}

// SessionProcess is one launched server. Terminate must kill the whole process
// tree; Close releases handles. Wait must return after Terminate and Close.
type SessionProcess interface {
	Wait() (int, error)
	Terminate() error
	Close() error
}

// SessionLaunch is the only execution seam. It is called only after the
// known-directory and approval gates pass.
type SessionLaunch func(Invocation) (SessionProcess, SessionStreams, error)

// SessionOptions configures a manager. Zero values take the proposed defaults;
// limits can only be narrowed. After supplies every deadline and grace timer.
type SessionOptions struct {
	MaxActive       int
	IdleGrace       time.Duration
	MaxRestarts     int
	InitTimeout     time.Duration
	ShutdownTimeout time.Duration
	CleanupTimeout  time.Duration
	LSP             LSPLimits
	After           func(time.Duration) <-chan time.Time
	Launch          SessionLaunch
	// Notifications receives publishDiagnostics (only) from the current
	// generation, synchronously on the session's serve goroutine. It must be
	// quick and must not call back into SessionManager. Nil drops them.
	Notifications func(SessionNotification)
	// FailureLog receives one report per failed session (not idle expiry,
	// revocation or Close), after its process is gone. It runs on the stopping
	// goroutine, must be quick and must not call back into SessionManager.
	FailureLog func(SessionFailure)
}

// SessionFailure is the local crash report of one failed session. It carries
// sanitized, bounded server stderr, so it is for the owner's on-disk log only
// and must never reach a snapshot, event or the UI.
type SessionFailure struct {
	Project         Project
	ProviderID      string
	Generation      string
	Executable      string
	Args            []string
	Directory       string // relative to the project root; empty is the root
	Started, Failed time.Time
	Phase           string // before the failure: starting, initialized or idle
	Restarts        int
	Cause           error // the bounded session failure
	Transport       error // raw transport failure; nil when nothing was launched
	// Exit is how the process ended ("exit status 1", "signal: segmentation
	// fault"); empty if never reaped. A server still running at the failure is
	// killed by the app and reads "signal: killed".
	Exit               string
	Stderr, StderrTail string // see LSPStatus
	StderrTruncated    bool
	StderrBytes        int64
}

// SessionNotification is one bounded server publication. Params is the raw,
// transport-bounded JSON; it is never logged or retained by the session.
type SessionNotification struct {
	Project    Project
	ProviderID string
	Generation string
	Method     string
	Params     json.RawMessage
}

func (o SessionOptions) defaults() SessionOptions {
	narrow := func(v, max int) int {
		if v <= 0 || v > max {
			return max
		}
		return v
	}
	dur := func(v, max time.Duration) time.Duration {
		if v <= 0 || v > max {
			return max
		}
		return v
	}
	o.MaxActive = narrow(o.MaxActive, maxActiveProjects)
	o.MaxRestarts = narrow(o.MaxRestarts, maxRestarts)
	o.IdleGrace = dur(o.IdleGrace, idleGrace)
	o.InitTimeout = dur(o.InitTimeout, initTimeout)
	o.ShutdownTimeout = dur(o.ShutdownTimeout, shutdownTimeout)
	o.CleanupTimeout = dur(o.CleanupTimeout, cleanupTimeout)
	if o.After == nil {
		o.After = time.After
	}
	return o
}

// sessionKey is the reuse identity: canonical authorized root (host-scoped) plus
// the exact server configuration. Different args/env/policy never share a server.
type sessionKey struct {
	project  Project
	provider string
	config   string
}

type sessionEntry struct {
	key       sessionKey
	restarted int
	session   *lspSession
}

// SessionManager owns host-local LSP sessions for one connection/app lifetime.
// Authorization and approval come from CD-02 (AuthorizedRoot, Trust); it never
// grants either. Close must run on disconnect/shutdown and must not be reused.
type SessionManager struct {
	trust   *Trust
	options SessionOptions

	mu      sync.Mutex
	entries map[sessionKey]*sessionEntry
	closed  bool
	wg      sync.WaitGroup
	idleSeq atomic.Uint64 // idle ordering for eviction; no wall clock

	owned ownedCounters // test-visible accounting of goroutines this manager started
}

// ownedCounters counts only resources this SessionManager started; it is not
// process matching. Each counter returns to zero once its owner has finished.
type ownedCounters struct {
	runs           atomic.Int64 // session run loops
	processWaiters atomic.Int64 // goroutines blocked in SessionProcess.Wait
	timerWaiters   atomic.Int64 // goroutines blocked on an injected After timer
}

// OwnedResources is a bounded snapshot of live goroutines owned by one manager.
type OwnedResources struct{ Runs, ProcessWaiters, TimerWaiters int64 }

// OwnedResources reports goroutines still owned by this manager.
func (m *SessionManager) OwnedResources() OwnedResources {
	return OwnedResources{m.owned.runs.Load(), m.owned.processWaiters.Load(), m.owned.timerWaiters.Load()}
}

// waitProcess starts the single process waiter for s.
func (s *lspSession) waitProcess(proc SessionProcess) {
	s.manager.owned.processWaiters.Add(1)
	go func() {
		defer s.manager.owned.processWaiters.Add(-1)
		code, err := proc.Wait()
		status := fmt.Sprintf("exit status %d", code)
		if d, ok := proc.(interface{ ExitStatus() string }); ok && d.ExitStatus() != "" {
			status = d.ExitStatus()
		} else if err != nil {
			status = "wait failed: " + err.Error()
		}
		s.mu.Lock()
		s.exitStatus = status
		s.mu.Unlock()
		close(s.exited)
	}()
}

func NewSessionManager(trust *Trust, options SessionOptions) *SessionManager {
	return &SessionManager{trust: trust, options: options.defaults(), entries: map[sessionKey]*sessionEntry{}}
}

func keyOf(root AuthorizedRoot, cfg ServerConfig) sessionKey {
	return sessionKey{root.Project(), cfg.ProviderID, cfg.Policy()}
}

func live(phase string) bool {
	return phase == phaseStarting || phase == phaseInitialized || phase == phaseIdle
}

// Attach returns the single reusable initialized session for this root and
// server, starting it if needed. Concurrent attaches share one start. A session
// inside its idle grace is resumed and its timer canceled. A failed session is
// reported as failed until an explicit, capped Retry.
func (m *SessionManager) Attach(ctx context.Context, root AuthorizedRoot, cfg ServerConfig) (SessionInfo, error) {
	cfg = cloneServer(cfg)
	tool := cfg.Tool()
	if m.trust == nil || m.options.Launch == nil {
		return SessionInfo{}, failure("invalidArgument", "Language server owner is not configured")
	}
	if err := m.gate(root, cfg); err != nil {
		return SessionInfo{}, err
	}
	// Preflight rechecks approval and rehashes executable/config identity.
	lease, err := m.trust.Preflight(root, cfg.approvalTools())
	if err != nil {
		return SessionInfo{}, err
	}
	return m.attach(ctx, root, cfg, tool, lease, false)
}

func (m *SessionManager) attach(ctx context.Context, root AuthorizedRoot, cfg ServerConfig, tool ToolInput, lease *Lease, restart bool) (SessionInfo, error) {
	key := keyOf(root, cfg)
	for {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return SessionInfo{}, failure("disconnected", "Language server owner is closed")
		}
		e := m.entries[key]
		if e != nil && e.session != nil {
			s := e.session
			switch phase := s.phaseNow(); {
			case live(phase) && s.current():
				s.resume()
				m.mu.Unlock()
				return s.wait(ctx)
			case live(phase) || phase == phaseStopping:
				// Revoked/stale or shutting down: finish it before starting anew.
				m.mu.Unlock()
				s.stop(phaseStopped, nil, false)
				if err := waitDone(ctx, s.disposed); err != nil {
					return SessionInfo{}, err
				}
				continue
			case phase == phaseFailed && !restart:
				info := s.info()
				m.mu.Unlock()
				return info, info.Err
			}
		}
		if cfg.ProviderID == goplsProviderID {
			active := 0
			for other, entry := range m.entries {
				if other != key && other.project == key.project && other.provider == goplsProviderID && entry.session != nil {
					phase := entry.session.phaseNow()
					if live(phase) || phase == phaseStopping {
						active++
					}
				}
			}
			if active >= maxGoContexts+1 {
				m.mu.Unlock()
				return SessionInfo{}, failure("limitExceeded", "Go build context budget exceeded")
			}
		}
		if victim := m.capacityVictimLocked(key.project); victim != nil || m.atCapacityLocked(key.project) {
			m.mu.Unlock()
			if victim == nil {
				return SessionInfo{}, failure("limitExceeded", "Too many active language server projects")
			}
			victim.stop(phaseStopped, nil, true)
			if err := waitDone(ctx, victim.disposed); err != nil {
				return SessionInfo{}, err
			}
			continue
		}
		if e == nil {
			e = &sessionEntry{key: key}
			m.entries[key] = e
		} else if !restart {
			e.restarted = 0 // a deliberate new start, not a crash retry
		}
		s := newLSPSession(m, e, root, cfg, tool, lease)
		e.session = s
		m.wg.Add(1)
		m.owned.runs.Add(1)
		m.mu.Unlock()
		go s.run()
		return s.wait(ctx)
	}
}

// gate enforces the fixed executable/argv contract before anything can start.
func (m *SessionManager) gate(root AuthorizedRoot, cfg ServerConfig) error {
	if err := root.Revalidate(); err != nil { // known-directory authorization
		return err
	}
	if len(cfg.ConfigPaths) > 128 {
		return failure("invalidArgument", "Too many configuration identities")
	}
	if cfg.InitializationOptions != nil && (len(cfg.InitializationOptions) > maxInitBytes || !json.Valid(cfg.InitializationOptions)) {
		return failure("invalidArgument", "Invalid fixed initialization options")
	}
	selected := cfg.Tool()
	member := false
	for _, tool := range cfg.approvalTools() {
		if tool.ProviderID == selected.ProviderID && tool.Executable == selected.Executable && tool.Policy == selected.Policy && slices.Equal(tool.ConfigPaths, selected.ConfigPaths) {
			member = true
		}
	}
	if !member {
		return failure("invalidArgument", "Server is not in the complete approval proposal")
	}
	// Validate membership in the whole proposal; never authorize arbitrary argv
	// merely because another tool in that proposal is approved.
	return validateCommand(root, cfg.approvalTools(), cfg.command())
}

// atCapacityLocked reports whether starting a project would exceed the cap.
// Only projects with a live or stopping session count; a project already live
// under another server configuration does not count twice.
func (m *SessionManager) atCapacityLocked(project Project) bool {
	projects := map[Project]bool{}
	for _, e := range m.entries {
		if e.session != nil && (live(e.session.phaseNow()) || e.session.phaseNow() == phaseStopping) {
			projects[e.key.project] = true
		}
	}
	return !projects[project] && len(projects) >= m.options.MaxActive
}

// capacityVictimLocked returns the oldest idle session of another project when
// the cap is reached. Visible (non-idle) projects are never evicted.
func (m *SessionManager) capacityVictimLocked(project Project) *lspSession {
	if !m.atCapacityLocked(project) {
		return nil
	}
	var victim *lspSession
	var at uint64
	for _, e := range m.entries {
		if e.session == nil || e.key.project == project {
			continue
		}
		if since, ok := e.session.idleAt(); ok && (victim == nil || since < at) {
			victim, at = e.session, since
		}
	}
	return victim
}

// Hide starts the idle grace. The grace timer comes from After so tests drive it.
func (m *SessionManager) Hide(root AuthorizedRoot, cfg ServerConfig) {
	m.mu.Lock()
	e := m.entries[keyOf(root, cfg)]
	var s *lspSession
	if e != nil {
		s = e.session
	}
	m.mu.Unlock()
	if s == nil {
		return
	}
	cancel, ok := s.makeIdle(m.idleSeq.Add(1))
	if !ok {
		return
	}
	timer := m.options.After(m.options.IdleGrace)
	m.owned.timerWaiters.Add(1)
	go func() {
		defer m.owned.timerWaiters.Add(-1)
		select {
		case <-timer:
			if s.expireIdle(cancel) {
				s.stop(phaseStopped, nil, true) // shutdown/exit, kill fallback
			}
		case <-cancel:
		case <-s.disposed:
		}
	}()
}

// Revoke, Disconnect and Close bypass idle grace. Revoke/Disconnect kill the
// process tree at once; Close (app shutdown) tries shutdown/exit under deadline.
func (m *SessionManager) Revoke(root AuthorizedRoot) {
	m.stopWhere(func(k sessionKey) bool { return k.project == root.Project() }, phaseDisconnected, failure("approvalRequired", "Execution approval was revoked"), false)
}

func (m *SessionManager) Disconnect(hostID string) {
	m.stopWhere(func(k sessionKey) bool { return k.project.HostID == hostID }, phaseDisconnected, failure("disconnected", "Project host disconnected"), false)
}

func (m *SessionManager) Close() {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.stopWhere(func(sessionKey) bool { return true }, phaseDisconnected, failure("disconnected", "Language server owner is closed"), true)
	m.wg.Wait()
}

func (m *SessionManager) stopWhere(match func(sessionKey) bool, phase string, cause error, graceful bool) {
	m.mu.Lock()
	var list []*lspSession
	for k, e := range m.entries {
		if e.session != nil && match(k) {
			list = append(list, e.session)
		}
	}
	m.mu.Unlock()
	// Start every teardown before joining: a hung server must not add its
	// shutdown deadline to every other server's deadline.
	var stops sync.WaitGroup
	for _, s := range list {
		stops.Add(1)
		go func() {
			defer stops.Done()
			s.stop(phase, cause, graceful)
		}()
	}
	stops.Wait()
}

// Retry starts a fresh generation after failure or disconnect, at most
// MaxRestarts times per server. The failed generation is already invalid and can
// never publish again. Exhaustion is permanent until a deliberate new Attach.
func (m *SessionManager) Retry(ctx context.Context, root AuthorizedRoot, cfg ServerConfig) (SessionInfo, error) {
	cfg = cloneServer(cfg)
	tool := cfg.Tool()
	if m.trust == nil || m.options.Launch == nil {
		return SessionInfo{}, failure("invalidArgument", "Language server owner is not configured")
	}
	if err := m.gate(root, cfg); err != nil {
		return SessionInfo{}, err
	}
	m.mu.Lock()
	e := m.entries[keyOf(root, cfg)]
	if m.closed || e == nil || e.session == nil || (e.session.phaseNow() != phaseFailed && e.session.phaseNow() != phaseDisconnected) {
		m.mu.Unlock()
		return SessionInfo{}, ErrSessionNotRetryable
	}
	if e.restarted >= m.options.MaxRestarts {
		m.mu.Unlock()
		return SessionInfo{}, ErrSessionRestart
	}
	e.restarted++
	m.mu.Unlock()
	lease, err := m.trust.Preflight(root, cfg.approvalTools())
	if err != nil {
		return SessionInfo{}, err
	}
	return m.attach(ctx, root, cfg, tool, lease, true)
}

// Info reports the current generation, or a stopped placeholder.
func (m *SessionManager) Info(root AuthorizedRoot, cfg ServerConfig) SessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.entries[keyOf(root, cfg)]; e != nil && e.session != nil {
		return e.session.info()
	}
	return SessionInfo{Phase: phaseStopped, ExternalChanges: CoverageExternalChanges}
}

// CurrentGeneration reports whether results tagged with generation may still be
// published: false for any invalidated, replaced or stopped generation.
func (m *SessionManager) CurrentGeneration(root AuthorizedRoot, cfg ServerConfig, generation string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entries[keyOf(root, cfg)]
	return e != nil && e.session != nil && e.session.generation == generation && e.session.current()
}

func waitDone(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// lspSession is one generation of one server.
type lspSession struct {
	manager    *SessionManager
	entry      *sessionEntry
	root       AuthorizedRoot
	config     ServerConfig
	tool       ToolInput
	lease      *Lease
	generation string

	mu        sync.Mutex
	phase     string
	dead      bool // invalidated: publications must be dropped
	snap      SessionInfo
	tr        *LSPTransport
	proc      SessionProcess
	idleStamp uint64
	restarts  int
	idle      chan struct{}
	watchRej  int
	// Failure report facts (FailureLog only).
	started    time.Time
	exitStatus string

	// Transport-lifetime history: never evicted on close or attachment changes.
	publicationURIs map[string]bool // value true means reused/ambiguous
	publicationFull bool            // budget exhausted: reject every path push

	ready    chan struct{} // closed once with the first terminal/ready result
	readyErr error
	exited   chan struct{} // closed when the process Wait returns
	disposed chan struct{} // closed when all cleanup finished
	stopOnce sync.Once
	rdyOnce  sync.Once
}

func newLSPSession(m *SessionManager, e *sessionEntry, root AuthorizedRoot, cfg ServerConfig, tool ToolInput, lease *Lease) *lspSession {
	return &lspSession{
		manager: m, entry: e, root: root, config: cfg, tool: tool, lease: lease,
		generation: opaqueID(), phase: phaseStarting, restarts: e.restarted,
		ready: make(chan struct{}), exited: make(chan struct{}), disposed: make(chan struct{}),
	}
}

func (s *lspSession) phaseNow() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase
}

// current is false once the generation is invalidated or trust is revoked.
func (s *lspSession) current() bool {
	s.mu.Lock()
	dead := s.dead
	s.mu.Unlock()
	if dead || s.lease.Context.Err() != nil {
		return false
	}
	// Lock/config/toolchain drift invalidates publication and status without
	// pretending the callback is approval or a sandbox.
	if s.config.ValidateStart != nil {
		if err := s.config.ValidateStart(); err != nil {
			// Drift is terminal: reverting a config cannot revive a server that
			// might already have loaded changed inputs.
			s.stop(phaseFailed, boundedFailure(err), false)
			return false
		}
	}
	return true
}

func (s *lspSession) markReady(err error) {
	s.rdyOnce.Do(func() { s.readyErr = err; close(s.ready) })
}

func (s *lspSession) wait(ctx context.Context) (SessionInfo, error) {
	if err := waitDone(ctx, s.ready); err != nil {
		return SessionInfo{}, err
	}
	info := s.info()
	if s.readyErr != nil {
		return info, s.readyErr
	}
	return info, nil
}

func (s *lspSession) info() SessionInfo {
	s.mu.Lock()
	info := s.snap
	info.Generation, info.Phase, info.Root, info.ProviderID = s.generation, s.phase, s.root.Project(), s.config.ProviderID
	info.ExternalChanges, info.RejectedWatchers = CoverageExternalChanges, s.watchRej
	info.Restarts = s.restarts
	info.RetryAvailable = (s.phase == phaseFailed || s.phase == phaseDisconnected) && info.Restarts < s.manager.options.MaxRestarts
	info.Notice = "Unopened files are not continuously synchronized."
	s.mu.Unlock()
	if live(info.Phase) && s.config.ValidateStart != nil && !s.current() {
		info.Phase, info.Err = phaseFailed, failure("approvalChanged", "Language server inputs changed; review the workspace again")
		info.Notice = "Language server inputs changed; review the workspace again."
	}
	return info
}

func (s *lspSession) resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase == phaseIdle {
		s.phase = phaseInitialized
		s.idleStamp = 0
		close(s.idle)
		s.idle = nil
	}
}

func (s *lspSession) makeIdle(seq uint64) (chan struct{}, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != phaseInitialized || s.dead {
		return nil, false
	}
	s.phase = phaseIdle
	s.idleStamp = seq
	s.idle = make(chan struct{})
	return s.idle, true
}

func (s *lspSession) idleAt() (uint64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.idleStamp, s.phase == phaseIdle
}

// expireIdle reports whether the same idle period (not a resumed/re-hidden one) ended.
func (s *lspSession) expireIdle(token chan struct{}) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase == phaseIdle && s.idle == token
}

// deadline derives a context canceled by the injected timer, not the wall clock.
func (s *lspSession) deadline(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	timer := s.manager.options.After(d)
	s.manager.owned.timerWaiters.Add(1)
	go func() {
		defer s.manager.owned.timerWaiters.Add(-1)
		select {
		case <-timer:
			cancel(context.DeadlineExceeded)
		case <-ctx.Done():
		}
	}()
	return ctx, func() { cancel(context.Canceled) }
}

func (s *lspSession) run() {
	defer s.manager.wg.Done()
	defer s.manager.owned.runs.Add(-1)
	caps, err := s.initialize()
	if err != nil {
		s.stop(phaseFailed, boundedFailure(err), false)
		return
	}
	s.mu.Lock()
	if s.dead || s.phase != phaseStarting {
		s.mu.Unlock()
		return
	}
	s.phase = phaseInitialized
	s.snap.Capabilities = caps
	s.mu.Unlock()
	s.markReady(nil)
	s.serve()
}

func (s *lspSession) initialize() (ServerCapabilities, error) {
	dir, err := commandDirectory(s.root, s.config.Directory)
	if err == nil && s.config.ValidateStart != nil {
		err = s.config.ValidateStart()
	}
	if err == nil {
		err = s.lease.Validate(s.root, s.config.approvalTools()) // immediately before spawn
	}
	if err != nil {
		return ServerCapabilities{}, err
	}
	proc, streams, err := s.manager.options.Launch(Invocation{
		Executable: s.config.Executable, Args: s.config.Args, Env: s.config.command().environment(),
		Directory: dir, CleanupTimeout: s.manager.options.CleanupTimeout,
	})
	if err != nil {
		return ServerCapabilities{}, failure("toolFailed", "Language server could not be started")
	}
	s.mu.Lock()
	if s.dead { // stopped while launching: the tree must not outlive the generation
		s.mu.Unlock()
		_ = proc.Terminate()
		_ = proc.Close()
		s.waitProcess(proc)
		return ServerCapabilities{}, failure("staleContext", "Language server generation is no longer current")
	}
	s.proc, s.started = proc, time.Now()
	s.tr = NewLSPTransport(opaqueID(), s.generation, streams.Stdin, streams.Stdout, streams.Stderr,
		sessionCloser{proc, streams.Stdin}, s.manager.options.LSP)
	tr := s.tr
	s.mu.Unlock()
	s.waitProcess(proc)

	ctx, cancel := s.deadline(s.lease.Context, s.manager.options.InitTimeout)
	defer cancel()
	params := s.initializeParams()
	// Revalidate the contained directory after launch as well; initialize must
	// never silently widen an invalid workspace back to the attachment root.
	if params == nil {
		return ServerCapabilities{}, failure("unauthorized", "Invalid language server workspace")
	}
	raw, err := tr.call(ctx, "initialize", params, func(ctx context.Context, msg LSPMessage) {
		// Startup cannot publish diagnostics or re-enter the document manager,
		// which may be waiting on Attach. Requests still need prompt replies.
		if msg.Kind == LSPServerRequest && s.current() {
			s.handleRequest(ctx, tr, msg)
		}
	})
	if err != nil {
		if errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
			err = context.DeadlineExceeded
		}
		return ServerCapabilities{}, err
	}
	if !s.current() {
		return ServerCapabilities{}, failure("staleContext", "Language server generation is no longer current")
	}
	caps, err := negotiateCapabilities(raw)
	if err != nil {
		return ServerCapabilities{}, err
	}
	// initialized strictly follows a validated initialize result.
	if err := tr.Notify(ctx, "initialized", struct{}{}); err != nil {
		return ServerCapabilities{}, err
	}
	return caps, nil
}

// initializeParams advertises only what this client implements: the UTF-16
// position encoding, plus workspace/configuration when the adapter supplies
// fixed settings and publishDiagnostics (no versionSupport) when it declares
// diagnostics. Definition (with links), references and hover are requests
// the client sends; they say what answers it can read, start nothing and are
// not part of the approval policy. No other textDocument or window
// capability, no edits, and no dynamic registration except an adapter's
// AcknowledgeWatchers (acknowledged, never delivered). A scoped
// workspace folder and rootUri name only the validated contained Directory,
// never a broader project root.
func (s *lspSession) initializeParams() map[string]any {
	dir, err := commandDirectory(s.root, s.config.Directory)
	if err != nil {
		return nil
	}
	capabilities := map[string]any{"general": map[string]any{"positionEncodings": []string{"utf-16"}}}
	workspace := map[string]any{}
	if s.config.Settings != nil {
		workspace["configuration"] = true
	}
	if s.config.AcknowledgeWatchers {
		workspace["didChangeWatchedFiles"] = map[string]any{"dynamicRegistration": true}
	}
	if len(workspace) > 0 {
		capabilities["workspace"] = workspace
	}
	textDocument := map[string]any{
		"definition": map[string]any{"linkSupport": true},
		"references": map[string]any{},
		"hover":      map[string]any{"contentFormat": []string{"markdown", "plaintext"}},
	}
	if s.config.DeclareDiagnostics {
		textDocument["publishDiagnostics"] = map[string]any{}
	}
	capabilities["textDocument"] = textDocument
	params := map[string]any{
		"processId":    nil,
		"clientInfo":   map[string]string{"name": clientName, "version": clientVersion},
		"rootUri":      fileURI(dir),
		"capabilities": capabilities,
	}
	if len(s.config.InitializationOptions) > 0 {
		var options any
		if json.Unmarshal(s.config.InitializationOptions, &options) != nil {
			return nil
		}
		params["initializationOptions"] = options
	}
	if s.config.Directory != "" {
		params["workspaceFolders"] = []map[string]string{{"uri": fileURI(dir), "name": filepath.Base(dir)}}
	}
	return params
}

// serve applies the request policy until the transport or generation ends.
// Anything arriving for an invalid generation is drained and dropped.
func (s *lspSession) serve() {
	tr := s.transport()
	for {
		select {
		case msg, ok := <-tr.Incoming():
			if !ok {
				s.stopIfLive(tr)
				return
			}
			if s.current() {
				s.handle(tr, msg)
			}
		case <-tr.Done():
			s.stopIfLive(tr)
			return
		case <-s.lease.Context.Done():
			s.stop(phaseDisconnected, failure("approvalRequired", "Execution approval is no longer current"), false)
			return
		case <-s.disposed:
			return
		}
	}
}

func (s *lspSession) stopIfLive(tr *LSPTransport) {
	if s.current() {
		s.stop(phaseFailed, boundedFailure(tr.Err()), false)
	}
}

// handle gives every server request an explicit, prompt reply. Only
// publishDiagnostics notifications are forwarded (CD-22) to an owner-supplied
// callback; this layer itself never interprets or publishes diagnostics.
func (s *lspSession) handle(tr *LSPTransport, msg LSPMessage) {
	if msg.Kind == LSPNotification {
		if h := s.manager.options.Notifications; h != nil && msg.Method == "textDocument/publishDiagnostics" {
			h(SessionNotification{Project: s.root.Project(), ProviderID: s.config.ProviderID, Generation: s.generation, Method: msg.Method, Params: msg.Params})
		}
		return
	}
	if msg.Kind == LSPServerRequest {
		s.handleRequest(s.lease.Context, tr, msg)
	}
}

// handleRequest serves the same fixed policy during startup and normal operation.
// It must never call notification callbacks or the document manager.
func (s *lspSession) handleRequest(parent context.Context, tr *LSPTransport, msg LSPMessage) {
	ctx, cancel := s.deadline(parent, s.manager.options.ShutdownTimeout)
	defer cancel()
	const methodNotFound, invalidParams = -32601, -32602
	switch msg.Method {
	case "workspace/configuration":
		if result, ok := boundedConfiguration(s.root, msg.Params, s.config.Settings); ok {
			_ = tr.Respond(ctx, msg.ID, result)
		} else {
			_ = tr.RespondError(ctx, msg.ID, invalidParams, "configuration request rejected")
		}
	case "client/registerCapability":
		// File watching is not supported: reject honestly instead of accepting a
		// registration this client would never deliver, unless the adapter needs
		// the acknowledgement (AcknowledgeWatchers). Coverage stays unwatched.
		if s.config.AcknowledgeWatchers && onlyWatcherRegistrations(msg.Params) {
			_ = tr.Respond(ctx, msg.ID, nil)
			return
		}
		s.mu.Lock()
		if bytesContain(msg.Params, "workspace/didChangeWatchedFiles") && s.watchRej < maxWatchRejects {
			s.watchRej++
		}
		s.mu.Unlock()
		_ = tr.RespondError(ctx, msg.ID, methodNotFound, "dynamic registration is not supported")
	case "workspace/applyEdit":
		_ = tr.RespondError(ctx, msg.ID, methodNotFound, "workspace edits are denied")
	case "workspace/executeCommand", "workspace/workspaceFolders", "client/unregisterCapability",
		"window/showDocument", "window/showMessageRequest", "window/workDoneProgress/create":
		_ = tr.RespondError(ctx, msg.ID, methodNotFound, "unsupported")
	default:
		_ = tr.RespondError(ctx, msg.ID, methodNotFound, "unsupported")
	}
}

// onlyWatcherRegistrations reports a bounded registration request made only of
// didChangeWatchedFiles entries; anything else is still rejected.
func onlyWatcherRegistrations(raw json.RawMessage) bool {
	if len(raw) > maxServerParams {
		return false
	}
	var params struct {
		Registrations []struct {
			Method string `json:"method"`
		} `json:"registrations"`
	}
	if json.Unmarshal(raw, &params) != nil || len(params.Registrations) == 0 {
		return false
	}
	for _, r := range params.Registrations {
		if r.Method != "workspace/didChangeWatchedFiles" {
			return false
		}
	}
	return true
}

func bytesContain(raw json.RawMessage, sub string) bool {
	if len(raw) > maxServerParams {
		raw = raw[:maxServerParams]
	}
	return strings.Contains(string(raw), sub)
}

// stop invalidates the generation BEFORE any cleanup, then shuts the server
// down. graceful tries shutdown+exit under a deadline; the process tree is
// always terminated afterwards, so a hung or descendant-leaving server cannot
// outlive its session. Pending waiters are failed once by the transport close.
func (s *lspSession) stop(phase string, cause error, graceful bool) {
	s.mu.Lock()
	s.dead = true
	if s.idle != nil {
		close(s.idle)
		s.idle = nil
	}
	s.mu.Unlock()
	s.stopOnce.Do(func() {
		s.mu.Lock()
		// shutdown is only legal after a successful initialize.
		graceful = graceful && (s.phase == phaseInitialized || s.phase == phaseIdle)
		before := s.phase
		s.phase = phaseStopping
		tr, proc := s.tr, s.proc
		s.mu.Unlock()
		if graceful && tr != nil && tr.Err() == nil {
			ctx, cancel := s.deadline(context.Background(), s.manager.options.ShutdownTimeout)
			if _, err := tr.Call(ctx, "shutdown", nil); err == nil {
				_ = tr.Notify(ctx, "exit", nil)
				select {
				case <-s.exited:
				case <-ctx.Done():
				}
			}
			cancel()
		}
		if proc != nil && phase == phaseFailed && s.manager.options.FailureLog != nil {
			s.drainCrash(tr)
		}
		if proc != nil {
			_ = proc.Terminate()
			_ = proc.Close()
		}
		// Close keeps the first failure and joins the stderr reader.
		var status LSPStatus
		if tr != nil {
			_ = tr.Close()
			status = tr.Status()
		}
		if proc != nil {
			select {
			case <-s.exited:
			case <-s.manager.options.After(s.manager.options.CleanupTimeout):
			}
		}
		s.mu.Lock()
		s.phase = phase
		s.snap.Err = cause
		var report SessionFailure
		if phase == phaseFailed {
			report = SessionFailure{
				Project: s.root.Project(), ProviderID: s.config.ProviderID, Generation: s.generation,
				Executable: s.config.Executable, Args: append([]string(nil), s.config.Args...), Directory: s.config.Directory,
				Started: s.started, Failed: time.Now(), Phase: before, Restarts: s.restarts, Cause: cause,
				Transport: status.Err, Exit: s.exitStatus,
				Stderr: status.Stderr, StderrTail: status.StderrTail, StderrTruncated: status.StderrTruncated, StderrBytes: status.StderrBytes,
			}
		}
		s.mu.Unlock()
		s.markReady(cause)
		close(s.disposed)
		if log := s.manager.options.FailureLog; log != nil && phase == phaseFailed {
			log(report)
		}
	})
	<-s.disposed
}

// drainCrash waits up to crashDrainTimeout for a failed server to be reaped
// and its stderr to reach EOF. The transport already killed the tree on
// failure; closing the pipes now could drop the end of a crash stack.
func (s *lspSession) drainCrash(tr *LSPTransport) {
	exited, stderr := (<-chan struct{})(s.exited), (<-chan struct{})(nil)
	if tr != nil {
		stderr = tr.StderrDone()
	}
	timer := time.NewTimer(crashDrainTimeout)
	defer timer.Stop()
	for exited != nil || stderr != nil {
		select {
		case <-exited:
			exited = nil
		case <-stderr:
			stderr = nil
		case <-timer.C:
			return
		}
	}
}

func (s *lspSession) transport() *LSPTransport {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tr
}

// sessionCloser is the transport closer: it unblocks pipe I/O and kills the tree.
type sessionCloser struct {
	proc  SessionProcess
	stdin io.WriteCloser
}

func (c sessionCloser) Close() error {
	_ = c.stdin.Close()
	_ = c.proc.Terminate()
	return nil
}

// boundedFailure maps internal errors to bounded CD-01 errors with no server text.
func boundedFailure(err error) error {
	var lsp *LSPResponseError
	var e *Error
	switch {
	case errors.As(err, &e):
		return e
	case errors.As(err, &lsp):
		return failure("toolFailed", "Language server rejected initialization")
	case errors.Is(err, context.DeadlineExceeded):
		return failure("timeout", "Language server did not respond in time")
	case errors.Is(err, context.Canceled):
		return failure("canceled", "Language server startup was canceled")
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.ErrClosedPipe):
		return failure("toolFailed", "Language server exited")
	case errors.Is(err, ErrLSPBadHeader), errors.Is(err, ErrLSPHeaderLimit), errors.Is(err, ErrLSPBadLength), errors.Is(err, ErrLSPBadJSON):
		return failure("toolFailed", "Language server wrote output that is not the language server protocol")
	case errors.Is(err, ErrLSPFrameTooLarge):
		return failure("limitExceeded", "Language server sent a message over the size limit")
	case errors.Is(err, ErrLSPQueueFull), errors.Is(err, ErrLSPTooManyPending):
		return failure("limitExceeded", "Language server sent messages faster than they could be handled")
	}
	return failure("toolFailed", "Language server stopped")
}

// negotiateCapabilities accepts only what the client implements: UTF-16 positions
// and full/incremental open/close synchronization. Anything else fails the
// session rather than letting later ranges or versions be misread.
func negotiateCapabilities(raw json.RawMessage) (ServerCapabilities, error) {
	invalid := failure("toolFailed", "Language server initialization is invalid")
	if len(raw) > maxInitBytes {
		return ServerCapabilities{}, failure("limitExceeded", "Language server initialization exceeds limits")
	}
	var reply struct {
		Capabilities *struct {
			PositionEncoding   string          `json:"positionEncoding"`
			TextDocumentSync   json.RawMessage `json:"textDocumentSync"`
			DiagnosticProvider json.RawMessage `json:"diagnosticProvider"`
			DefinitionProvider json.RawMessage `json:"definitionProvider"`
			ReferencesProvider json.RawMessage `json:"referencesProvider"`
			HoverProvider      json.RawMessage `json:"hoverProvider"`
		} `json:"capabilities"`
	}
	if json.Unmarshal(raw, &reply) != nil || reply.Capabilities == nil {
		return ServerCapabilities{}, invalid
	}
	c := reply.Capabilities
	if c.PositionEncoding != "" && c.PositionEncoding != "utf-16" {
		return ServerCapabilities{}, failure("unsupported", "Language server position encoding is not supported")
	}
	caps := ServerCapabilities{PositionEncoded: "utf-16"}
	caps.PullAdvertised = advertised(c.DiagnosticProvider) // recorded; push remains the only baseline
	caps.Definition = advertised(c.DefinitionProvider)
	caps.References = advertised(c.ReferencesProvider)
	caps.Hover = advertised(c.HoverProvider)
	if len(c.TextDocumentSync) == 0 || string(c.TextDocumentSync) == "null" {
		return ServerCapabilities{}, failure("unsupported", "Language server does not support document synchronization")
	}
	if c.TextDocumentSync[0] != '{' {
		var kind int
		if json.Unmarshal(c.TextDocumentSync, &kind) != nil {
			return ServerCapabilities{}, invalid
		}
		caps.OpenClose, caps.Change = true, kind
	} else {
		var sync struct {
			OpenClose bool            `json:"openClose"`
			Change    *int            `json:"change"`
			Save      json.RawMessage `json:"save"`
		}
		if json.Unmarshal(c.TextDocumentSync, &sync) != nil || sync.Change == nil {
			return ServerCapabilities{}, invalid
		}
		caps.OpenClose, caps.Change = sync.OpenClose, *sync.Change
		switch s := string(sync.Save); {
		case s == "" || s == "null" || s == "false":
		case s == "true" || s[0] == '{':
			caps.Save = true // didSave only; the client never sends document text in it
		default:
			return ServerCapabilities{}, invalid
		}
	}
	if caps.Change < 1 || caps.Change > 2 || !caps.OpenClose {
		return ServerCapabilities{}, failure("unsupported", "Language server synchronization mode is not supported")
	}
	return caps, nil
}

// advertised reads a provider capability: true or an options object.
func advertised(raw json.RawMessage) bool {
	s := string(raw)
	return s != "" && s != "null" && s != "false"
}

// boundedConfiguration answers workspace/configuration from the adapter's fixed
// settings only: the empty section is the whole object, a dotted section is the
// nested value, anything absent is null. Without settings every item is null.
// The request is still validated: bounded size/items/sections and every scope
// must lie inside the approved root. Scopes never select different settings.
func boundedConfiguration(root AuthorizedRoot, params json.RawMessage, settings json.RawMessage) ([]any, bool) {
	if len(params) == 0 || len(params) > maxServerParams {
		return nil, false
	}
	var req struct {
		Items []struct {
			ScopeURI *string `json:"scopeUri"`
			Section  *string `json:"section"`
		} `json:"items"`
	}
	if json.Unmarshal(params, &req) != nil || len(req.Items) == 0 || len(req.Items) > maxConfigItems {
		return nil, false
	}
	for _, item := range req.Items {
		// vtsls asks for the empty section (the whole configuration).
		if item.Section != nil && *item.Section != "" && !bounded(*item.Section, maxConfigSect) {
			return nil, false
		}
		if item.ScopeURI != nil && !insideRoot(root, *item.ScopeURI) {
			return nil, false
		}
	}
	out := make([]any, len(req.Items))
	if settings == nil {
		return out, true
	}
	var whole any
	if json.Unmarshal(settings, &whole) != nil {
		return nil, false
	}
	for i, item := range req.Items {
		out[i] = whole
		if item.Section != nil && *item.Section != "" {
			out[i] = settingsSection(whole, *item.Section)
		}
	}
	return out, true
}

func settingsSection(v any, section string) any {
	for _, key := range strings.Split(section, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		if v, ok = m[key]; !ok {
			return nil
		}
	}
	return v
}

// insideRoot is a lexical, metadata-free containment check on file URIs.
func insideRoot(root AuthorizedRoot, raw string) bool {
	if !bounded(raw, maxScopeURI) {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "file" || u.Host != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	path := u.Path
	if runtime.GOOS == "windows" && len(path) > 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	native := filepath.FromSlash(path)
	if !filepath.IsAbs(native) || hasParent(native) {
		return false
	}
	rel, err := filepath.Rel(root.Project().Root, filepath.Clean(native))
	return err == nil && filepath.IsLocal(rel) || rel == "."
}

func fileURI(path string) string {
	p := filepath.ToSlash(path)
	if len(p) > 1 && p[1] == ':' {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}
