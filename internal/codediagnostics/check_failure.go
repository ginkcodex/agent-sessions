package codediagnostics

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// maxReportOutput bounds each kept end (start and end) of a failed run's
// stdout and stderr in a CheckFailure.
const maxReportOutput = 8 << 10

// CheckFailure is the local log report of one failed checker or preparation
// run. Like SessionFailure it carries sanitized, bounded tool output, so it is
// for the owner's on-disk log only and must never reach a snapshot, event or
// the UI.
type CheckFailure struct {
	Project     Project
	ProviderID  string
	Executable  string
	Args        []string
	Directory   string // relative to the project root; empty is the root
	Preparation bool   // a setup step (downloads, toolchain installs), not a check
	Started     time.Time
	Finished    time.Time
	Kind        OutcomeKind
	// Reason is what the app concluded, e.g. "Checker output could not be parsed".
	Reason   string
	ExitCode int    // -1 when the process never exited on its own
	Launch   string // why the process could not be started; empty otherwise
	Stdout   ReportOutput
	Stderr   ReportOutput
}

// ReportOutput is the sanitized start and end of one output stream.
type ReportOutput struct {
	Head, Tail string
	Omitted    int  // raw bytes left out between Head and Tail
	Bytes      int  // captured bytes in total
	Truncated  bool // capture itself stopped at its limit
}

// BoundOutput keeps the sanitized start and end of b for a local log.
func BoundOutput(b []byte, truncated bool) ReportOutput {
	out := ReportOutput{Bytes: len(b), Truncated: truncated}
	if len(b) <= 2*maxReportOutput {
		out.Head = sanitizeText(strings.ToValidUTF8(string(b), ""), len(b))
		return out
	}
	out.Head = sanitizeText(strings.ToValidUTF8(string(b[:maxReportOutput]), ""), maxReportOutput)
	out.Tail = sanitizeText(strings.ToValidUTF8(string(b[len(b)-maxReportOutput:]), ""), maxReportOutput)
	out.Omitted = len(b) - 2*maxReportOutput
	return out
}

// ReportFailure logs a run the checker rejects although the supervisor
// accepted it: unparseable output, unexpected stderr, an unsupported version.
// Runs the supervisor did not accept are already logged, and canceled runs
// are not failures, so both are ignored here.
func (r *Run) ReportFailure(reason string) {
	<-r.done
	if r.outcome.Kind == OutcomeAccepted {
		r.supervisor.report(r, r.outcome, reason)
	}
}

// report hands the run to the owner's FailureLog, if any.
func (s *Supervisor) report(r *Run, o Outcome, reason string) {
	log := s.options.FailureLog
	if log == nil {
		return
	}
	c := r.command
	f := CheckFailure{
		Project: r.root.Project(), ProviderID: c.ProviderID, Executable: c.Executable,
		Args: slices.Clone(c.Args), Directory: c.Directory, Preparation: c.PreparationDirectory != "",
		Started: r.started, Finished: r.finishedAt, Kind: o.Kind, Reason: reason, ExitCode: o.ExitCode,
		Stdout: BoundOutput(o.Stdout, o.Truncated), Stderr: BoundOutput(o.Stderr, o.Truncated),
	}
	if r.launchErr != nil {
		f.Launch = r.launchErr.Error()
	}
	log(f)
}

// failureReason describes an outcome the supervisor itself did not accept.
func failureReason(o Outcome) string {
	if o.Notice != "" {
		return o.Notice
	}
	switch o.Kind {
	case OutcomeToolFailed:
		return "Checker exited with a status it does not accept"
	case OutcomeTimeout:
		return "Checker did not finish in time"
	case OutcomeLimit:
		return "Checker output exceeded the size limit"
	}
	return string(o.Kind)
}

// rejectedReason describes why a checker refused an outcome.
func rejectedReason(o Outcome) string {
	switch {
	case len(o.Stderr) > 0:
		return "Checker wrote to stderr"
	case o.ExitCode != 0:
		return "Checker exited with status " + strconv.Itoa(o.ExitCode)
	}
	return "Checker run was incomplete"
}
