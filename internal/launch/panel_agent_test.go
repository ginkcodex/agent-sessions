package launch

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aymanbagabas/go-pty"
)

func panelPTYOutput(t *testing.T, ctx context.Context, spec *exec.Cmd, input string) ([]byte, error) {
	t.Helper()
	terminal, err := pty.New()
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = terminal.Close() })
	defer stopClose()
	cmd := terminal.CommandContext(ctx, spec.Path, spec.Args[1:]...)
	cmd.Env, cmd.Dir = spec.Env, spec.Dir
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Let reads end when the child closes its terminal, as real panels do.
	if unix, ok := terminal.(pty.UnixPty); ok {
		_ = unix.Slave().Close()
	}
	output := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(terminal)
		output <- b
	}()
	if input != "" {
		if _, err := io.WriteString(terminal, input); err != nil {
			t.Fatal(err)
		}
	}
	err = cmd.Wait()
	return <-output, err
}

func TestRemotePanelAgentStartup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shells")
	}
	for _, shell := range []string{"bash", "zsh", "fish", "tcsh"} {
		modes := []string{"rc", "profile", "profile-sources-rc"}
		if shell == "tcsh" {
			modes = []string{"rc", "profile"}
		}
		for _, mode := range modes {
			t.Run(shell+"/"+mode, func(t *testing.T) {
				path, err := exec.LookPath(shell)
				if err != nil {
					t.Skip(err)
				}
				home := t.TempDir()
				dir := filepath.Join(home, "project ' space")
				tmp := filepath.Join(home, "tmp")
				for _, d := range []string{dir, tmp} {
					if err := os.Mkdir(d, 0700); err != nil {
						t.Fatal(err)
					}
				}
				write := func(name, text string) {
					t.Helper()
					p := filepath.Join(home, name)
					if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(p, []byte(text), 0600); err != nil {
						t.Fatal(err)
					}
				}
				// A prompt-time hook simulates mise selecting tools for this project.
				switch shell {
				case "bash":
					activation := "export ACTIVATED=${ACTIVATED}x\nPROMPT_COMMAND='export PROJECT_READY=$PWD'\n"
					if mode == "profile" {
						write(".profile", activation)
					} else {
						write(".bashrc", activation)
						if mode == "profile-sources-rc" {
							write(".bash_profile", ". \"$HOME/.bashrc\"\n")
						}
					}
				case "zsh":
					activation := "export ACTIVATED=${ACTIVATED}x\nprecmd() { export PROJECT_READY=$PWD; }\n"
					if mode == "profile" {
						write(".zprofile", activation)
					} else {
						write(".zshrc", activation)
					}
				case "tcsh":
					activation := "setenv ACTIVATED ${ACTIVATED}x\nalias precmd 'setenv PROJECT_READY \"$cwd\"'\n"
					if mode == "profile" {
						write(".login", activation)
					} else {
						write(".tcshrc", activation)
					}
				case "fish":
					write(".config/fish/config.fish", "set -gx ACTIVATED x\nfunction project_ready --on-event fish_prompt\n set -gx PROJECT_READY $PWD\nend\n")
				}
				agent := filepath.Join(home, "agent")
				out := filepath.Join(home, "result")
				if err := os.WriteFile(agent, []byte("#!/bin/sh\ntest -t 0 && test -t 1 && test -t 2 || exit 99\nprintf '%s\\n' \"$ACTIVATED\" \"$PROJECT_READY\" \"$PWD\" \"$1\" > "+ShellEscape(out)+"\nexit 7\n"), 0700); err != nil {
					t.Fatal(err)
				}
				script := posixPanelAgentScript([]string{agent, "argument ' $ quoted"}, dir)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				// Exercise the actual shell-neutral SSH transport, with stdin left for
				// the agent rather than injecting a command into its terminal input.
				// tcsh reads .tcshrc for -c too, so an ssh server's tcsh would
				// add one more x; RF-21's ssh tests cover tcsh as that shell.
				outer := path
				if shell == "tcsh" {
					outer = "/bin/sh"
				}
				cmd := exec.CommandContext(ctx, outer, "-c", JoinCommand(PosixBootstrapArgv(script)))
				cmd.Env = append(os.Environ(), "HOME="+home, "ZDOTDIR="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "TMPDIR="+tmp, "TERM=dumb", "SHELL="+path, "ACTIVATED=", "PROJECT_READY=")
				b, err := panelPTYOutput(t, ctx, cmd, "")
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 7 {
					t.Fatalf("want exit 7: %v: %s", err, b)
				}
				got, err := os.ReadFile(out)
				if err != nil {
					t.Fatalf("%v: %s", err, b)
				}
				want := "x\n" + dir + "\n" + dir + "\nargument ' $ quoted\n"
				if string(got) != want {
					t.Fatalf("got %q, want %q; output %s", got, want, b)
				}
				files, err := os.ReadDir(tmp)
				if err != nil {
					t.Fatal(err)
				}
				for _, file := range files {
					if strings.HasPrefix(file.Name(), "agent-sessions-shell.") {
						t.Fatalf("integration temporary files not cleaned: %v", files)
					}
				}
				if !strings.Contains(string(b), "exited with status 7") {
					t.Fatalf("missing status: %s", b)
				}
			})
		}
	}
}
