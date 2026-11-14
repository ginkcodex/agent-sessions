package remote

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ginkcode/agent-sessions/internal/rpc"
)

// HostProbe reports system details of a remote host.
type HostProbe struct {
	OS   string // "linux" or "darwin"
	Arch string // "amd64" or "arm64"
	Home string // remote user $HOME
	// Installed lists the server builds of this app version on the host
	// ("<version>-<sha8>" directory names), newest first.
	Installed []string
}

// maxInstalled caps the builds a probe reads, against a hostile listing.
const maxInstalled = 64

// Has reports whether the build dir tag is installed.
func (p *HostProbe) Has(tag string) bool {
	return slices.Contains(p.Installed, tag)
}

// Sentinel errors for probe failures.
var (
	ErrUnsupportedOS   = errors.New("unsupported remote operating system")
	ErrUnsupportedArch = errors.New("unsupported remote architecture")
)

// NormalizeOS converts uname -s output to standard Go GOOS ("linux" or "darwin").
func NormalizeOS(s string) (string, error) {
	val := strings.ToLower(strings.TrimSpace(s))
	switch val {
	case "linux":
		return "linux", nil
	case "darwin":
		return "darwin", nil
	default:
		return "", fmt.Errorf("%w: %s (only Linux and macOS supported)", ErrUnsupportedOS, s)
	}
}

// NormalizeArch converts uname -m output to standard Go GOARCH ("amd64" or "arm64").
func NormalizeArch(m string) (string, error) {
	val := strings.ToLower(strings.TrimSpace(m))
	switch val {
	case "x86_64", "amd64":
		return "amd64", nil
	case "aarch64", "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("%w: %s (only x86_64/amd64 and aarch64/arm64 supported)", ErrUnsupportedArch, m)
	}
}

var probeTimeout = 45 * time.Second // variable so tests can shorten automatic probes

// ProbeHost probes a remote machine via SSH or WSL to detect OS, architecture, $HOME,
// and which server builds of appVersion are installed.
func ProbeHost(ctx context.Context, alias string, opts SSHOptions, appVersion string) (*HostProbe, error) {
	var timeout time.Duration
	if _, wsl := ParseWSLTarget(alias); opts.Automatic || wsl || !SupportsSSHAskpass() {
		timeout = probeTimeout
	}
	return probeHost(ctx, alias, opts, appVersion, timeout)
}

func probeHost(ctx context.Context, alias string, opts SSHOptions, appVersion string, timeout time.Duration) (*HostProbe, error) {
	parent := ctx
	// Only automatic Unix SSH probes pause their budget for credentials.
	// Windows and WSL retain their unconditional wall-clock deadline.
	if timeout > 0 {
		var broker *AskpassBroker
		if _, wsl := ParseWSLTarget(alias); opts.Automatic && !wsl && SupportsSSHAskpass() {
			broker = opts.AskpassBroker
		}
		var cancel context.CancelFunc
		ctx, cancel = probeBudget(ctx, timeout, broker)
		defer cancel()
	}
	opts.NoTTY = true
	nonce := rpc.GenerateNonce()
	preface := rpc.FormatPreface(nonce)

	remoteScript := probeScript(preface, appVersion)

	// In the login environment, for PATH and the agents' variables.
	cmd, err := BuildHostCmd(ctx, alias, LoginCommand(remoteScript), opts)
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	defer stdout.Close()
	var stderrBuf sshStderr
	cmd.Stderr = &stderrBuf
	// Killing ssh alone need not close a ProxyCommand child's copy of stdout.
	// Closing our read end also interrupts a stalled preface/output scan.
	stopClose := context.AfterFunc(ctx, func() { _ = stdout.Close() })
	defer stopClose()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start probe: %w", err)
	}

	// Wait for nonce preface (skipping login shell banners up to 64 KiB).
	r, err := rpc.WaitForPreface(ctx, stdout, nonce)
	if err != nil {
		// Give an exiting ssh time to preserve its status (not a forced kill),
		// but kill a malformed/stalled producer that has not exited on its own.
		kill := time.AfterFunc(250*time.Millisecond, func() { _ = cmd.Process.Kill() })
		waitErr := cmd.Wait()
		kill.Stop()
		if ctx.Err() != nil {
			return nil, sshFailure(probeContextError(parent, ctx, timeout), stderrBuf.String())
		}
		if waitErr != nil {
			return nil, fmt.Errorf("probe: %w", sshFailure(waitErr, stderrBuf.String()))
		}
		return nil, fmt.Errorf("probe handshake: %w", sshFailure(err, stderrBuf.String()))
	}

	scanner := bufio.NewScanner(r)
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "END" {
			break
		}
		if line != "" {
			lines = append(lines, line)
		}
		if len(lines) >= 3+maxInstalled {
			break
		}
	}

	// Stop reading before Wait, which closes StdoutPipe. Any later output is
	// outside the bounded probe response and must not stall the child.
	_ = stdout.Close()
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, sshFailure(probeContextError(parent, ctx, timeout), stderrBuf.String())
	}
	if waitErr != nil {
		return nil, fmt.Errorf("probe: %w", sshFailure(waitErr, stderrBuf.String()))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("probe output: %w", err)
	}
	if len(lines) < 3 {
		return nil, fmt.Errorf("probe output truncated (got %d lines, want at least 3)", len(lines))
	}

	goOS, err := NormalizeOS(lines[0])
	if err != nil {
		return nil, err
	}
	goArch, err := NormalizeArch(lines[1])
	if err != nil {
		return nil, err
	}
	home := lines[2]

	probe := &HostProbe{
		OS:   goOS,
		Arch: goArch,
		Home: home,
	}
	for _, line := range lines[3:] {
		tag, ok := strings.CutPrefix(line, "INSTALLED:")
		if ok && isBuildTag(appVersion, tag) {
			probe.Installed = append(probe.Installed, tag)
		}
	}
	return probe, nil
}

