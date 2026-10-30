package codediagnostics

import "strings"

// pushSpec describes a provider whose diagnostics pushes need no normalizer of
// their own: which files it owns and what to say when results are partial.
type pushSpec struct {
	partial string
	owns    func(file string) bool
}

var pushSpecs = map[string]pushSpec{
	tsProviderID:           {partialTSNotice, func(file string) bool { _, ok := TSLanguageID(file); return ok }},
	ruffProviderID:         {partialRuffNotice, func(file string) bool { _, ok := PythonLanguageID(file); return ok }},
	pyrightProviderID:      {partialPyrightNotice, func(file string) bool { _, ok := PythonLanguageID(file); return ok }},
	rustAnalyzerProviderID: {partialRustNotice, func(file string) bool { return strings.HasSuffix(file, ".rs") }},
}

// PushBridge turns one provider's publishDiagnostics for the files it owns into
// document-set replacements (handleOwnedPush).
type PushBridge struct {
	root       AuthorizedRoot
	sink       goplsSink
	providerID string
	spec       pushSpec
	Notice     func(string)
}

func newPushBridge(providerID string, root AuthorizedRoot, m *Manager) *PushBridge {
	return &PushBridge{root: root, sink: m, providerID: providerID, spec: pushSpecs[providerID]}
}

func (b *PushBridge) note(s string) {
	if b.Notice != nil {
		b.Notice(s)
	}
}

// Handle must be quick and runs on the session goroutine.
func (b *PushBridge) Handle(n SessionNotification) {
	handleOwnedPush(b.root, b.sink, n, b.providerID, b.spec.partial, b.spec.owns, b.note)
}

// liveBridge returns the handler for a provider's notifications, or nil for a
// provider that has none.
func liveBridge(providerID string, root AuthorizedRoot, m *Manager, notice func(string)) func(SessionNotification) {
	switch providerID {
	case goplsProviderID:
		b := NewGoplsBridge(root, m)
		b.Notice = notice
		return b.Handle
	case svelteLSProviderID:
		b := NewSvelteBridge(root, m)
		b.Notice = notice
		return b.Handle
	}
	if _, ok := pushSpecs[providerID]; !ok {
		return nil
	}
	b := newPushBridge(providerID, root, m)
	b.Notice = notice
	return b.Handle
}
