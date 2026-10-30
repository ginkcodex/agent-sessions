package codediagnostics

import (
	"encoding/json"
	"strings"

	"github.com/ginkcode/agent-sessions/internal/managedtools"
)

const rustAnalyzerProviderID = "rust-analyzer"
const rustLivePolicy = "rust-live-v1:pinned-managed:offline:locked:revalidate-inputs:approved-build-scripts-proc-macros"
const rustLiveNotice = "Rust live analysis covers opened Rust documents with the reviewed workspace's default Cargo features and targets. Approved build scripts and procedural macros may execute and write build/cache output. Cargo is offline and locked; project code is not sandboxed. External changes are unwatched."
const rustLiveUnpinnedNotice = "Rust live analysis requires this app's pinned managed rust-analyzer; an unrelated installed server cannot enforce the reviewed loading policy. Saved Cargo checking remains independent."

// rustLiveOptions is passed before rust-analyzer starts loading the workspace.
// Verified against the pinned source: config.rs check_extra_args first clones
// cargo.extraArgs and then appends check.extraArgs. Supplying --locked in both
// would duplicate Cargo's clap flag, so check deliberately inherits it once.
// metadata filters cargo.extraArgs and needs its separate metadataExtraArgs.
const rustLiveOptions = `{"cargo":{"metadataExtraArgs":["--locked"],"extraArgs":["--locked"]},"check":{"extraArgs":[]}}`

func rustLiveRegistration(p *RustPlan) Registration {
	return rustLiveRegistrationWithPin(p, managedtools.IsPinnedRustAnalyzer)
}

// The predicate seam lets protocol tests use synthetic tool bytes, never an
// actual Rust toolchain or downloads. Production always uses the managed pin.
func rustLiveRegistrationWithPin(p *RustPlan, pinned func(string) bool) Registration {
	state := ProviderState{ProviderID: rustAnalyzerProviderID, Availability: p.Availability(rustAnalyzerProviderID), Phase: "idle", Freshness: "uncertain",
		Coverage:           Coverage{Kind: "openedDocuments", Roots: p.Roots(), ExternalChanges: CoverageExternalChanges},
		DocumentExtensions: []string{".rs"}, Notice: shorten(p.Notice(), 2048)}
	live := &LiveRegistration{State: state}
	registration := Registration{ProviderID: rustAnalyzerProviderID, Availability: state.Availability, Live: live}
	// The shared plan is attached to live status too; collectPreparations dedupes
	// it against the saved Cargo registration by exact step identity.
	registration.Prepare = p.Preparation()
	if state.Availability != "available" {
		return registration
	}
	unavailable := func(notice string) Registration {
		registration.Availability = "unsupported"
		live.State.Availability = "unsupported"
		live.State.Coverage.Partial = true
		live.State.Notice = notice
		return registration
	}
	if pinned == nil || !pinned(p.analyzer) {
		return unavailable(rustLiveUnpinnedNotice)
	}
	if len(p.roots) > maxLiveWorkspaces {
		return unavailable("Rust live workspace budget exceeded; review a scope with at most 16 Cargo roots.")
	}
	var toolchain []ToolInput
	for _, tool := range p.Tools(rustAnalyzerProviderID) {
		// ServerConfig binds the executable plus its loading options. Retain all
		// other reviewed toolchain/host inputs, without duplicating the analyzer.
		if tool.Executable != p.analyzer {
			toolchain = append(toolchain, tool)
		}
	}
	validate := func() error {
		if !pinned(p.analyzer) {
			return failure("approvalChanged", "Pinned Rust language server changed; review again")
		}
		return p.Unchanged()
	}
	live.State.Notice = rustLiveNotice
	for _, dir := range p.roots {
		covers := func(file string) bool {
			return ValidatePath(file) == nil && strings.HasSuffix(file, ".rs") && (dir == "" || strings.HasPrefix(file, dir+"/"))
		}
		options := json.RawMessage(rustLiveOptions)
		live.Workspaces = append(live.Workspaces, LiveWorkspace{
			Server: ServerConfig{
				ProviderID: rustAnalyzerProviderID, Executable: p.analyzer, Directory: dir,
				Env: p.Environment(), AdapterPolicy: rustLivePolicy,
				ConfigPaths: p.ConfigPaths(), Toolchain: cloneTools(toolchain),
				InitializationOptions: options,
				// RA asks for section "rust-analyzer"; its value must be exactly
				// the object supplied as initializationOptions.
				Settings:           json.RawMessage(`{"rust-analyzer":` + rustLiveOptions + `}`),
				DeclareDiagnostics: true, ValidateStart: validate,
			},
			Resolve:  func(file string) (string, bool) { return "rust", covers(file) },
			Covers:   covers,
			Validate: func(_, _ string) error { return validate() },
		})
	}
	return registration
}