// probeBudget counts only time without an open prompt. The broker invokes
// transitions synchronously, so answering immediately cannot lose a pause.
func probeBudget(parent context.Context, limit time.Duration, broker *AskpassBroker) (context.Context, context.CancelFunc) {
	if broker == nil {
		return context.WithTimeout(parent, limit)
	}
	ctx, cancel := context.WithCancelCause(parent)
	var mu sync.Mutex
	remaining, started := limit, time.Now()
	paused, closed := false, false
	var timer *time.Timer
	expire := func() {
		mu.Lock()
		defer mu.Unlock()
		if closed || paused {
			return
		}
		// A stopped timer's callback may already be waiting for the lock.
		// Recheck elapsed active time rather than expiring a resumed budget.
		left := remaining - time.Since(started)
		if left > 0 {
			timer.Reset(left)
			return
		}
		closed = true
		cancel(context.DeadlineExceeded)
	}
	mu.Lock()
	timer = time.AfterFunc(remaining, expire)
	mu.Unlock()
	unsubscribe := broker.observePrompts(func(open bool) {
		mu.Lock()
		defer mu.Unlock()
		if closed || open == paused {
			return
		}
		if open {
			remaining -= time.Since(started)
			timer.Stop()
		} else {
			started = time.Now()
			timer.Reset(remaining)
		}
		paused = open
	})
	return ctx, func() {
		unsubscribe()
		mu.Lock()
		closed = true
		timer.Stop()
		mu.Unlock()
		cancel(context.Canceled)
	}
}

func probeContextError(parent, run context.Context, timeout time.Duration) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	return fmt.Errorf("probe timed out after %v: %w", timeout, context.Cause(run))
}

// isBuildTag reports whether tag is a build dir of appVersion:
// "<version>-" followed by exactly 8 lowercase hex digits. The prefix alone
// also matches other versions such as "<version>-rc1-…".
func isBuildTag(appVersion, tag string) bool {
	sum, ok := strings.CutPrefix(tag, appVersion+"-")
	if !ok || len(sum) != 8 {
		return false
	}
	for _, c := range sum {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// probeScript prints the preface, uname -s, uname -m, $HOME, one
// "INSTALLED:<dir>" line per server build of appVersion (newest first), and
// END. Every build of the version is listed, not just one, so the client can
// pick the one matching its own bundle: two builds of one version (a
// rebuild, or clients built apart) must not run each other's server.
func probeScript(preface, appVersion string) string {
	return fmt.Sprintf(
		`echo %[1]s; uname -s; uname -m; echo "$HOME"; `+
			`ls -1dt "$HOME"%[2]s%[3]s* 2>/dev/null | while IFS= read -r d; do `+
			`if [ -x "$d/agent-sessions-cli" ]; then echo "INSTALLED:${d##*/}"; fi; done; echo END`,
		QuotePOSIX(preface),
		QuotePOSIX(serverRoot),
		QuotePOSIX(appVersion+"-"),
	)
}
