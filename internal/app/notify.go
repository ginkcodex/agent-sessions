package app

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// notifyOpenEvent carries the tab of an agent alert the user clicked.
const notifyOpenEvent = "notify:open"

// NotificationStatus says whether agent alerts reach the desktop's
// notifications, and when not, why and what would turn them on.
type NotificationStatus struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// AgentNotification is an alert about an agent tab: it needs the user or
// finished its turn. Tab is the panel tab it is about.
type AgentNotification struct {
	Tab   string `json:"tab"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Notifier is what agent alerts need from the desktop's notifications: the
// notification center on macOS, D-Bus on Linux and toasts on Windows. The
// desktop shell provides it in Desktop.Notifications; tests replace it.
type Notifier interface {
	Available() bool
	Initialize() error
	Authorized() (bool, error)
	RequestAuthorization() (bool, error)
	Send(id, title, body string) error
	Remove(id string) error
	// OnClick calls fn with the identifier of a notification the user clicked.
	OnClick(fn func(id string))
	// Cleanup releases what Initialize opened, such as the D-Bus connection.
	Cleanup()
}

// notifications sets up the desktop's notifications once, on first use.
type notifications struct {
	mu     sync.Mutex
	n      Notifier
	ready  bool
	status NotificationStatus
	// asked is set once authorization was requested.
	asked bool
}

// notificationIDPrefix starts the identifiers of this window's
// notifications. Each window is its own process; one ignores clicks on
// another's notifications.
func notificationIDPrefix() string {
	return "agent-sessions-" + strconv.Itoa(os.Getpid()) + "-"
}

// notificationID names the notification of a tab; a tab has at most one.
func notificationID(tab string) string { return notificationIDPrefix() + tab }

// notificationTab is the tab a notification of this window is about.
func notificationTab(id string) (string, bool) {
	tab, ok := strings.CutPrefix(id, notificationIDPrefix())
	return tab, ok && tab != ""
}

func (a *App) notifications() *notifications {
	a.notifyOnce.Do(func() {
		n := a.notifierOverride
		if n == nil && a.ctx != nil {
			n = desktopFrom(a.ctx).Notifications
		}
		a.notify = &notifications{n: n}
	})
	return a.notify
}

// setup initializes notifications and, when ask is true, asks the user for
// permission where the platform needs it (macOS), once per run.
func (a *App) setupNotifications(ask bool) NotificationStatus {
	ns := a.notifications()
	ns.mu.Lock()
	defer ns.mu.Unlock()
	if ns.n == nil {
		return NotificationStatus{Reason: "Notifications are shown by the desktop app only."}
	}
	if !ns.ready {
		if !ns.n.Available() {
			ns.status = NotificationStatus{Reason: unavailableReason()}
			return ns.status
		}
		if err := ns.n.Initialize(); err != nil {
			ns.status = NotificationStatus{Reason: fmt.Sprintf("Notifications could not start: %v.", err)}
			return ns.status
		}
		ns.ready = true
		ns.n.OnClick(a.notificationClicked)
	}
	ok, err := ns.n.Authorized()
	if err == nil && !ok && ask && !ns.asked {
		ns.asked = true
		ok, err = ns.n.RequestAuthorization()
	}
	switch {
	case err != nil:
		ns.status = NotificationStatus{Reason: fmt.Sprintf("Could not check permission to notify: %v.", err)}
	case ok:
		ns.status = NotificationStatus{Available: true}
	case !ns.asked:
		ns.status = NotificationStatus{Reason: "Agent Sessions asks for permission to notify when an agent tab starts."}
	default:
		ns.status = NotificationStatus{Reason: deniedReason()}
	}
	return ns.status
}

func unavailableReason() string {
	if runtime.GOOS == "darwin" {
		return "Notifications need macOS 11 or later."
	}
	return "This desktop has no notification service."
}

func deniedReason() string {
	if runtime.GOOS == "darwin" {
		return "Notifications from Agent Sessions are turned off. Allow them in System Settings → Notifications → Agent Sessions."
	}
	return "Notifications from Agent Sessions are turned off in the system settings."
}

// NotificationStatus reports whether agent alerts can be shown, without
// asking the user for permission.
func (a *App) NotificationStatus() NotificationStatus {
	return a.setupNotifications(false)
}

// PrepareNotifications readies agent alerts, asking the user for
// permission the first time where the platform needs it.
func (a *App) PrepareNotifications() NotificationStatus {
	return a.setupNotifications(true)
}

// Notify shows an alert about an agent tab, replacing the tab's previous one.
func (a *App) Notify(n AgentNotification) error {
	if n.Tab == "" {
		return fmt.Errorf("notify: no tab")
	}
	if status := a.setupNotifications(false); !status.Available {
		return fmt.Errorf("notify: %s", status.Reason)
	}
	ns := a.notifications()
	id := notificationID(n.Tab)
	_ = ns.n.Remove(id)
	if err := ns.n.Send(id, n.Title, n.Body); err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	return nil
}

// ClearNotification removes a tab's alert, once the user has seen the tab.
func (a *App) ClearNotification(tab string) error {
	ns := a.notifications()
	ns.mu.Lock()
	ready := ns.ready
	ns.mu.Unlock()
	if !ready || tab == "" {
		return nil
	}
	return ns.n.Remove(notificationID(tab))
}

// closeNotifications releases the notifications set up, at shutdown.
func (a *App) closeNotifications() {
	ns := a.notifications()
	ns.mu.Lock()
	defer ns.mu.Unlock()
	if ns.ready {
		ns.n.Cleanup()
		ns.ready = false
	}
}

// notificationClicked brings the window forward and tells the frontend
// which tab to show.
func (a *App) notificationClicked(id string) {
	tab, ok := notificationTab(id)
	if !ok {
		return
	}
	if a.notifyRaise != nil {
		a.notifyRaise()
	} else if a.ctx != nil {
		raiseWindow(a.ctx)
	}
	if a.desktopEvents != nil {
		a.desktopEvents.Emit(notifyOpenEvent, map[string]string{"id": tab})
	}
}
