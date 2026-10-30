package codediagnostics

import "errors"

// LiveNotice accepts only adapter-owned, path-free bounded notices. It never
// treats a malformed or rejected publication as an empty successful replacement.
func (m *Manager) LiveNotice(project Project, providerID, notice string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !bounded(notice, 2048) {
		return
	}
	for _, a := range m.attachments {
		if a.root.Project() != project || a.snapshot.Approval.State != "approved" {
			continue
		}
		for i := range a.snapshot.Providers {
			p := &a.snapshot.Providers[i]
			if p.ProviderID != providerID || p.Coverage.Kind != "openedDocuments" || p.Notice == notice {
				continue
			}
			p.Notice, p.Coverage.Partial = notice, true
			m.commit(a)
		}
	}
}

// LiveFailure marks a provider's failed session. cause is the session's bounded
// error (fixed text, never server output); others read as unavailable.
func (m *Manager) LiveFailure(project Project, providerID string, cause error) {
	reason := &Error{Code: "disconnected", Message: "Language session is unavailable"}
	var e *Error
	if errors.As(cause, &e) && bounded(e.Code, 64) && bounded(e.Message, 256) {
		reason = &Error{Code: e.Code, Message: e.Message}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.attachments {
		if a.root.Project() != project || a.snapshot.Approval.State != "approved" {
			continue
		}
		for i := range a.snapshot.Providers {
			p := &a.snapshot.Providers[i]
			if p.ProviderID != providerID || p.Coverage.Kind != "openedDocuments" || p.Phase == "failed" {
				continue
			}
			p.Phase, p.Freshness, p.Coverage.Partial = "failed", "stale", true
			p.Notice = "Language session failed; retained findings are stale. Reopen after explicit session recovery."
			p.Failure = &Error{Code: reason.Code, Message: reason.Message}
			for j := range a.snapshot.ResultSets {
				set := &a.snapshot.ResultSets[j]
				if set.ProviderID == providerID && set.Origin == "lsp" {
					for k := range set.Diagnostics {
						set.Diagnostics[k].Freshness = "stale"
					}
				}
			}
			m.commit(a)
		}
	}
}

func (m *Manager) idleDocuments(a *attachment) {
	if len(a.docs) == 0 {
		if p, ok := m.provider.(interface{ Idle(AuthorizedRoot) }); ok {
			p.Idle(a.root)
		}
	}
}
