package codediagnostics

const partialRustNotice = "Some Rust diagnostics could not be verified; results are partial."

// RustBridge turns rust-analyzer notifications into owned document-set
// replacements using the shared LSP normalizer and manager publication guards.
// Only a matching publication version can certify current results; unversioned
// findings stay uncertain and unpositioned, and no push never means clean.
type RustBridge = PushBridge

func NewRustBridge(root AuthorizedRoot, m *Manager) *RustBridge {
	return newPushBridge(rustAnalyzerProviderID, root, m)
}
