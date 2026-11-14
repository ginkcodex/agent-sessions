package launch

import (
	"os"
	"runtime"
	"strings"

	"github.com/ginkcode/agent-sessions/internal/provider"
)

// PanelArgv returns what the app's built-in terminal panel runs for cmd on
// this computer, in cmd.Dir: the script an external terminal runs, except
// that it exits with the agent's status rather than staying in the user's
// shell, so the tab closes when the agent exits.
func PanelArgv(cmd provider.Command) ([]string, error) {
	if runtime.GOOS == "windows" {
		ps, err := windowsPowerShell()
		if err != nil {
			return nil, err
		}
		return powerShellPanelArgv(cmd, Find, ps)
	}
	return posixPanelArgv(cmd, Find)
}

// PanelShellArgv returns the plain shell a panel tab runs on this computer:
// the user's shell with ephemeral prompt-time OSC7 cwd reporting, or Windows
// PowerShell with its profile's prompt wrapped. Unknown POSIX shells fall back
// to an unmodified login shell.
func PanelShellArgv() ([]string, error) {
	if runtime.GOOS == "windows" {
		ps, err := windowsPowerShell()
		if err != nil {
			return nil, err
		}
		return []string{ps, "-NoLogo", "-NoExit", "-Command", panelPowerShellHook}, nil
	}
	return []string{"/bin/sh", "-c", panelPosixShellScript(ShellEscape(loginShell(runtime.GOOS, os.Getenv)))}, nil
}

func posixPanelArgv(cmd provider.Command, find func(string) (Executable, error)) ([]string, error) {
	argv, err := posixArgv(cmd, find)
	if err != nil {
		return nil, err
	}
	return []string{"/bin/sh", "-c", posixScript(argv, cmd.Dir, "")}, nil
}

func powerShellPanelArgv(cmd provider.Command, find func(string) (Executable, error), ps string) ([]string, error) {
	script, err := terminalScript(cmd, find)
	if err != nil {
		return nil, err
	}
	return []string{ps, "-NoLogo", "-Command", script + powerShellExitStatus}, nil
}

// powerShellExitStatus ends a -Command script with the agent's exit code.
// PowerShell would exit 0 or 1; a failed cmdlet, which sets no exit code,
// still exits 1.
const powerShellExitStatus = "; $ok = $?; if ($LASTEXITCODE) { exit $LASTEXITCODE }; if (-not $ok) { exit 1 }"

// PanelShellScript is the /bin/sh script a WSL or SSH host's shell tab
// runs: it enters dir when there is one and it exists, then becomes the
// user's login shell. An unconfigured locale defaults to an installed UTF-8
// locale on that host, so the shell can edit the terminal's Unicode input.
func PanelShellScript(dir string) string {
	script := panelRemoteLocale + panelPosixShellScript(`"${SHELL:-/bin/sh}"`)
	if dir != "" {
		script = "cd -- " + ShellEscape(dir) + " 2>/dev/null\n" + script
	}
	return script
}

// posixPanelAgentScript initializes the remote user's interactive/login shell
// in the project before running the safely quoted POSIX agent payload.
func posixPanelAgentScript(argv []string, dir string) string {
	return "cd -- " + ShellEscape(dir) + " || exit 1\n" +
		panelPosixStartupScript(`"${SHELL:-/bin/sh}"`, posixScript(argv, dir, ""))
}

// PosixBootstrapArgv runs script with /bin/sh. The script travels in
// OctalEscape form, so the argv means the same to whatever shell an ssh
// server runs it with, fish and tcsh included (see bootstrapLine).
func PosixBootstrapArgv(script string) []string {
	return []string{"/bin/sh", "-c", octalDecoder, "sh", OctalEscape(script)}
}

// PosixLoginArgv runs script with /bin/sh in the environment of the user's
// login shell, whatever that shell is. Like PosixBootstrapArgv, the argv
// means the same to bash, zsh, fish, csh and tcsh. /bin/sh starts the login
// shell (SHELL, else /bin/sh) and has it run a line that hands script, still
// in OctalEscape form, to /bin/sh again, so the login shell parses only that
// shell-neutral line. csh and tcsh take -l only as their sole flag: they run
// the line with -c, which reads their rc file, and read the login files
// first (cshLoginFiles).
func PosixLoginArgv(script string) []string {
	return []string{"/bin/sh", "-c", octalDecoder, "sh", OctalEscape(loginWrapper), OctalEscape(script)}
}

// loginWrapper is the /bin/sh script PosixLoginArgv decodes and runs; $2 is
// the script in OctalEscape form.
var loginWrapper = `__as_shell=${SHELL:-/bin/sh}
__as_run="/bin/sh -c '` + shDoubleQuoted.Replace(octalDecoder) + `' sh '$2'"
case "${__as_shell##*/}" in
csh|tcsh) exec "$__as_shell" -c "` + shDoubleQuoted.Replace(cshLoginFiles) + `exec $__as_run" ;;
*) exec "$__as_shell" -lc "$__as_run" ;;
esac
`

// shDoubleQuoted escapes text for a /bin/sh double-quoted string that holds
// no backquote.
var shDoubleQuoted = strings.NewReplacer(`"`, `\"`, `$`, `\$`)

// cshLoginFiles is the csh script that reads the login files a csh or tcsh
// started with -c skips, in a login shell's order after the rc files, then
// returns to where it started: Gentoo's and Alpine's csh.login end with cd.
// A login file with an error ends the script, where a login shell would go
// on to its prompt.
const cshLoginFiles = `set __as_cwd = $cwd:q
if ( -r /etc/csh.login ) source /etc/csh.login
if ( -r ~/.login ) source ~/.login
cd $__as_cwd:q
unset __as_cwd
`

// WSLPanelArgv returns the wsl.exe argv that runs script with /bin/sh in
// distro.
func WSLPanelArgv(distro, script string) ([]string, error) {
	wsl, err := WSLBinary()
	if err != nil {
		return nil, err
	}
	return append([]string{wsl}, WSLExecArgs(distro, script)...), nil
}
