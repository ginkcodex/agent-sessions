package codediagnostics

import (
	"path/filepath"
	"strings"
	"time"
)

const preparationTimeout = 15 * time.Minute

// PreparationPlan is backend-owned setup covered by the same exact approval as
// checker execution. It never accepts commands or output paths from a client.
// Each step is a fixed Command whose Policy() must equal the Policy of one of
// Tools, so the step is launched through the same Preflight/lease path as any
// other approved checker process.
//
// Validate re-enumerates immutable inputs while tolerating only planned outputs;
// it runs immediately before the first step. Prepared validates a full set of
// rediscovered registrations and returns nil only when the change from the
// pre-preparation discovery is exactly the planned outputs, so the manager can
// carry the approval forward without a second user review.
type PreparationPlan struct {
	Steps   []PreparationStep
	Tools   []ToolInput
	Outputs []string
	// Covers lists provider-ID prefixes whose tool identities may legitimately
	// change once the steps finish (for example the Rust toolchain). Every other
	// tool must be byte-identical after preparation or approval is not carried over.
	Covers   []string
	Validate func() error
	Prepared func([]Registration) error
}

type PreparationStep struct {
	Name    string
	Command Command
}

// collectPreparations gathers backend-owned setup plans from registrations.
// Plans are deduplicated by their step identity so one plan shared by a saved
// checker and its live server is approved and run once; tools are deduplicated
// by provider, policy and executable.
func collectPreparations(registrations []Registration) ([]PreparationPlan, []ToolInput) {
	var plans []PreparationPlan
	var tools []ToolInput
	seenPlan := map[string]bool{}
	seenTool := map[string]bool{}
	for _, r := range registrations {
		if r.Prepare == nil || len(r.Prepare.Steps) == 0 {
			continue
		}
		key := preparationIdentity(r.Prepare)
		if key == "" || seenPlan[key] {
			continue
		}
		seenPlan[key] = true
		plans = append(plans, *r.Prepare)
		for _, tool := range cloneTools(r.Prepare.Tools) {
			tk := tool.ProviderID + "\x00" + tool.Policy + "\x00" + tool.Executable
			if seenTool[tk] {
				continue
			}
			seenTool[tk] = true
			tools = append(tools, tool)
		}
	}
	return plans, tools
}

// preparationIdentity returns a stable key for a plan's exact step sequence so
// a shared plan registers once. An empty key rejects a malformed plan.
func preparationIdentity(p *PreparationPlan) string {
	if p == nil {
		return ""
	}
	parts := make([]string, 0, len(p.Steps))
	for _, s := range p.Steps {
		if !bounded(s.Name, 256) || !bounded(s.Command.ProviderID, 256) || !bounded(s.Command.Executable, 4096) || !filepath.IsAbs(s.Command.Executable) {
			return ""
		}
		parts = append(parts, s.Command.ProviderID+"\x00"+s.Command.Executable+"\x00"+s.Command.Policy())
	}
	return strings.Join(parts, "\x01")
}

// validatePreparation ensures every step is a fixed, approval-bound command and
// that each step's policy is present among the plan's tool identities. It is
// backend defense in depth, never approval authority (the lease is still
// validated independently before launch).
func validatePreparation(root AuthorizedRoot, p *PreparationPlan) error {
	if p == nil || len(p.Steps) == 0 || len(p.Steps) > 64 || len(p.Tools) == 0 || len(p.Tools) > maxProposalTools || len(p.Outputs) > 256 {
		return failure("invalidArgument", "Invalid preparation plan")
	}
	if preparationIdentity(p) == "" {
		return failure("invalidArgument", "Invalid preparation step identity")
	}
	byPolicy := map[string]bool{}
	for _, t := range p.Tools {
		if t.Policy != "" {
			byPolicy[t.Policy] = true
		}
	}
	seen := map[string]bool{}
	for _, s := range p.Steps {
		if seen[s.Name] || len(s.Command.AcceptedExits) == 0 || s.Command.AdapterPolicy == "" {
			return failure("invalidArgument", "Invalid preparation step")
		}
		seen[s.Name] = true
		if !byPolicy[s.Command.Policy()] {
			return failure("invalidArgument", "Preparation step does not match an approved tool policy")
		}
	}
	for _, o := range p.Outputs {
		if !filepath.IsAbs(o) || !bounded(o, 4096) || hasParent(o) {
			return failure("invalidArgument", "Preparation output must be an absolute path")
		}
	}
	// Preparation may install into host toolchain/cache directories (rustup and
	// cargo write to RUSTUP_HOME/CARGO_HOME), but it runs from an app-owned
	// directory outside the project to avoid implicit cwd-based project writes.
	// This is not a sandbox: adapters must separately prevent project writes (for
	// Rust dependency resolution, --locked is mandatory and has no fallback).
	for _, s := range p.Steps {
		if s.Command.PreparationDirectory == "" {
			return failure("invalidArgument", "Preparation step needs an app-owned working directory")
		}
		if _, err := runDirectory(root, s.Command); err != nil {
			return err
		}
	}
	return nil
}

func preparationStepCount(plans []PreparationPlan) int {
	n := 0
	for _, p := range plans {
		n += len(p.Steps)
	}
	return n
}
