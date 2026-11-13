package update

import (
	"context"
	"errors"
	"log"
	"sync"
)

// State is where an update is, as the UI sees it.
type State string

const (
	// StateIdle: nothing to show. No newer release, or a download that failed
	// and will be retried at the next check.
	StateIdle State = "idle"
	// StateDownloading: the new version is being fetched in the background.
	StateDownloading State = "downloading"
	// StateReady: the new version is downloaded and checked, and installs when
	// the app next quits.
	StateReady State = "ready"
	// StateManual: a newer release is out and this install can update itself,
	// but will not: the setting is off or downloading failed too often. The UI
	// shows the release page.
	StateManual State = "manual"
	// StateUnsupported: a newer release is out and this install cannot update
	// itself (Linux, a development build, a portable copy). The UI shows the
	// release page.
	StateUnsupported State = "unsupported"
)

// Status is the update state sent to the UI.
type Status struct {
	State State `json:"state"`
	// Version is the newer release's version; empty when idle.
	Version string `json:"version,omitempty"`
	// URL is its release page, for the states that send the user there.
	URL string `json:"url,omitempty"`
	// Supported says this install can update itself, so the setting means
	// something. It is filled in when the status is read.
	Supported bool `json:"supported"`
}

// Manager turns the result of a check into a downloaded, checked installer,
// and installs it when the app quits. One Manager serves every window of the
// process.
type Manager struct {
	// Store holds the downloads.
	Store *Store
	// Supported reports whether this install can replace itself. Nil means no.
	Supported func() bool
	// Auto reports the "Download updates automatically" setting. Nil means on.
	Auto func() bool
	// Prepare gets a downloaded version ready to install, such as staging it
	// beside the app, and runs before the version is called ready. An error
	// sends the user to the release page for that version. Nil means nothing
	// to prepare. It must stop when ctx ends.
	Prepare func(ctx context.Context, r Ready) error
	// Apply installs r once the app has quit, and starts the new version when
	// relaunch is set. It is called from Finish. Nil means no installer.
	Apply func(r Ready, relaunch bool) error

	mu      sync.Mutex
	status  Status
	last    *Info // the newest successful offer, to redo when the setting changes
	cancel  context.CancelFunc
	gen     int // identifies the current download
	subs    map[int]func(Status)
	nextSub int
	closed  bool
	wg      sync.WaitGroup
	// prepared and unusable are the versions Prepare accepted or refused
	// during this run, so a daily check does not repeat the work.
	prepared map[string]bool
	unusable map[string]bool

	emitMu sync.Mutex
}

// Status is the current state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	s := m.status
	m.mu.Unlock()
	return m.view(s)
}

// view fills in what is not stored. mu must not be held.
func (m *Manager) view(s Status) Status {
	if s.State == "" {
		s.State = StateIdle
	}
	s.Supported = m.supported()
	return s
}

// Subscribe calls fn with the new status whenever it changes, from another
// goroutine. fn always receives the latest status, so a burst of changes may
// be reported once. Call the result to stop.
func (m *Manager) Subscribe(fn func(Status)) (unsubscribe func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.subs == nil {
		m.subs = map[int]func(Status){}
	}
	id := m.nextSub
	m.nextSub++
	m.subs[id] = fn
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.subs, id)
	}
}

func (m *Manager) supported() bool { return m.Supported != nil && m.Supported() }
func (m *Manager) auto() bool      { return m.Auto == nil || m.Auto() }

// set records the status and tells subscribers if it changed. mu must not be
// held.
func (m *Manager) set(s Status) {
	m.mu.Lock()
	changed := m.status != s
	m.status = s
	m.mu.Unlock()
	if changed {
		m.notify()
	}
}

func (m *Manager) notify() {
	m.emitMu.Lock()
	defer m.emitMu.Unlock()
	m.mu.Lock()
	cur := m.status
	subs := make([]func(Status), 0, len(m.subs))
	for _, fn := range m.subs {
		subs = append(subs, fn)
	}
	m.mu.Unlock()
	cur = m.view(cur)
	for _, fn := range subs {
		fn(cur)
	}
}

// Offer takes the outcome of a successful check. It starts the download when
// one is due, drops downloads that are no longer the latest release, and
// updates the status. It does not wait for the download.
func (m *Manager) Offer(info Info) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	copied := info
	m.last = &copied
	downloading := m.cancel != nil && m.status.State == StateDownloading
	sameDownload := downloading && info.Release != nil && m.status.Version == info.Release.Version
	m.mu.Unlock()

	if sameDownload && m.supported() && m.auto() {
		return
	}

	if !info.Available {
		m.stop()
		m.reconcile("")
		m.set(Status{State: StateIdle})
		return
	}
	page := Status{Version: info.Latest, URL: info.URL}
	rel := info.Release
	if rel == nil || m.Store == nil || !m.supported() {
		m.stop()
		page.State = StateUnsupported
		m.set(page)
		return
	}

	// Anything but the latest release goes: older versions, and one that
	// GitHub no longer offers.
	m.stop()
	m.reconcile(rel.Version)

	if m.isUnusable(rel.Version) {
		page.State = StateManual
		m.set(page)
		return
	}
	if r, ok := m.Store.Ready(rel.Version); ok && r.File == rel.File && r.SHA256 == rel.SHA256 && m.isPrepared(rel.Version) {
		m.set(Status{State: StateReady, Version: r.Version, URL: info.URL})
		return
	}
	if m.Store.GaveUp(rel.Version) || !m.auto() {
		page.State = StateManual
		m.set(page)
		return
	}
	m.start(rel.Version, info.URL, func(ctx context.Context) (Ready, error) { return m.Store.Fetch(ctx, *rel) })
}

