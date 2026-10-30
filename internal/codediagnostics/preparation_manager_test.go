package codediagnostics

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// All binaries in this fixture are inert identity files. Only fakeFactory can
// launch a process, so these tests never execute a tool or access the network.
type preparationManagerFixture struct {
	manager        *Manager
	root           AuthorizedRoot
	snapshot       Snapshot
	starts         chan started
	plan           *PreparationPlan
	oldTools       []ToolInput
	newTools       []ToolInput
	registrations  []Registration
	validated      atomic.Int32
	prepared       atomic.Int32
	published      atomic.Int32
	rejectPrepared bool
}

func newPreparationManagerFixture(t *testing.T, grants *GrantStore) *preparationManagerFixture {
	t.Helper()
	root, tools, _ := fixture(t)
	starts := make(chan started, 8)
	trust := NewTrust(grants)
	manager, err := NewManager("preparation-test", trust, NewSupervisor(trust, SupervisorOptions{Start: fakeFactory(starts)}), ManagerLimits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	f := &preparationManagerFixture{manager: manager, root: root, starts: starts}
	workRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(workRoot, "setup")
	command := Command{ProviderID: "setup", Executable: tools[0].Executable, Args: []string{"--fixed-setup"}, PreparationDirectory: work, AdapterPolicy: "fake-setup-v1", AcceptedExits: []int{0}}
	setupTool := ToolInput{ProviderID: command.ProviderID, Executable: command.Executable, ConfigPaths: []string{"config"}, Policy: command.Policy()}
	output := filepath.Join(t.TempDir(), "prepared-tool")
	if err := os.WriteFile(output, []byte("inert prepared executable identity"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.oldTools = append(cloneTools(tools), setupTool)
	f.newTools = append(cloneTools(f.oldTools), ToolInput{ProviderID: "prepared-checker", Executable: output, Policy: "prepared-checker-v1"})
	f.plan = &PreparationPlan{
		Steps:   []PreparationStep{{Name: "Install synthetic tool", Command: command}},
		Tools:   []ToolInput{setupTool},
		Outputs: []string{output},
		Covers:  []string{"prepared"},
		Validate: func() error {
			f.validated.Add(1)
			return nil
		},
		Prepared: func(next []Registration) error {
			f.prepared.Add(1)
			if f.rejectPrepared {
				return errors.New("unplanned output")
			}
			if len(next) != 1 || !reflect.DeepEqual(next[0].Adapter.Tools(), f.newTools) {
				return errors.New("unexpected rediscovered registrations")
			}
			return nil
		},
	}
	// Two providers share the same plan; both discovery and execution must count
	// it once, not run a second setup process for a second consumer.
	f.registrations = []Registration{
		{ProviderID: "checker", Availability: "available", Adapter: &fakeChecker{tools: tools, calls: make(chan adapterCall, 1)}},
		{ProviderID: "setup-consumer", Availability: "missingTool", Prepare: f.plan},
		{ProviderID: "another-consumer", Availability: "missingTool", Prepare: f.plan},
	}
	f.snapshot, err = manager.Attach(root, 1, f.registrations)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *preparationManagerFixture) approve(t *testing.T, mode string) {
	t.Helper()
	var err error
	f.snapshot, err = f.manager.Approve(f.snapshot.Context, f.snapshot.Proposal.Fingerprint, mode)
	if err != nil {
		t.Fatal(err)
	}
}

func (f *preparationManagerFixture) discover(AuthorizedRoot) ([]Registration, error) {
	return []Registration{{ProviderID: "checker", Availability: "available", Adapter: &fakeChecker{tools: f.newTools, calls: make(chan adapterCall, 1)}}}, nil
}

func (f *preparationManagerFixture) publish(root AuthorizedRoot, next []Registration) error {
	f.published.Add(1)
	if root.Project() != f.root.Project() || len(next) != 1 {
		return errors.New("wrong prepared metadata")
	}
	return nil
}

func waitPreparationWorkers(t *testing.T, manager *Manager) {
	t.Helper()
	done := make(chan struct{})
	go func() { manager.workers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("preparation worker did not finish")
	}
}

func noPreparationStart(t *testing.T, starts <-chan started) {
	t.Helper()
	select {
	case <-starts:
		t.Fatal("unexpected preparation process")
	default:
	}
}

func TestPreparationManagerApprovalAndCoveredAddition(t *testing.T) {
	f := newPreparationManagerFixture(t, nil)
	initial := f.snapshot.Proposal.Fingerprint
	if f.snapshot.Approval.State != "required" || f.snapshot.Preparation != nil || len(f.snapshot.Proposal.Preparation) != 1 || len(f.snapshot.Proposal.Outputs) != 1 {
		t.Fatalf("initial proposal: %+v", f.snapshot)
	}
	code(t, f.manager.StartPreparation(f.snapshot.Context, f.discover, f.publish), "invalidArgument")
	noPreparationStart(t, f.starts)
	f.approve(t, "session")
	if f.snapshot.Preparation == nil || f.snapshot.Preparation.State != "running" || f.snapshot.Preparation.Total != 1 {
		t.Fatalf("approval did not leave preparation pending: %+v", f.snapshot)
	}
	noPreparationStart(t, f.starts)
	_, err := f.manager.Run(f.snapshot.Context, []string{"checker"}, false)
	code(t, err, "busy")
	if err := f.manager.StartPreparation(f.snapshot.Context, f.discover, f.publish); err != nil {
		t.Fatal(err)
	}
	process := nextStart(t, f.starts)
	if process.in.Directory != f.plan.Steps[0].Command.PreparationDirectory || !reflect.DeepEqual(process.in.Args, f.plan.Steps[0].Command.Args) {
		t.Fatalf("setup command changed: %+v", process.in)
	}
	code(t, f.manager.StartPreparation(f.snapshot.Context, f.discover, f.publish), "invalidArgument")
	if err := process.process.Terminate(); err != nil {
		t.Fatal(err)
	}
	waitPreparationWorkers(t, f.manager)
	snapshot, err := f.manager.GetSnapshot(f.snapshot.Context)
	if err != nil || snapshot.Preparation != nil || snapshot.Approval.State != "approved" || snapshot.Proposal.Fingerprint == initial || snapshot.Approval.Fingerprint != snapshot.Proposal.Fingerprint {
		t.Fatalf("prepared approval: %+v, %v", snapshot, err)
	}
	if f.validated.Load() != 1 || f.prepared.Load() != 1 || f.published.Load() != 1 {
		t.Fatalf("callbacks: validated=%d prepared=%d published=%d", f.validated.Load(), f.prepared.Load(), f.published.Load())
	}
	old, err := Fingerprint(f.root, f.oldTools)
	if err != nil || old != initial {
		t.Fatalf("original input changed during setup: %s, %v", old, err)
	}
	if _, err := f.manager.trust.Preflight(f.root, f.newTools); err != nil {
		t.Fatal("covered tools did not receive the carried approval", err)
	}
	noPreparationStart(t, f.starts)
}

func TestPreparationManagerRejectsUnreviewedChanges(t *testing.T) {
	for _, change := range []string{"noncovered tool", "old config bytes", "old executable bytes", "Prepared rejection"} {
		t.Run(change, func(t *testing.T) {
			f := newPreparationManagerFixture(t, nil)
			f.approve(t, "session")
			if change == "noncovered tool" {
				// Prefix matching must require a dash boundary.
				f.newTools[len(f.newTools)-1].ProviderID = "preparedx-checker"
			}
			if change == "Prepared rejection" {
				f.rejectPrepared = true
			}
			discover := func(root AuthorizedRoot) ([]Registration, error) {
				if change == "old config bytes" || change == "old executable bytes" {
					path := f.oldTools[0].Executable
					if change == "old config bytes" {
						path = filepath.Join(root.Project().Root, "config")
					}
					before, err := os.Stat(path)
					if err != nil {
						return nil, err
					}
					bytes, err := os.ReadFile(path)
					if err != nil {
						return nil, err
					}
					bytes[0] ^= 1
					if err := os.WriteFile(path, bytes, before.Mode()); err != nil {
						return nil, err
					}
					// Same size, permissions and mtime: only the content hash differs.
					if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
						return nil, err
					}
				}
				return f.discover(root)
			}
			if err := f.manager.StartPreparation(f.snapshot.Context, discover, f.publish); err != nil {
				t.Fatal(err)
			}
			_ = nextStart(t, f.starts).process.Terminate()
			waitPreparationWorkers(t, f.manager)
			snapshot, err := f.manager.GetSnapshot(f.snapshot.Context)
			if err != nil || snapshot.Preparation == nil || snapshot.Preparation.State != "failed" || snapshot.Proposal.Fingerprint != f.snapshot.Proposal.Fingerprint || f.published.Load() != 0 {
				t.Fatalf("changed input accepted: %+v, %v, published=%d", snapshot, err, f.published.Load())
			}
		})
	}
}

func TestPreparationManagerLateRediscoveryCannotActivate(t *testing.T) {
	for _, action := range []string{"revoke", "detach", "cancel", "reapprove"} {
		t.Run(action, func(t *testing.T) {
			f := newPreparationManagerFixture(t, store(t))
			f.approve(t, "persistent")
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			discover := func(root AuthorizedRoot) ([]Registration, error) {
				close(entered)
				<-release // Deliberately ignore cancellation and return late metadata.
				return f.discover(root)
			}
			if err := f.manager.StartPreparation(f.snapshot.Context, discover, f.publish); err != nil {
				t.Fatal(err)
			}
			_ = nextStart(t, f.starts).process.Terminate()
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				t.Fatal("metadata rediscovery did not start")
			}
			var err error
			switch action {
			case "revoke":
				f.snapshot, err = f.manager.Revoke(f.snapshot.Context)
			case "detach":
				err = f.manager.Detach(f.snapshot.Context)
			case "cancel":
				// The worker context is backend-owned; exercise cancellation directly
				// while rediscovery is in flight, after the last process completed.
				f.manager.mu.Lock()
				f.manager.attachments[f.snapshot.Context.AttachmentID].preparing()
				f.manager.mu.Unlock()
			case "reapprove":
				f.approve(t, "persistent")
			}
			if err != nil {
				t.Fatal(err)
			}
			unblock()
			waitPreparationWorkers(t, f.manager)
			if f.published.Load() != 0 {
				t.Fatal("late preparation published live metadata")
			}
			if action == "detach" {
				_, err := f.manager.GetSnapshot(f.snapshot.Context)
				code(t, err, "staleContext")
			} else {
				snapshot, err := f.manager.GetSnapshot(f.snapshot.Context)
				if err != nil {
					t.Fatal(err)
				}
				if action == "revoke" && (snapshot.Approval.State != "revoked" || snapshot.Preparation != nil) {
					t.Fatalf("revoked approval restored: %+v", snapshot)
				}
				if action != "revoke" && (snapshot.Proposal.Fingerprint != f.snapshot.Proposal.Fingerprint || snapshot.Preparation == nil) {
					t.Fatalf("late worker replaced current preparation: %+v", snapshot)
				}
			}
			// Read durable grants directly, so stale in-memory approval cannot
			// conceal a late worker rewriting or restoring persistent approval.
			fingerprint, err := Fingerprint(f.root, f.newTools)
			if err != nil {
				t.Fatal(err)
			}
			moved, err := hasGrant(f.manager.trust.store, f.root.Project(), fingerprint)
			if err != nil || moved {
				t.Fatalf("late worker rewrote the persistent approval: %v", err)
			}
			kept, err := hasGrant(f.manager.trust.store, f.root.Project(), f.snapshot.Proposal.Fingerprint)
			if err != nil || kept != (action != "revoke") {
				t.Fatalf("persistent approval after %s: %v %v", action, kept, err)
			}
		})
	}
}

func TestPreparationManagerPersistentAttachRemainsPending(t *testing.T) {
	grants := store(t)
	f := newPreparationManagerFixture(t, grants)
	f.approve(t, "persistent")
	if err := f.manager.Detach(f.snapshot.Context); err != nil {
		t.Fatal(err)
	}
	// A new connection must still prepare; the remembered grant alone must not
	// make saved or live checks executable before its worker finishes.
	trust := NewTrust(grants)
	starts := make(chan started, 1)
	manager, err := NewManager("remembered-preparation", trust, NewSupervisor(trust, SupervisorOptions{Start: fakeFactory(starts)}), ManagerLimits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	snapshot, err := manager.Attach(f.root, 1, f.registrations)
	if err != nil || snapshot.Approval.State != "approved" || snapshot.Approval.Mode != "persistent" || snapshot.Preparation == nil || snapshot.Preparation.State != "running" || snapshot.Preparation.Total != 1 {
		t.Fatalf("remembered attach bypassed preparation: %+v, %v", snapshot, err)
	}
	_, err = manager.Run(snapshot.Context, []string{"checker"}, false)
	code(t, err, "busy")
	if f.validated.Load() != 0 || f.prepared.Load() != 0 || f.published.Load() != 0 {
		t.Fatal("metadata-only attach ran a preparation callback")
	}
	noPreparationStart(t, starts)
}
