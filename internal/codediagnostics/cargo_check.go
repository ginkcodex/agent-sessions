package codediagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

const cargoCheckPolicy = "cargo-check-v1:workspace:offline:locked:json:saved-default-features-build-selected-targets"
const cargoCheckNotice = "Cargo check covers saved workspace files with default features and build-selected targets, not unsaved drafts or every feature/target. It runs approved build scripts and procedural macros and writes build/cache output. Offline prevents Cargo-managed downloads, not network access by project code; this is not a sandbox. Prepare missing dependencies and lockfiles separately. External changes are unwatched."

type CargoCheckPlan struct {
	root      AuthorizedRoot
	discovery *RustPlan
	commands  []Command
	tools     []ToolInput
	roots     []string
}

var _ CheckerAdapter = (*CargoCheckPlan)(nil)

func DiscoverCargoCheck(root AuthorizedRoot, reviewedPath []string, env RustEnvironment) (*CargoCheckPlan, error) {
	discovery, err := DiscoverRust(root, reviewedPath, env)
	if err != nil {
		return nil, err
	}
	return NewCargoCheckPlan(root, discovery)
}

// NewCargoCheckPlan reuses the metadata-only discovery shared with live analysis.
func NewCargoCheckPlan(root AuthorizedRoot, discovery *RustPlan) (*CargoCheckPlan, error) {
	if discovery == nil {
		return nil, failure("invalidArgument", "Rust discovery is required")
	}
	executable, ok := discovery.Cargo()
	if !ok || discovery.Availability("cargo-check") != "available" {
		return nil, failure(discovery.Availability("cargo-check"), discovery.Notice())
	}
	p := &CargoCheckPlan{root: root, discovery: discovery, roots: discovery.Roots(), tools: discovery.Tools("cargo-check")}
	if len(p.roots) == 0 {
		return nil, failure("unsupported", "No contained Cargo workspace was found")
	}
	for i, dir := range p.roots {
		c := Command{ProviderID: fmt.Sprintf("cargo-check-%d", i), Executable: executable, Directory: dir, Args: []string{"check", "--workspace", "--offline", "--locked", "--message-format=json"}, Env: discovery.Environment(), AdapterPolicy: cargoCheckPolicy, AcceptedExits: []int{0, 101}, ValidateStart: discovery.Unchanged}
		p.commands = append(p.commands, c)
		p.tools = append(p.tools, ToolInput{ProviderID: c.ProviderID, Executable: executable, ConfigPaths: discovery.ConfigPaths(), Policy: c.Policy()})
	}
	if _, err := Fingerprint(root, p.tools); err != nil {
		return nil, err
	}
	return p, nil
}
func (p *CargoCheckPlan) Tools() []ToolInput {
	out := slices.Clone(p.tools)
	for i := range out {
		out[i].ConfigPaths = slices.Clone(out[i].ConfigPaths)
	}
	return out
}
func (p *CargoCheckPlan) Run(ctx context.Context, s *Supervisor, runID string) CheckerResult {
	return p.RunApproved(ctx, s, runID, p.Tools())
}
func (p *CargoCheckPlan) RunApproved(ctx context.Context, s *Supervisor, runID string, approved []ToolInput) CheckerResult {
	out := CheckerResult{Diagnostics: []Diagnostic{}, Roots: slices.Clone(p.roots), Availability: "available", Phase: "idle", Freshness: "uncertain", Notice: cargoCheckNotice}
	fail := func(code string) CheckerResult {
		out.Replace = false
		out.Partial = true
		out.Phase = "failed"
		out.Freshness = "stale"
		out.Failure = &Error{code, "Cargo check did not complete; previous results must be retained stale. Prepare the workspace lockfile and dependency cache separately, then retry offline and locked."}
		if code == "canceled" {
			out.Phase = "canceled"
		}
		return out
	}
	if ctx == nil || s == nil || !bounded(runID, 200) {
		return fail("invalidArgument")
	}
	if p.discovery.Unchanged() != nil {
		return fail("approvalChanged")
	}
	var runs []*Run
	var parsed []cargoParsed
	for i, c := range p.commands {
		if ctx.Err() != nil {
			return fail("canceled")
		}
		// Revalidate between workspaces too: the first approved build can change config.
		if p.discovery.Unchanged() != nil {
			return fail("approvalChanged")
		}
		r, err := s.Submit(ctx, p.root, approved, c, fmt.Sprintf("%s-%d", runID, i), false)
		if err != nil {
			var e *Error
			if errors.As(err, &e) {
				return fail(e.Code)
			}
			return fail("toolFailed")
		}
		runs = append(runs, r)
		o := r.Result()
		if o.Kind != OutcomeAccepted || o.Partial || o.Truncated {
			r.ReportFailure(rejectedReason(o))
			out.Truncated = o.Truncated
			if o.Kind == OutcomeCanceled {
				return fail("canceled")
			}
			if o.Kind == OutcomeLimit {
				return fail("limitExceeded")
			}
			return fail("toolFailed")
		}
		// Cargo routinely writes progress and compiler summaries to stderr. It is not
		// parsed, surfaced, or itself evidence that an otherwise complete build failed.
		result, err := parseCargoCheck(p.root, c.Directory, o.Stdout, o.ExitCode, 10000-len(out.Diagnostics))
		if err != nil {
			r.ReportFailure("Checker output could not be parsed: " + err.Error())
			var e *Error
			if errors.As(err, &e) {
				out.Truncated = e.Code == "limitExceeded"
				return fail(e.Code)
			}
			return fail("toolFailed")
		}
		parsed = append(parsed, result)
		out.Diagnostics = append(out.Diagnostics, result.diagnostics...)
		if result.partial {
			out.Partial = true
			out.Notice = cargoCheckNotice + " Some external, generated, or unlocated messages were omitted."
		}
	}
	if p.discovery.Unchanged() != nil {
		return fail("approvalChanged")
	}
	for _, result := range parsed {
		if !result.validateSources(p.root) {
			return fail("resyncRequired")
		}
	}
	encoded, err := json.Marshal(out.Diagnostics)
	if err != nil || len(encoded) > 7<<20 {
		out.Truncated = true
		return fail("limitExceeded")
	}
	for _, r := range runs {
		if r.ValidatePublication() != nil {
			return fail("approvalChanged")
		}
	}
	out.Replace = true
	return out
}
