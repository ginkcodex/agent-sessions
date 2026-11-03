package app

import (
	"errors"
	"sync"
)

// NotificationHub shares the desktop's one notification service among the
// app's windows. The service is a process singleton with a single click
// handler, so each window's App gets a view of the hub: the hub starts the
// service once, passes every click to every view (each App keeps only its
// own notifications' clicks) and releases the service only when the app
// ends.
type NotificationHub struct {
	n Notifier

	mu      sync.Mutex
	started bool
	closed  bool
	views   map[*notifierView]func(id string)
}

// NewNotificationHub wraps n, the desktop's notifications.
func NewNotificationHub(n Notifier) *NotificationHub {
	return &NotificationHub{n: n, views: map[*notifierView]func(string){}}
}

// View returns a Notifier for one window. Its Cleanup forgets the window's
// click handler and leaves the service running for the other windows.
func (h *NotificationHub) View() Notifier {
	return &notifierView{hub: h}
}

// Close releases the service. The shell calls it once, when the app ends.
func (h *NotificationHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	h.views = map[*notifierView]func(string){}
	if h.started {
		h.started = false
		h.n.Cleanup()
	}
}

func (h *NotificationHub) initialize() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.started {
		return nil
	}
	if h.closed {
		return errNotificationsClosed
	}
	if err := h.n.Initialize(); err != nil {
		return err
	}
	h.started = true
	h.n.OnClick(h.clicked)
	return nil
}

func (h *NotificationHub) clicked(id string) {
	h.mu.Lock()
	handlers := make([]func(string), 0, len(h.views))
	for _, fn := range h.views {
		handlers = append(handlers, fn)
	}
	h.mu.Unlock()
	for _, fn := range handlers {
		fn(id)
	}
}

var errNotificationsClosed = errors.New("the app is shutting down")

// notifierView is one window's handle on a NotificationHub.
type notifierView struct{ hub *NotificationHub }

func (v *notifierView) Available() bool           { return v.hub.n.Available() }
func (v *notifierView) Initialize() error         { return v.hub.initialize() }
func (v *notifierView) Authorized() (bool, error) { return v.hub.n.Authorized() }
func (v *notifierView) RequestAuthorization() (bool, error) {
	return v.hub.n.RequestAuthorization()
}
func (v *notifierView) Send(id, title, body string) error { return v.hub.n.Send(id, title, body) }
func (v *notifierView) Remove(id string) error            { return v.hub.n.Remove(id) }

func (v *notifierView) OnClick(fn func(id string)) {
	v.hub.mu.Lock()
	defer v.hub.mu.Unlock()
	if !v.hub.closed {
		v.hub.views[v] = fn
	}
}

func (v *notifierView) Cleanup() {
	v.hub.mu.Lock()
	defer v.hub.mu.Unlock()
	delete(v.hub.views, v)
}
