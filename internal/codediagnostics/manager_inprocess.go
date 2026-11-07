package codediagnostics

import (
	"slices"
	"strings"
)

// InProcessApproved reports whether an in-process provider may analyze for this
// exact context now: the attachment and generation are current, the provider is
// registered as in-process, and project approval is current.
func (m *Manager) InProcessApproved(c Context, id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, err := m.lookup(c)
	if err != nil || !a.inProcess[id] || a.snapshot.Approval.State != "approved" {
		return false
	}
	p := provider(a, id)
	return p != nil && p.Availability == "available"
}

// PublishInProcess replaces an in-process provider's complete project result.
// It is rejected for stale contexts/generations, revoked or unapproved
// attachments and non-in-process providers, so a late scan can never resurrect
// findings after revoke, detach, reapproval or close. externalChanges is the
// caller's honest watcher status ("watching" only while its watcher is live).
func (m *Manager) PublishInProcess(c Context, id string, r CheckerResult, externalChanges string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, err := m.lookup(c)
	if err != nil {
		return err
	}
	if !a.inProcess[id] {
		return failure("invalidArgument", "Provider is not an in-process analyzer")
	}
	if a.snapshot.Approval.State != "approved" {
		return failure("approvalRequired", "Explicit project approval is required")
	}
	if externalChanges != "watching" && externalChanges != "unwatched" {
		return failure("invalidArgument", "Invalid external change status")
	}
	m.apply(a, id, opaqueID(), r)
	p := provider(a, id)
	p.Coverage.ExternalChanges = externalChanges
	if externalChanges != "watching" {
		p.Coverage.Partial = true
	}
	m.commit(a)
	return nil
}

// InProcessStatus marks an in-process provider's retained findings stale after
// its watcher stopped or failed, without dropping them.
func (m *Manager) InProcessStatus(c Context, id, notice string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, err := m.lookup(c)
	if err != nil {
		return err
	}
	if !a.inProcess[id] {
		return failure("invalidArgument", "Provider is not an in-process analyzer")
	}
	p := provider(a, id)
	m.stale(a, id, "failed")
	p.Coverage.ExternalChanges = "unwatched"
	p.Notice = shorten(notice, 2048)
	p.Failure = &Error{"toolFailed", "Project watching stopped; retained findings are stale"}
	m.commit(a)
	return nil
}

// LiveWatched reports whether the project watcher has saved files to report
// for this exact context: it is current and approved, and an available live
// server takes saved-file transitions (watchedFileProviders).
func (m *Manager) LiveWatched(c Context) bool {
	return len(m.liveWatch(c, false)) > 0
}

// liveWatch lists the approved, available live servers told of saved-file
// transitions. gopls is left out while go-syntax analyzes the Go files, since
// those transitions come from that analysis.
func (m *Manager) liveWatch(c Context, goSyntax bool) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, err := m.lookup(c)
	if err != nil || a.snapshot.Approval.State != "approved" {
		return nil
	}
	var out []string
	for _, p := range a.snapshot.Providers {
		if _, ok := watchedFileProviders[p.ProviderID]; ok && p.Availability == "available" && p.Coverage.Kind == "openedDocuments" && !(goSyntax && p.ProviderID == goplsProviderID) {
			out = append(out, p.ProviderID)
		}
	}
	slices.Sort(out)
	return out
}

// liveWatchNote adds a watcher note to live providers' notices, once, and
// marks their coverage partial: some saved changes do not reach them.
func (m *Manager) liveWatchNote(c Context, providers []string, note string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, err := m.lookup(c)
	if err != nil || a.snapshot.Approval.State != "approved" {
		return
	}
	changed := false
	for i := range a.snapshot.Providers {
		p := &a.snapshot.Providers[i]
		if !slices.Contains(providers, p.ProviderID) || strings.Contains(p.Notice, note) {
			continue
		}
		p.Notice = shorten(strings.TrimSpace(p.Notice+" "+note), 2048)
		p.Coverage.Partial = true
		changed = true
	}
	if changed {
		m.commit(a)
	}
}