func (m *Manager) isPrepared(version string) bool {
	if m.Prepare == nil {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.prepared[version]
}

func (m *Manager) isUnusable(version string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unusable[version]
}

func (m *Manager) remember(set *map[string]bool, version string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if *set == nil {
		*set = map[string]bool{}
	}
	(*set)[version] = true
}

// Resume picks up a download an earlier run left ready: it is installed at
// the next quit without waiting for the day's check, which then confirms it
// or removes it. Anything else in the folder that is not newer than the
// running version is removed. A build without a release version does nothing.
func (m *Manager) Resume(current string) {
	if m.Store == nil || !Newer("999999.0.0", current) {
		return // not a release build
	}
	if r, ok := m.Store.Newest(); ok && Newer(r.Version, current) && m.supported() {
		page := ReleasesPage + "/tag/v" + r.Version
		if m.Prepare == nil {
			m.set(Status{State: StateReady, Version: r.Version, URL: page})
			return
		}
		// Ready once it has been prepared again, which checks what a
		// previous run staged.
		m.start(r.Version, page, func(context.Context) (Ready, error) { return r, nil })
		return
	}
	m.reconcile("")
}

// stop cancels the download in progress and waits for it to end, which
// releases the store.
func (m *Manager) stop() {
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.gen++
	m.mu.Unlock()
	if cancel != nil {
		cancel()
		m.wg.Wait()
	}
}

func (m *Manager) reconcile(keep string) {
	if m.Store == nil {
		return
	}
	if err := m.Store.Reconcile(keep); err != nil && !errors.Is(err, ErrBusy) {
		log.Printf("agent-sessions: update cleanup: %v", err)
	}
}

// start runs fetch, then Prepare, in the background and sets the status to
// downloading until they are done.
func (m *Manager) start(version, page string, fetch func(ctx context.Context) (Ready, error)) {
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		cancel()
		return
	}
	m.gen++
	gen := m.gen
	m.cancel = cancel
	m.wg.Add(1)
	m.mu.Unlock()
	m.set(Status{State: StateDownloading, Version: version, URL: page})

	go func() {
		defer m.wg.Done()
		defer cancel()
		r, err := fetch(ctx)
		prepareErr := error(nil)
		if err == nil && m.Prepare != nil {
			if prepareErr = m.Prepare(ctx, r); prepareErr == nil {
				m.remember(&m.prepared, version)
			}
		}

		m.mu.Lock()
		current := m.gen == gen
		if current {
			m.cancel = nil
		}
		m.mu.Unlock()
		if !current {
			return // replaced or stopped; whoever did it set the status
		}
		switch {
		case err == nil && prepareErr == nil:
			m.set(Status{State: StateReady, Version: r.Version, URL: page})
		case ctx.Err() != nil:
			m.set(Status{State: StateIdle})
		case err == nil:
			// The download is fine but this install cannot use it; trying
			// again would give the same answer.
			log.Printf("agent-sessions: update %s cannot be installed: %v", version, prepareErr)
			m.remember(&m.unusable, version)
			m.set(Status{State: StateManual, Version: version, URL: page})
		case m.Store.GaveUp(version):
			log.Printf("agent-sessions: update %s: %v; giving up", version, err)
			m.set(Status{State: StateManual, Version: version, URL: page})
		default:
			log.Printf("agent-sessions: update %s: %v; will retry at the next check", version, err)
			m.set(Status{State: StateIdle})
		}
	}()
}

// SettingsChanged redoes the last offer after the automatic-download setting
// changed: turning it off stops a download, turning it on starts one.
func (m *Manager) SettingsChanged() {
	m.mu.Lock()
	last := m.last
	m.mu.Unlock()
	if last == nil {
		return
	}
	info := *last
	if m.Status().State == StateDownloading && !m.auto() {
		// Offer would see the same download; stop it first.
		m.stop()
		m.set(Status{State: StateManual, Version: info.Latest, URL: info.URL})
		return
	}
	m.Offer(info)
}

// Pending returns the version that installs when the app quits. Only its size
// has been compared since it was downloaded; Finish hashes it before use.
func (m *Manager) Pending() (Ready, bool) {
	m.mu.Lock()
	s := m.status
	m.mu.Unlock()
	if s.State != StateReady || m.Store == nil || m.Apply == nil {
		return Ready{}, false
	}
	return m.Store.Ready(s.Version)
}

// Finish runs when the app is quitting, after its windows have closed: it
// stops any download and, if a version is ready, starts its installer.
// relaunch asks the installer to open the new version afterwards, as in a
// restart. The result says whether an installer took over, in which case the
// caller must not start the app again itself. A Manager takes no more offers
// after Finish.
func (m *Manager) Finish(relaunch bool) bool {
	r, ok := m.Pending()
	m.stop()
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	if !ok {
		return false
	}
	if r.Verify() != nil {
		log.Printf("agent-sessions: update %s changed on disk; not installing", r.Version)
		return false
	}
	if err := m.Apply(r, relaunch); err != nil {
		log.Printf("agent-sessions: install update %s: %v", r.Version, err)
		return false
	}
	return true
}
