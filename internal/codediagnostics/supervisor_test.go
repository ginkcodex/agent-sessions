package codediagnostics

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeProcess struct {
	finish chan struct{}
	once   sync.Once
	exit   int
	err    error
}

func newFakeProcess() *fakeProcess        { return &fakeProcess{finish: make(chan struct{})} }
func (p *fakeProcess) Wait() (int, error) { <-p.finish; return p.exit, p.err }
func (p *fakeProcess) Terminate() error   { p.once.Do(func() { close(p.finish) }); return nil }
func (p *fakeProcess) Close() error       { return nil }

type fakeTimer struct{ ch chan time.Time }

func (t *fakeTimer) Done() <-chan time.Time { return t.ch }
func (t *fakeTimer) Stop()                  {}

type started struct {
	in      Invocation
	process *fakeProcess
}

func fakeFactory(ch chan started) ProcessFactory {
	return func(in Invocation) (Process, error) {
		p := newFakeProcess()
		ch <- started{in, p}
		return p, nil
	}
}

func nextStart(t *testing.T, ch <-chan started) started {
	t.Helper()
	select {
	case in := <-ch:
		return in
	case <-time.After(10 * time.Second):
		t.Fatal("checker did not start")
		return started{}
	}
}

func result(t *testing.T, r *Run) Outcome {
	t.Helper()
	select {
	case <-r.Done():
		return r.Result()
	case <-time.After(10 * time.Second):
		t.Fatal("checker did not finish")
		return Outcome{}
	}
}

func approvedCommand(t *testing.T, tr *Trust) (AuthorizedRoot, []ToolInput, Command) {
	t.Helper()
	root, tools, _ := fixture(t)
	c := Command{ProviderID: "test", Executable: tools[0].Executable, Args: []string{"--fixed", "space ; $(not-a-shell)"}, Env: []string{"SYNTHETIC=value"}, AdapterPolicy: "test-v1", AcceptedExits: []int{0, 1}}
	tools[0].Policy = c.Policy()
	fp, err := Fingerprint(root, tools)
	if err != nil {
		t.Fatal(err)
	}
	approve(t, tr, root, tools, fp, "session")
	return root, tools, c
}

