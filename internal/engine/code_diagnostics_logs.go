package engine

// Local logs for failed code checks, like an editor's language server output
// panel kept on disk. Each failed language server, checker run, setup step or
// managed tool installation writes one owner-only file: what ran, how it
// ended, and the sanitized, bounded start and end of its output. The
// environment is never written. Logs reach neither events nor the network;
// Settings only reports their count and size, and deletes them.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	cd "github.com/ginkcode/agent-sessions/internal/codediagnostics"
	mt "github.com/ginkcode/agent-sessions/internal/managedtools"
)

const (
	checkLogDir  = "code-diagnostics/logs"
	maxCheckLogs = 50
)

// CodeCheckLogsInfo describes the code check logs kept in the data directory.
type CodeCheckLogsInfo struct {
	Dir   string `json:"dir"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
}

// checkLogData is the data directory logs go under; tests replace it so
// engines without a configured data directory never write to the real one.
var checkLogData = func(e *Engine) (string, error) {
	roots, err := e.appRoots()
	return roots.Data, err
}

// checkLogs serializes writing, pruning and clearing one host's logs.
type checkLogs struct {
	mu  sync.Mutex
	dir string
}

// codeCheckLogs returns this engine's logs. Callers may hold e.mu.
func (e *Engine) codeCheckLogs() (*checkLogs, error) {
	e.logsMu.Lock()
	defer e.logsMu.Unlock()
	if e.logs == nil {
		data, err := checkLogData(e)
		if err != nil {
			return nil, err
		}
		if !filepath.IsAbs(data) {
			return nil, diagnosticError("unsupported", "App directories unavailable")
		}
		e.logs = &checkLogs{dir: filepath.Join(data, filepath.FromSlash(checkLogDir))}
	}
	return e.logs, nil
}

// CodeCheckLogs reports the code check logs on disk.
func (e *Engine) CodeCheckLogs(_ context.Context) (CodeCheckLogsInfo, error) {
	l, err := e.codeCheckLogs()
	if err != nil {
		return CodeCheckLogsInfo{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.usage(), nil
}

// ClearCodeCheckLogs deletes the code check logs and reports what remains.
func (e *Engine) ClearCodeCheckLogs(_ context.Context) (CodeCheckLogsInfo, error) {
	l, err := e.codeCheckLogs()
	if err != nil {
		return CodeCheckLogsInfo{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var first error
	for _, name := range l.names() {
		if err := os.Remove(filepath.Join(l.dir, name)); err != nil && !os.IsNotExist(err) && first == nil {
			first = err
		}
	}
	return l.usage(), first
}

// names lists the regular log files, oldest first: names start with a UTC
// timestamp, so name order is age order.
func (l *checkLogs) names() []string {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil
	}
	var logs []string
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".log") {
			logs = append(logs, e.Name())
		}
	}
	sort.Strings(logs)
	return logs
}

func (l *checkLogs) usage() CodeCheckLogsInfo {
	info := CodeCheckLogsInfo{Dir: l.dir}
	for _, name := range l.names() {
		if st, err := os.Lstat(filepath.Join(l.dir, name)); err == nil && st.Mode().IsRegular() {
			info.Files++
			info.Bytes += st.Size()
		}
	}
	return info
}

func (l *checkLogs) session(r cd.SessionFailure) {
	l.write(r.Failed, r.ProviderID, "server", formatSessionLog(r))
}

func (l *checkLogs) check(r cd.CheckFailure) {
	at, kind := r.Finished, "check"
	if at.IsZero() {
		at = time.Now()
	}
	if r.Preparation {
		kind = "setup"
	}
	l.write(at, r.ProviderID, kind, formatCheckLog(r))
}

func (l *checkLogs) install(r mt.InstallReport) {
	l.write(r.Failed, r.Tool, "install", formatInstallLog(r))
}

// write is best effort: a log that cannot be written must not affect the
// server, run or installation that failed.
func (l *checkLogs) write(at time.Time, name, kind, text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return
	}
	base := at.UTC().Format("20060102T150405.000Z") + "-" + checkLogName(name) + "-" + kind
	for i := 1; i <= 8; i++ {
		name := base + ".log"
		if i > 1 {
			name = base + "-" + strconv.Itoa(i) + ".log"
		}
		f, err := os.OpenFile(filepath.Join(l.dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return
		}
		_, _ = f.WriteString(text)
		_ = f.Close()
		break
	}
	// Keep the newest maxCheckLogs.
	logs := l.names()
	for len(logs) > maxCheckLogs {
		_ = os.Remove(filepath.Join(l.dir, logs[0]))
		logs = logs[1:]
	}
}

func checkLogName(name string) string {
	b := []byte(name)
	for i, c := range b {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			b[i] = '_'
		}
	}
	if len(b) == 0 || len(b) > 64 {
		return "tool"
	}
	return string(b)
}

type logText struct{ strings.Builder }

func (b *logText) line(key, value string) {
	if value == "" {
		value = "(none)"
	}
	fmt.Fprintf(b, "%-10s %s\n", key+":", value)
}

func (b *logText) command(executable string, args []string) {
	if executable == "" {
		return
	}
	command := []string{strconv.Quote(executable)}
	for _, a := range args {
		command = append(command, strconv.Quote(a))
	}
	b.line("command", strings.Join(command, " "))
}

func (b *logText) project(p cd.Project, dir string) {
	b.line("project", p.HostID+" "+p.Root)
	if dir == "" {
		dir = "."
	}
	b.line("directory", dir)
}

// output writes one stream's size line; body writes its kept start and end.
func (b *logText) output(name string, bytes int64, capped bool) {
	size := strconv.FormatInt(bytes, 10) + " bytes"
	if capped {
		size += " (capture limit reached)"
	}
	b.line(name, size)
}

func (b *logText) body(name, head string, omitted int64, tail string) {
	if head == "" && tail == "" {
		return
	}
	b.WriteString("\n--- " + name + " ---\n")
	b.WriteString(head)
	if tail != "" {
		if omitted > 0 {
			fmt.Fprintf(b, "\n--- %d bytes omitted ---\n", omitted)
		}
		b.WriteString(tail)
	}
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}
	b.WriteString("--- end ---\n")
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func since(start, end time.Time) string {
	if start.IsZero() || end.IsZero() {
		return ""
	}
	return end.Sub(start).Round(time.Millisecond).String()
}

func formatSessionLog(r cd.SessionFailure) string {
	var b logText
	b.WriteString("Language server failure\n\n")
	b.line("time", r.Failed.Format(time.RFC3339Nano))
	b.line("provider", r.ProviderID)
	b.project(r.Project, r.Directory)
	b.command(r.Executable, r.Args)
	phase := r.Phase
	if up := since(r.Started, r.Failed); up != "" {
		phase += " (up " + up + ")"
	}
	b.line("phase", phase)
	b.line("restarts", strconv.Itoa(r.Restarts))
	b.line("cause", errText(r.Cause))
	b.line("transport", errText(r.Transport))
	b.line("exit", r.Exit)
	b.output("stderr", r.StderrBytes, false)
	omitted := int64(0)
	if r.StderrTruncated {
		omitted = r.StderrBytes - int64(len(r.Stderr)) - int64(len(r.StderrTail))
	}
	b.body("stderr", r.Stderr, omitted, r.StderrTail)
	return b.String()
}

func formatCheckLog(r cd.CheckFailure) string {
	var b logText
	if r.Preparation {
		b.WriteString("Code check setup failure\n\n")
	} else {
		b.WriteString("Code check failure\n\n")
	}
	b.line("time", r.Started.Format(time.RFC3339Nano))
	b.line("provider", r.ProviderID)
	b.project(r.Project, r.Directory)
	b.command(r.Executable, r.Args)
	b.line("outcome", string(r.Kind))
	b.line("reason", r.Reason)
	exit := ""
	if r.ExitCode >= 0 && r.Launch == "" {
		exit = "status " + strconv.Itoa(r.ExitCode)
	}
	b.line("exit", exit)
	if r.Launch != "" {
		b.line("launch", r.Launch)
	}
	b.line("duration", since(r.Started, r.Finished))
	b.output("stdout", int64(r.Stdout.Bytes), r.Stdout.Truncated)
	b.output("stderr", int64(r.Stderr.Bytes), r.Stderr.Truncated)
	b.body("stdout", r.Stdout.Head, int64(r.Stdout.Omitted), r.Stdout.Tail)
	b.body("stderr", r.Stderr.Head, int64(r.Stderr.Omitted), r.Stderr.Tail)
	return b.String()
}

func formatInstallLog(r mt.InstallReport) string {
	var b logText
	b.WriteString("Managed tool installation failure\n\n")
	b.line("time", r.Started.Format(time.RFC3339Nano))
	b.line("tool", strings.TrimSpace(r.Tool+" "+r.Version))
	b.line("step", r.Step)
	b.line("cause", errText(r.Err))
	b.line("duration", since(r.Started, r.Failed))
	if len(r.Command) > 0 {
		b.command(r.Command[0], r.Command[1:])
		out := cd.BoundOutput(r.Output, r.OutputTruncated)
		b.output("output", int64(out.Bytes), out.Truncated)
		b.body("output", out.Head, int64(out.Omitted), out.Tail)
	}
	return b.String()
}
