package main

// linuxAppID is the GTK application id. GTK 4 sends it to Wayland as the
// windows' app_id and to X11 as _GTK_APPLICATION_ID, so GNOME and KDE pair
// the windows with the desktop entry of the same name
// (packaging/linux/<linuxAppID>.desktop) and its icon. Without it Wails
// derives "org.wails.agent_sessions", which matches no desktop entry.
const linuxAppID = "io.github.ginkcode.AgentSessions"

// linuxProgramName is the GLib program name: the X11 WM_CLASS, matched by the
// desktop entry's StartupWMClass, and the directory WebKit keeps localStorage
// in (~/.local/share/agent-sessions). Changing it resets saved preferences.
const linuxProgramName = "agent-sessions"
