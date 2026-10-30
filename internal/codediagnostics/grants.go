package codediagnostics

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/ginkcode/agent-sessions/internal/configfile"
)

// GrantStore is a host-user-owned dedicated store, not the shared config.toml.
// All reads and read/modify/writes take the same cross-process configfile lock.
type GrantStore struct{ path string }

func NewGrantStore(configDir string) (*GrantStore, error) {
	if !filepath.IsAbs(configDir) {
		return nil, failure("invalidArgument", "Grant config directory must be absolute")
	}
	return &GrantStore{filepath.Join(configDir, "code-diagnostics", "grants.json")}, nil
}

type grantRecord struct {
	Project     Project `json:"project"`
	Fingerprint string  `json:"fingerprint"`
	// Kind is what was approved; records saved before kinds have none.
	Kind string `json:"kind,omitempty"`
}

// A project is approved once per kind: later tool or configuration changes
// carry its grant over to the new fingerprint. Approving programs covers
// in-process analyzers too, never the reverse, because the in-process approval
// says that it runs no program. Kindless records carry over only after an
// exact match has stamped their kind.
const (
	grantPrograms  = "programs"
	grantInProcess = "inProcess"
)

func covers(granted, wanted string) bool {
	return granted != "" && (granted == wanted || granted == grantPrograms)
}

type grantDisk struct {
	Version int           `json:"version"`
	Grants  []grantRecord `json:"grants"`
}

func validProject(p Project) bool {
	return bounded(p.HostID, 256) && bounded(p.Root, 4096) && filepath.IsAbs(p.Root) && !hasParent(p.Root) && filepath.Clean(p.Root) == p.Root
}

func (s *GrantStore) transaction(edit func(*grantDisk) error, write bool) error {
	lock, err := configfile.Lock(s.path)
	if err != nil {
		return failure("busy", "Cannot lock execution grants")
	}
	defer func() { _ = lock.Unlock() }()
	disk := grantDisk{1, []grantRecord{}}
	f, err := os.Open(s.path)
	if err == nil {
		data, readErr := io.ReadAll(io.LimitReader(f, (1<<20)+1))
		_ = f.Close()
		if readErr != nil || len(data) > 1<<20 || json.Unmarshal(data, &disk) != nil || disk.Version != 1 || disk.Grants == nil || len(disk.Grants) > 1024 {
			return failure("approvalRequired", "Execution grant store is invalid; repair or remove it before approval")
		}
		seen := map[Project]bool{}
		for _, g := range disk.Grants {
			if !validProject(g.Project) || !validFingerprint(g.Fingerprint) || (g.Kind != "" && g.Kind != grantPrograms && g.Kind != grantInProcess) || seen[g.Project] {
				return failure("approvalRequired", "Execution grant records are invalid")
			}
			seen[g.Project] = true
		}
	} else if !os.IsNotExist(err) {
		return failure("approvalRequired", "Execution grants cannot be read")
	}
	if err := edit(&disk); err != nil {
		return err
	}
	if !write {
		return nil
	}
	if len(disk.Grants) > 1024 {
		return failure("limitExceeded", "Too many persistent execution grants")
	}
	data, err := json.Marshal(disk)
	if err != nil || len(data) > 1<<20 {
		return failure("limitExceeded", "Execution grants exceed storage limit")
	}
	if err := configfile.WriteAtomic(s.path, data); err != nil {
		return failure("toolFailed", "Cannot save execution grants")
	}
	return nil
}

// lookup reports the project's grant kind if it covers fp of kind, carrying the
// record over to fp first when the fingerprint changed.
func (s *GrantStore) lookup(p Project, fp, kind string) (string, error) {
	var record *grantRecord
	err := s.transaction(func(d *grantDisk) error {
		for _, g := range d.Grants {
			if g.Project == p {
				record = &g
			}
		}
		return nil
	}, false)
	if err != nil || record == nil {
		return "", err
	}
	granted := record.Kind
	switch {
	case record.Fingerprint == fp && granted != "":
		return granted, nil
	case record.Fingerprint == fp:
		granted = kind // An exact match of a kindless record learns its kind.
	case !covers(granted, kind):
		return "", nil
	}
	// Compare-and-swap: another owner may have revoked or replaced the record.
	found := ""
	err = s.transaction(func(d *grantDisk) error {
		for i := range d.Grants {
			if d.Grants[i] == *record {
				d.Grants[i].Fingerprint, d.Grants[i].Kind = fp, granted
				found = granted
			}
		}
		return nil
	}, true)
	return found, err
}

func (s *GrantStore) set(p Project, fp, kind string) error {
	return s.transaction(func(d *grantDisk) error {
		kept := d.Grants[:0]
		for _, g := range d.Grants {
			if g.Project != p {
				kept = append(kept, g)
			}
		}
		d.Grants = kept
		if fp != "" {
			d.Grants = append(d.Grants, grantRecord{p, fp, kind})
		}
		return nil
	}, true)
}

