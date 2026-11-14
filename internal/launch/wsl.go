package launch

import "errors"

// ErrWSLNotInstalled means this machine has no wsl.exe.
var ErrWSLNotInstalled = errors.New("WSL is not installed")

// WSLExecArgs returns the wsl.exe arguments that run script with /bin/sh in
// distro, starting in the user's home. wsl.exe hands --exec arguments to
// Linux unchanged, but they pass through a Windows command line first; in
// OctalEscape form the script needs no quoting there, and the fixed decoder
// is the only argument that does.
func WSLExecArgs(distro, script string) []string {
	return []string{"-d", distro, "--cd", "~", "--exec", "/bin/sh", "-c", octalDecoder, "sh", OctalEscape(script)}
}

// WSLLoginExecArgs returns the wsl.exe arguments that run script as
// PosixLoginArgv does, in distro, starting in the user's home.
func WSLLoginExecArgs(distro, script string) []string {
	return append([]string{"-d", distro, "--cd", "~", "--exec"}, PosixLoginArgv(script)...)
}
