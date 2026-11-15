package codediagnostics

import (
	"context"
	"time"
)

const restartBackoff = time.Second

type liveRetry struct {
	generation string
	timer      <-chan time.Time
}

// liveSessionWantedLocked never revives detached, revoked or undisplayed work.
func (m *Manager) liveSessionWantedLocked(root AuthorizedRoot, cfg ServerConfig) bool {
	owner := DocumentOwnerIdentity{cfg.ProviderID, cfg.Policy()}
	for _, a := range m.attachments {
		if a.root.Project() != root.Project() || m.liveGate(a) != nil {
			continue
		}
		for _, d := range a.docs {
			routes := d.routes
			if len(routes) == 0 {
				routes = []*documentState{d}
			}
			for _, route := range routes {
				if route.owner == owner && !route.contextBlocked {
					return true
				}
			}
		}
	}
	return false
}

func (d *LiveDocuments) recoverSession(root AuthorizedRoot, cfg ServerConfig, info SessionInfo) {
	d.manager.mu.Lock()
	wanted := d.manager.liveSessionWantedLocked(root, cfg)
	d.manager.mu.Unlock()
	key := keyOf(root, cfg)
	d.mu.Lock()
	if !wanted || info.Phase != phaseFailed || !info.RetryAvailable {
		delete(d.retries, key)
		d.mu.Unlock()
		return
	}
	r, ok := d.retries[key]
	if !ok || r.generation != info.Generation {
		d.retries[key] = liveRetry{info.Generation, d.Sessions.options.After(restartBackoff << info.Restarts)}
		d.mu.Unlock()
		return
	}
	select {
	case <-r.timer:
		// Consume once per failed generation, including a refused preflight.
		r.timer = nil
		d.retries[key] = r
		d.mu.Unlock()
		d.manager.mu.Lock()
		if !d.manager.closed {
			d.manager.workers.Add(1)
			go func() {
				defer d.manager.workers.Done()
				d.manager.mu.Lock()
				defer d.manager.mu.Unlock()
				if d.Sessions.Info(root, cfg).Generation == info.Generation {
					_ = d.manager.retryLiveLocked(root, cfg, false)
				}
			}()
		}
		d.manager.mu.Unlock()
	default:
		d.mu.Unlock()
	}
}

// retryLive serializes recovery with document changes, revoke and detach. Only
// acknowledged displayed text is replayed; disk and failed sends are not trusted.
func (m *Manager) retryLiveLocked(root AuthorizedRoot, cfg ServerConfig, manual bool) error {
	if m.closed || (!manual && !m.liveSessionWantedLocked(root, cfg)) {
		return ErrSessionNotRetryable
	}
	d, ok := m.provider.(*LiveDocuments)
	if !ok {
		return ErrSessionNotRetryable
	}
	info, err := d.Sessions.retry(context.Background(), root, cfg, manual)
	if err != nil {
		return err
	}
	owner := DocumentOwnerIdentity{cfg.ProviderID, cfg.Policy()}
	for _, a := range m.attachments {
		if a.root.Project() != root.Project() || m.liveGate(a) != nil {
			continue
		}
		for id, doc := range a.docs {
			routes := doc.routes
			if len(routes) == 0 {
				routes = []*documentState{doc}
			}
			for _, route := range routes {
				if route.owner != owner || route.contextBlocked || route.broken || route.generation == info.Generation {
					continue
				}
				if err := m.validateDocument(root, route.path, route.owner, route.text); err != nil {
					route.contextBlocked = true
					continue
				}
				if err := m.docStep(context.Background(), a, route, DocumentStep{Generation: info.Generation, Kind: "open", Version: route.version, Text: route.text}); err != nil {
					route.broken = true
					return err
				}
				route.generation = info.Generation
				m.dropProviderDocumentResults(a, id, cfg.ProviderID)
			}
		}
		if p := provider(a, cfg.ProviderID); p != nil {
			p.Phase, p.Freshness, p.Failure = "idle", "uncertain", nil
			p.Notice = "Language session restarted; diagnostics cover synchronized opened documents only."
			m.commit(a)
		}
	}
	return nil
}

// RetryLive uses the existing explicit Run route without adding an RPC method.
func (m *Manager) RetryLive(c Context, id string) (bool, error) {
	m.mu.Lock()
	a, err := m.lookup(c)
	if err != nil {
		m.mu.Unlock()
		return false, err
	}
	p := provider(a, id)
	if p == nil || p.Coverage.Kind != "openedDocuments" {
		m.mu.Unlock()
		return false, nil
	}
	d, ok := m.provider.(*LiveDocuments)
	if !ok || m.liveGate(a) != nil || p.Availability != "available" || p.Phase != "failed" {
		m.mu.Unlock()
		return true, ErrSessionNotRetryable
	}
	defer m.mu.Unlock()
	root := a.root
	project, ok := d.project(root)
	if !ok {
		return true, ErrSessionNotRetryable
	}
	for _, cfg := range project.servers {
		if cfg.ProviderID == id && d.Sessions.Info(root, cfg).Phase == phaseFailed {
			if err := m.retryLiveLocked(root, cfg, true); err != nil {
				return true, err
			}
		}
	}
	return true, nil
}