type Approval struct {
	State       string `json:"state"`
	Mode        string `json:"mode,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}
type grantState struct {
	fingerprint, mode, kind string
	revoked                 bool
	epoch                   uint64
	ctx                     context.Context
	cancel                  context.CancelFunc
}

// Trust owns exactly one authenticated connection/app lifetime (including Local).
// Close MUST be called on disconnect/shutdown. Never reuse it on reconnect.
// One authority per host-user owns revocation; peer processes observe durable
// changes on lookup/preflight, not via a cross-process notification service.
type Trust struct {
	mu     sync.Mutex
	store  *GrantStore
	states map[Project]*grantState
	closed bool
}

func NewTrust(store *GrantStore) *Trust {
	return &Trust{store: store, states: map[Project]*grantState{}}
}

func (t *Trust) state(p Project) *grantState {
	s := t.states[p]
	if s == nil {
		ctx, cancel := context.WithCancel(context.Background())
		s = &grantState{ctx: ctx, cancel: cancel}
		t.states[p] = s
	}
	return s
}

func invalidate(s *grantState) {
	s.epoch++
	s.cancel()
	s.ctx, s.cancel = context.WithCancel(context.Background())
}

func (t *Trust) lookup(p Project, fp, kind string) (Approval, error) {
	if t.closed {
		return Approval{}, failure("disconnected", "Execution trust owner is closed")
	}
	s := t.state(p)
	if s.revoked {
		return Approval{State: "revoked"}, nil
	}
	if s.mode == "session" && covers(s.kind, kind) {
		// Leases of the old identity end; their inputs are no longer current.
		if s.fingerprint != fp {
			invalidate(s)
			s.fingerprint = fp
		}
		return Approval{"approved", "session", fp}, nil
	}
	if t.store != nil {
		granted, err := t.store.lookup(p, fp, kind)
		if err != nil {
			invalidate(s)
			return Approval{}, err
		}
		if granted != "" {
			if s.fingerprint != fp || s.mode != "persistent" {
				invalidate(s)
				s.fingerprint, s.mode = fp, "persistent"
			}
			s.kind = granted
			return Approval{"approved", "persistent", fp}, nil
		}
	}
	// A changed fingerprint or externally removed grant cancels old leases.
	invalidate(s)
	return Approval{State: "required"}, nil
}

// Lookup is the approval of a proposal that runs programs; LookupInProcess of
// one made only of in-process analyzers.
func (t *Trust) Lookup(root AuthorizedRoot, fp string) (Approval, error) {
	return t.lookupRoot(root, fp, grantPrograms)
}

func (t *Trust) LookupInProcess(root AuthorizedRoot, fp string) (Approval, error) {
	return t.lookupRoot(root, fp, grantInProcess)
}

func (t *Trust) lookupRoot(root AuthorizedRoot, fp, kind string) (Approval, error) {
	if !validFingerprint(fp) {
		return Approval{}, failure("invalidArgument", "Invalid execution fingerprint")
	}
	if err := root.Revalidate(); err != nil {
		return Approval{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lookup(root.project, fp, kind)
}

// Approve re-discovers the proposed metadata under the trust lock. Approval
// never launches tools. A stale proposal cannot approve a changed executable.
func (t *Trust) Approve(root AuthorizedRoot, tools []ToolInput, proposal, mode string) (Approval, error) {
	return t.approve(root, func() (string, error) { return Fingerprint(root, tools) }, grantPrograms, proposal, mode)
}

// ApproveInProcess approves a proposal made only of reviewed in-process
// analyzers (no executable). The identity is recomputed under the trust lock
// exactly like Approve; it grants no executable and Preflight still requires
// an executable fingerprint, so no tool can launch on this grant.
func (t *Trust) ApproveInProcess(root AuthorizedRoot, analyzers []string, proposal, mode string) (Approval, error) {
	return t.approve(root, func() (string, error) { return InProcessFingerprint(root, analyzers) }, grantInProcess, proposal, mode)
}

func (t *Trust) approve(root AuthorizedRoot, identity func() (string, error), kind, proposal, mode string) (Approval, error) {
	if mode != "session" && mode != "persistent" {
		return Approval{}, failure("invalidArgument", "Invalid approval mode")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return Approval{}, failure("disconnected", "Execution trust owner is closed")
	}
	fp, err := identity()
	if err != nil {
		return Approval{}, err
	}
	if fp != proposal {
		return Approval{}, failure("approvalChanged", "Execution identity changed; review a new approval proposal")
	}
	if mode == "persistent" {
		if t.store == nil {
			return Approval{}, failure("unsupported", "Persistent execution grants are unavailable")
		}
		if err := t.store.set(root.project, fp, kind); err != nil {
			return Approval{}, err
		}
	}
	s := t.state(root.project)
	invalidate(s)
	s.fingerprint = fp
	s.mode = mode
	s.kind = kind
	s.revoked = false
	return Approval{"approved", mode, fp}, nil
}

// Rebind carries a single current lease over planned setup outputs. The caller
// validates outputs; this method checks the exact epoch and old durable grant.
func (t *Trust) Rebind(root AuthorizedRoot, lease *Lease, oldTools, tools []ToolInput, expected string) (Approval, error) {
	if lease == nil || lease.trust != t || lease.project != root.project {
		return Approval{}, failure("approvalRequired", "Execution approval is no longer current")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.state(root.project)
	if t.closed || s.revoked || s.epoch != lease.epoch || s.fingerprint != lease.fingerprint {
		return Approval{}, failure("approvalRequired", "Execution approval is no longer current")
	}
	oldFP, err := Fingerprint(root, oldTools)
	if err != nil || oldFP != lease.fingerprint {
		invalidate(s)
		return Approval{}, failure("approvalChanged", "Original execution inputs changed during setup")
	}
	current, err := t.lookup(root.project, lease.fingerprint, grantPrograms)
	if err != nil {
		return Approval{}, err
	}
	if current.State != "approved" || s.epoch != lease.epoch {
		return Approval{}, failure("approvalRequired", "Execution approval is no longer current")
	}
	fp, err := Fingerprint(root, tools)
	if err != nil {
		return Approval{}, err
	}
	if fp != expected {
		return Approval{}, failure("approvalChanged", "Prepared execution inputs changed during verification")
	}
	if current.Mode == "persistent" {
		// Compare-and-swap in one disk transaction: another owner may revoke
		// between lookup and this write. Never resurrect their removed grant.
		err = t.store.transaction(func(d *grantDisk) error {
			for i := range d.Grants {
				if d.Grants[i].Project == root.project && d.Grants[i].Fingerprint == lease.fingerprint {
					d.Grants[i].Fingerprint, d.Grants[i].Kind = fp, grantPrograms
					return nil
				}
			}
			return failure("approvalRequired", "Execution approval is no longer current")
		}, true)
		if err != nil {
			invalidate(s)
			return Approval{}, err
		}
	}
	invalidate(s)
	s.fingerprint, s.mode = fp, current.Mode
	return Approval{"approved", current.Mode, fp}, nil
}

// Deny is Not now: it creates no grant, and does not revoke an existing grant.
func (t *Trust) Deny() {}

// Revoke invalidates generation BEFORE cancellation and durable removal. Even
// if the disk write fails, this owner remains revoked and its leases are dead.
func (t *Trust) Revoke(root AuthorizedRoot) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return failure("disconnected", "Execution trust owner is closed")
	}
	s := t.state(root.project)
	s.revoked = true
	s.fingerprint = ""
	s.mode = ""
	s.kind = ""
	invalidate(s)
	if t.store != nil {
		return t.store.set(root.project, "", "")
	}
	return nil
}

func (t *Trust) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	for _, s := range t.states {
		s.epoch++
		s.cancel()
		s.fingerprint = ""
		s.mode = ""
		s.kind = ""
	}
}

// Lease is a queued/running-work cancellation hook, not a process launch permit.
// Future manager/supervisor must validate immediately before spawn AND before
// publication, and observe Context for process-tree cancellation. Epoch changes
// are committed before Context is canceled. Never hold work across reconnects.
type Lease struct {
	trust       *Trust
	project     Project
	epoch       uint64
	fingerprint string
	Context     context.Context
}

func (t *Trust) Preflight(root AuthorizedRoot, tools []ToolInput) (*Lease, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fp, err := Fingerprint(root, tools)
	if err != nil {
		invalidate(t.state(root.project))
		return nil, err
	}
	approval, err := t.lookup(root.project, fp, grantPrograms)
	if err != nil {
		return nil, err
	}
	if approval.State != "approved" {
		return nil, failure("approvalRequired", "Explicit execution approval is required")
	}
	s := t.state(root.project)
	return &Lease{t, root.project, s.epoch, fp, s.ctx}, nil
}

// Validate also rehashes identity and rechecks the durable grant, not just epoch.
func (l *Lease) Validate(root AuthorizedRoot, tools []ToolInput) error {
	t := l.trust
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.state(l.project)
	if t.closed || s.epoch != l.epoch || root.project != l.project {
		return failure("staleContext", "Execution trust generation is no longer current")
	}
	fp, err := Fingerprint(root, tools)
	if err != nil || fp != l.fingerprint {
		invalidate(s)
		return failure("approvalChanged", "Execution identity changed")
	}
	a, err := t.lookup(l.project, fp, grantPrograms)
	if err != nil {
		return err
	}
	if a.State != "approved" || s.epoch != l.epoch {
		return failure("approvalRequired", "Execution approval is no longer current")
	}
	return nil
}