func submit(t *testing.T, s *Supervisor, root AuthorizedRoot, tools []ToolInput, c Command, id string) *Run {
	t.Helper()
	r, err := s.Submit(context.Background(), root, tools, c, id, false)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSupervisorConcurrencyAndCoalescing(t *testing.T) {
	tr := NewTrust(nil)
	starts := make(chan started, 8)
	s := NewSupervisor(tr, SupervisorOptions{Start: fakeFactory(starts)})
	defer s.Close()
	root, tools, c := approvedCommand(t, tr)
	r1 := submit(t, s, root, tools, c, "one")
	first := nextStart(t, starts)
	same := submit(t, s, root, tools, c, "coalesced")
	if same != r1 {
		t.Fatal("did not coalesce")
	}
	if first.in.Executable != c.Executable || first.in.Directory != root.Project().Root || !reflect.DeepEqual(first.in.Args, c.Args) || !reflect.DeepEqual(first.in.Env, c.environment()) {
		t.Fatal("invocation changed")
	}
	// Different provider, same project: queued, not concurrent.
	c2 := c
	c2.ProviderID = "other"
	tools2 := []ToolInput{{ProviderID: c2.ProviderID, Executable: c2.Executable, Policy: c2.Policy()}}
	fp, err := Fingerprint(root, tools2)
	if err != nil {
		t.Fatal(err)
	}
	// New approval deliberately invalidates the old lease; use both identities
	// from the beginning for the project serialization check below instead.
	approve(t, tr, root, tools2, fp, "session")
	if result(t, r1).Kind != OutcomeCanceled {
		t.Fatal("approval did not cancel")
	}
	r2 := submit(t, s, root, tools2, c2, "two")
	second := nextStart(t, starts)
	rootB, toolsB, cB := approvedCommand(t, tr)
	rB := submit(t, s, rootB, toolsB, cB, "B")
	third := nextStart(t, starts)
	rootC, toolsC, cC := approvedCommand(t, tr)
	rC := submit(t, s, rootC, toolsC, cC, "C")
	s.mu.Lock()
	if len(s.active) != 2 || len(s.queue) != 1 {
		t.Fatal("backend parallelism not bounded")
	}
	s.mu.Unlock()
	_ = second.process.Terminate()
	if result(t, r2).Kind != OutcomeAccepted {
		t.Fatal("accepted exit failed")
	}
	fourth := nextStart(t, starts)
	_ = third.process.Terminate()
	_ = fourth.process.Terminate()
	result(t, rB)
	result(t, rC)
}

func TestSupervisorProjectSerializationSupersessionAndQueue(t *testing.T) {
	tr := NewTrust(nil)
	root, tools, c := approvedCommand(t, tr)
	other := c
	other.ProviderID = "other"
	tools = append(tools, ToolInput{ProviderID: other.ProviderID, Executable: other.Executable, Policy: other.Policy()})
	fp, err := Fingerprint(root, tools)
	if err != nil {
		t.Fatal(err)
	}
	approve(t, tr, root, tools, fp, "session")
	starts := make(chan started, 8)
	s := NewSupervisor(tr, SupervisorOptions{Start: fakeFactory(starts), Pending: 1})
	defer s.Close()
	old := submit(t, s, root, tools, c, "old")
	nextStart(t, starts)
	queued := submit(t, s, root, tools, other, "queued")
	s.mu.Lock()
	if len(s.queue) != 1 || len(s.active) != 1 {
		t.Fatal("same project ran concurrently")
	}
	s.mu.Unlock()
	_, err = s.Submit(context.Background(), root, tools, c, "new", true)
	code(t, err, "busy")
	// Cancel queued work and let the worker remove it while the project is active.
	queued.Cancel()
	if result(t, queued).Kind != OutcomeCanceled {
		t.Fatal("queued cancellation")
	}
	replacement, err := s.Submit(context.Background(), root, tools, c, "new", true)
	if err != nil {
		t.Fatal(err)
	}
	if result(t, old).Kind != OutcomeCanceled {
		t.Fatal("supersession did not cancel")
	}
	code(t, old.ValidatePublication(), "staleContext")
	p := nextStart(t, starts)
	old.Cancel() // cannot cancel new work
	_ = p.process.Terminate()
	if result(t, replacement).Kind != OutcomeAccepted {
		t.Fatal("replacement canceled")
	}
	if err := replacement.ValidatePublication(); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s.Close()
	code(t, replacement.ValidatePublication(), "staleContext")
	_, err = s.Submit(context.Background(), root, tools, c, "closed", false)
	code(t, err, "disconnected")
}

func TestSupervisorTimeoutLimitsAndTrust(t *testing.T) {
	for _, test := range []string{"timeout", "capture", "message", "revoke", "disconnect", "cancel", "shutdown", "diagnostic", "toolFailure", "waitFailure", "launchFailure"} {
		t.Run(test, func(t *testing.T) {
			tr := NewTrust(nil)
			root, tools, c := approvedCommand(t, tr)
			starts := make(chan started, 1)
			timers := make(chan *fakeTimer, 1)
			factory := fakeFactory(starts)
			if test == "launchFailure" {
				factory = func(Invocation) (Process, error) { return nil, errors.New("private path") }
			}
			s := NewSupervisor(tr, SupervisorOptions{Start: factory, CaptureBytes: 20, MessageBytes: 8, NewTimer: func(time.Duration) Timer { timer := &fakeTimer{make(chan time.Time, 1)}; timers <- timer; return timer }})
			defer s.Close()
			r := submit(t, s, root, tools, c, test)
			timer := <-timers
			want := OutcomeCanceled
			if test != "launchFailure" {
				p := nextStart(t, starts)
				switch test {
				case "timeout":
					timer.ch <- time.Time{}
					want = OutcomeTimeout
				case "capture":
					_, _ = p.in.Stdout.Write([]byte("12345678\n12345678\n"))
					_, _ = p.in.Stderr.Write([]byte("abc"))
					want = OutcomeLimit
				case "message":
					_, _ = p.in.Stdout.Write([]byte("123456789"))
					want = OutcomeLimit
				case "revoke":
					if err := tr.Revoke(root); err != nil {
						t.Fatal(err)
					}
				case "disconnect":
					tr.Close()
				case "cancel":
					r.Cancel()
				case "shutdown":
					s.Close()
				case "diagnostic":
					p.process.exit = 1
					_ = p.process.Terminate()
					want = OutcomeAccepted
				case "toolFailure":
					p.process.exit = 2
					_ = p.process.Terminate()
					want = OutcomeToolFailed
				case "waitFailure":
					p.process.err = errors.New("secret output")
					_ = p.process.Terminate()
					want = OutcomeToolFailed
				}
			} else {
				want = OutcomeLaunchFailed
			}
			o := result(t, r)
			if o.Kind != want || o.FindingBudget != 10000 || len(o.Stdout)+len(o.Stderr) > 20 || (want != OutcomeAccepted && !o.Partial) {
				t.Fatalf("outcome: %+v, want %s", o, want)
			}
			if want == OutcomeLimit && !o.Truncated {
				t.Fatal("silent truncation")
			}
			if test == "revoke" || test == "disconnect" {
				if r.ValidatePublication() == nil {
					t.Fatal("published after trust cancellation")
				}
			}
		})
	}
}

func TestSupervisorPreexecutionRevalidation(t *testing.T) {
	tr := NewTrust(nil)
	root, tools, c := approvedCommand(t, tr)
	starts := make(chan started, 4)
	s := NewSupervisor(tr, SupervisorOptions{Start: fakeFactory(starts)})
	defer s.Close()
	// Occupy both workers so mutation occurs after preflight but before launch.
	a := submit(t, s, root, tools, c, "a")
	pa := nextStart(t, starts)
	rb, tb, cb := approvedCommand(t, tr)
	b := submit(t, s, rb, tb, cb, "b")
	pb := nextStart(t, starts)
	rc, tc, cc := approvedCommand(t, tr)
	pending := submit(t, s, rc, tc, cc, "pending")
	if err := os.WriteFile(filepath.Join(rc.Project().Root, "config"), []byte("changed synthetic config"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = pa.process.Terminate()
	result(t, a)
	if result(t, pending).Kind != OutcomeTrust {
		t.Fatal("changed config executed")
	}
	select {
	case <-starts:
		t.Fatal("failed validation spawned")
	default:
	}
	_ = pb.process.Terminate()
	result(t, b)
	bad := cc
	bad.Args = []string{"unapproved"}
	_, err := s.Submit(context.Background(), rc, tc, bad, "bad", false)
	code(t, err, "approvalChanged")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.Submit(ctx, root, tools, c, "canceled", false)
	code(t, err, "canceled")
}

func TestSupervisorValidateStartAtLaunch(t *testing.T) {
	tr := NewTrust(nil)
	root, tools, c := approvedCommand(t, tr)
	other := c
	other.ProviderID = "other"
	var mu sync.Mutex
	var startErr error
	calls := 0
	other.ValidateStart = func() error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return startErr
	}
	withoutCallback := other
	withoutCallback.ValidateStart = nil
	if other.Policy() != withoutCallback.Policy() {
		t.Fatal("callback changed the approved policy")
	}
	tools = append(tools, ToolInput{ProviderID: other.ProviderID, Executable: other.Executable, Policy: other.Policy()})
	fp, err := Fingerprint(root, tools)
	if err != nil {
		t.Fatal(err)
	}
	approve(t, tr, root, tools, fp, "session")
	starts := make(chan started, 4)
	s := NewSupervisor(tr, SupervisorOptions{Start: fakeFactory(starts)})
	defer s.Close()
	// Same project serializes, so the callback runs at launch, after Submit.
	first := submit(t, s, root, tools, c, "first")
	p := nextStart(t, starts)
	queued := submit(t, s, root, tools, other, "queued")
	mu.Lock()
	if calls != 0 {
		mu.Unlock()
		t.Fatal("start validation ran at submission, not launch")
	}
	startErr = failure("approvalChanged", "inputs changed after submission")
	mu.Unlock()
	_ = p.process.Terminate()
	result(t, first)
	if result(t, queued).Kind != OutcomeTrust {
		t.Fatal("failed start validation still executed")
	}
	select {
	case <-starts:
		t.Fatal("failed start validation spawned")
	default:
	}
	// A passing callback is not authority: the lease is still validated.
	mu.Lock()
	startErr = nil
	calls = 0
	mu.Unlock()
	second := submit(t, s, root, tools, c, "second")
	p = nextStart(t, starts)
	queued = submit(t, s, root, tools, other, "queued-again")
	if err := os.WriteFile(filepath.Join(root.Project().Root, "config"), []byte("changed synthetic config"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = p.process.Terminate()
	result(t, second)
	if result(t, queued).Kind != OutcomeTrust {
		t.Fatal("passing start validation bypassed fingerprint validation")
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("start validation calls %d", calls)
	}
	select {
	case <-starts:
		t.Fatal("changed fingerprint spawned")
	default:
	}
}

func TestCaptureExactAccounting(t *testing.T) {
	c := newCapture(5, 3)
	_, _ = c.writer(false).Write([]byte("abc\n"))
	_, _ = c.writer(true).Write([]byte("x"))
	out, stderr, truncated := c.result()
	if string(out) != "abc\n" || string(stderr) != "x" || truncated {
		t.Fatal("exact bound truncated")
	}
	_, _ = c.writer(true).Write([]byte("y"))
	_, _, truncated = c.result()
	if !truncated {
		t.Fatal("combined limit missing")
	}
}

func TestSupervisorFailureLog(t *testing.T) {
	tr := NewTrust(nil)
	starts := make(chan started, 8)
	reports := make(chan CheckFailure, 8)
	s := NewSupervisor(tr, SupervisorOptions{Start: fakeFactory(starts), FailureLog: func(f CheckFailure) { reports <- f }})
	defer s.Close()
	root, tools, c := approvedCommand(t, tr)
	none := func(what string) {
		t.Helper()
		select {
		case f := <-reports:
			t.Fatalf("%s reported %+v", what, f)
		default:
		}
	}

	// An exit status the command does not accept is logged with its output.
	r := submit(t, s, root, tools, c, "bad-exit")
	p := nextStart(t, starts)
	_, _ = io.WriteString(p.in.Stdout, "partial result\n")
	_, _ = io.WriteString(p.in.Stderr, "panic: \x1b[1mboom\n")
	p.process.exit = 2
	_ = p.process.Terminate()
	if result(t, r).Kind != OutcomeToolFailed {
		t.Fatal("not failed")
	}
	f := <-reports
	if f.ProviderID != "test" || f.Executable != c.Executable || !reflect.DeepEqual(f.Args, c.Args) || f.Kind != OutcomeToolFailed ||
		f.ExitCode != 2 || f.Reason != "Checker exited with a status it does not accept" || f.Stdout.Head != "partial result\n" ||
		f.Stderr.Head != "panic: [1mboom\n" || f.Started.IsZero() || f.Finished.Before(f.Started) || f.Preparation {
		t.Fatalf("%+v", f)
	}
	r.ReportFailure("again") // already logged by the supervisor
	none("rejecting a failed run")

	// An accepted run is logged only when the checker rejects it.
	r = submit(t, s, root, tools, c, "accepted")
	p = nextStart(t, starts)
	_ = p.process.Terminate()
	if result(t, r).Kind != OutcomeAccepted {
		t.Fatal("not accepted")
	}
	none("an accepted run")
	r.ReportFailure("Checker output could not be parsed")
	if f = <-reports; f.Kind != OutcomeAccepted || f.Reason != "Checker output could not be parsed" {
		t.Fatalf("%+v", f)
	}

	// Cancellation is not a failure.
	ctx, cancel := context.WithCancel(context.Background())
	r, err := s.Submit(ctx, root, tools, c, "canceled", false)
	if err != nil {
		t.Fatal(err)
	}
	nextStart(t, starts)
	cancel()
	if result(t, r).Kind != OutcomeCanceled {
		t.Fatal("not canceled")
	}
	r.ReportFailure("ignored")
	none("a canceled run")
}

func TestReportOutputKeepsBothEnds(t *testing.T) {
	small := BoundOutput([]byte("ok\n"), false)
	if small.Head != "ok\n" || small.Tail != "" || small.Bytes != 3 {
		t.Fatalf("%+v", small)
	}
	big := []byte("first line\n" + strings.Repeat("x", 3*maxReportOutput) + "last line\n")
	out := BoundOutput(big, true)
	if !strings.HasPrefix(out.Head, "first line\n") || !strings.HasSuffix(out.Tail, "xlast line\n") ||
		len(out.Head) != maxReportOutput || len(out.Tail) != maxReportOutput || out.Bytes != len(big) || out.Omitted != len(big)-2*maxReportOutput || !out.Truncated {
		t.Fatalf("head %d tail %d bytes %d", len(out.Head), len(out.Tail), out.Bytes)
	}
}
