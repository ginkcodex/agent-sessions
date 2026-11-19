//go:build windows

package paths

import (
	"path/filepath"
)

// FromEnv resolves roots using the provided getenv function and user home directory.
// Relative or empty environment variable values are ignored.
func FromEnv(getenv func(string) string, home string) Roots {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	envDir := func(env string, fallback ...string) string {
		if v := getenv(env); filepath.IsAbs(v) {
			return filepath.Clean(v)
		}
		return filepath.Join(append([]string{home}, fallback...)...)
	}

	// OpenCode resolves its data dir with xdg-basedir on every OS, so it lives
	// under %USERPROFILE%\.local\share rather than %LOCALAPPDATA%.
	dataBase := envDir("XDG_DATA_HOME", ".local", "share")
	localAppData := envDir("LOCALAPPDATA", "AppData", "Local")
	appData := envDir("APPDATA", "AppData", "Roaming")

	// ANTIGRAVITY_CLI_DIR is this app's own override: agy reads none.
	// shortcut: the Windows default is unverified, confirm it in AGY-08.
	return Roots{
		Claude:       envDir("CLAUDE_CONFIG_DIR", ".claude"),
		Codex:        envDir("CODEX_HOME", ".codex"),
		OpenCodeData: filepath.Join(dataBase, "opencode"),
		Antigravity:  envDir("ANTIGRAVITY_CLI_DIR", ".gemini", "antigravity-cli"),
		Cache:        filepath.Join(localAppData, "agent-sessions", "cache"),
		Config:       filepath.Join(appData, "agent-sessions"),
		Data:         filepath.Join(localAppData, "agent-sessions"),
	}
}
