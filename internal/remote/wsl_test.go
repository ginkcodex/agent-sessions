package remote

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/launch"
	"github.com/ginkcode/agent-sessions/internal/rpc"
	"github.com/ginkcode/agent-sessions/internal/testutil/platform"
)

func TestValidateHost(t *testing.T) {
	for _, tc := range []struct {
		host string
		ok   bool
	}{
		{"wsl:Ubuntu", true},
		{"wsl:Ubuntu-24.04", true},
		{"wsl:my_distro.2", true},
		{"wsl:", false},
		{"wsl:-x", false},
		{"wsl:.hidden", false},
		{"wsl:a b", false},
		{"wsl:a;b", false},
		{"wsl:a/b", false},
		{"wsl:Ubuntu\n", false},
		{"devbox", true},
		{"-oProxyCommand=x", false},
	} {
		err := ValidateHost(tc.host)
		if (err == nil) != tc.ok {
			t.Errorf("ValidateHost(%q) = %v", tc.host, err)
		}
		if err != nil && !errors.Is(err, ErrInvalidHostAlias) {
			t.Errorf("ValidateHost(%q) = %v, want ErrInvalidHostAlias", tc.host, err)
		}
	}
	if d, ok := ParseWSLTarget(WSLTarget("Debian")); !ok || d != "Debian" {
		t.Errorf("ParseWSLTarget(WSLTarget) = %q, %v", d, ok)
	}
	if _, ok := ParseWSLTarget("devbox"); ok {
		t.Error("SSH alias parsed as WSL")
	}
	// The server started for a target hosts diagnostics as that kind.
	if k := diagnosticsHostKind(WSLTarget("Debian")); k != "wsl" {
		t.Error(k)
	}
	if k := diagnosticsHostKind("devbox"); k != "ssh" {
		t.Error(k)
	}
}

func TestRemoteLine(t *testing.T) {
	if got, want := remoteLine(LoginCommand("echo 'x'")), QuoteArgs(launch.PosixLoginArgv("echo 'x'")); got != want {
		t.Errorf("login line = %s, want %s", got, want)
	}
	if got := remoteLine([]string{"/bin/cli", "transfer", "get"}); got != `'/bin/cli' 'transfer' 'get'` {
		t.Errorf("plain line = %s", got)
	}
}

func TestWSLArgs(t *testing.T) {
	args, err := wslArgs("Ubuntu", []string{"/bin/cli", "transfer", "put", "tok"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-d", "Ubuntu", "--cd", "~", "--exec", "/bin/sh", "-c", `eval "$(printf "$1")"`, "sh", `\047/bin/cli\047 \047transfer\047 \047put\047 \047tok\047`}
	if !slices.Equal(args, want) {
		t.Errorf("wslArgs = %q\nwant %q", args, want)
	}
	if _, err := wslArgs("-d", []string{"true"}); !errors.Is(err, ErrInvalidHostAlias) {
		t.Errorf("bad distro: %v", err)
	}
	if _, err := wslArgs("Ubuntu", []string{"printf", "a\x00b"}); !errors.Is(err, launch.ErrUnsafeArgument) {
		t.Errorf("NUL argument: %v", err)
	}
}

// runWSLArgs runs what wsl.exe would run in the distro: the arguments after
// --exec.
func runWSLArgs(t *testing.T, args []string, env ...string) []byte {
	t.Helper()
	i := slices.Index(args, "--exec")
	c := exec.Command(args[i+1], args[i+2:]...)
	c.Env = append(os.Environ(), env...)
	out, err := c.Output()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return out
}

func TestWSLArgsRunCommandExactly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell")
	}
	tricky := []string{"it's", `"q"`, "$HOME", "`id`", `back\slash`, "line1\nline2", "", "100%", "ünï"}

	// A plain argv, as transfers send.
	args, err := wslArgs("Ubuntu", append([]string{"/bin/sh", "-c", `printf '%s\0' "$@"`, "sh"}, tricky...))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSuffix(string(runWSLArgs(t, args)), "\x00"), "\x00")
	if !slices.Equal(got, tricky) {
		t.Errorf("args = %q, want %q", got, tricky)
	}

	// A login command, as probe, deploy and serve send: the login shell
	// sets up the environment and hands the script to /bin/sh. Unset,
	// SHELL is /bin/sh.
	shell := filepath.Join(platform.TempDir(t), "login-shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\n[ \"$1\" = -lc ] || exit 9\nexport AS_LOGIN=yes\nexec /bin/sh -c \"$2\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "printf '%s\\0' \"${AS_LOGIN:-none}\" 'it'\\''s' '$(id)' \"`echo y`\" '100%' 'ünï'"
	args, err = wslArgs("Ubuntu", LoginCommand(script))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"yes", "it's", "$(id)", "y", "100%", "ünï"}
	got = strings.Split(strings.TrimSuffix(string(runWSLArgs(t, args, "SHELL="+shell)), "\x00"), "\x00")
	if !slices.Equal(got, want) {
		t.Errorf("login command printed %q, want %q", got, want)
	}
	want[0] = "none"
	got = strings.Split(strings.TrimSuffix(string(runWSLArgs(t, args, "SHELL=")), "\x00"), "\x00")
	if !slices.Equal(got, want) {
		t.Errorf("login command without SHELL printed %q, want %q", got, want)
	}
}

// TestWSLArgsFitCommandLine keeps every command under the 32767-character
// Windows command-line limit after octal escaping.
func TestWSLArgsFitCommandLine(t *testing.T) {
	nonce := rpc.GenerateNonce()
	bin := "/home/someone-with-a-long-name/.cache/agent-sessions/server/v0.5.2-0123abcd/agent-sessions-cli"
	for name, script := range map[string]string{
		"probe":  probeScript(rpc.FormatPreface(nonce), "v0.5.2"),
		"unpack": unpackScript(filepath.Dir(bin), bin, strings.Repeat("a", 64), "0123456789abcdef"),
		"prune":  pruneScript("/home/someone/.cache/agent-sessions/server", "v0.5.2-0123abcd"),
	} {
		args, err := wslArgs("Ubuntu", LoginCommand(script))
		if err != nil {
			t.Fatal(err)
		}
		if n := len(strings.Join(args, " ")) + len(`C:\Windows\System32\wsl.exe`) + 16; n > 32767 {
			t.Errorf("%s command line is %d characters", name, n)
		}
	}
}

func TestBuildHostCmd(t *testing.T) {
	orig := wslBinary
	t.Cleanup(func() { wslBinary = orig })
	wslBinary = func() (string, error) { return "/fake/wsl.exe", nil }

	cmd, err := BuildHostCmd(context.Background(), "wsl:Ubuntu", LoginCommand("true"), SSHOptions{Binary: "/fake/ssh"})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := wslArgs("Ubuntu", LoginCommand("true"))
	if cmd.Path != "/fake/wsl.exe" || !slices.Equal(cmd.Args[1:], want) {
		t.Errorf("WSL command = %s %q", cmd.Path, cmd.Args)
	}
	if !slices.Contains(cmd.Env, "WSL_UTF8=1") {
		t.Error("WSL_UTF8 not set")
	}

	// An SSH alias builds exactly what BuildSSHCmd does.
	opts := SSHOptions{Binary: "/fake/ssh", ControlMaster: "no"}
	cmd, err = BuildHostCmd(context.Background(), "devbox", []string{"true"}, opts)
	if err != nil {
		t.Fatal(err)
	}
	ssh, err := BuildSSHCmd(context.Background(), "devbox", []string{"true"}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != ssh.Path || !slices.Equal(cmd.Args, ssh.Args) {
		t.Errorf("SSH command = %q, want %q", cmd.Args, ssh.Args)
	}

	wslBinary = func() (string, error) { return "", ErrWSLNotInstalled }
	if _, err := BuildHostCmd(context.Background(), "wsl:Ubuntu", []string{"true"}, opts); !errors.Is(err, ErrWSLNotInstalled) {
		t.Errorf("without wsl.exe: %v", err)
	}
}

func TestStartSessionChecksWSLTarget(t *testing.T) {
	if _, err := StartSession(context.Background(), "wsl:a b", SSHOptions{}, nil, nil); !errors.Is(err, ErrInvalidHostAlias) {
		t.Errorf("bad distro: %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if _, err := StartSession(context.Background(), "wsl:Ubuntu", SSHOptions{}, nil, nil); !errors.Is(err, ErrWSLNotInstalled) {
		t.Errorf("WSL outside Windows: %v", err)
	}
}

func TestStopControlMasterSkipsWSL(t *testing.T) {
	opts := SSHOptions{Binary: filepath.Join(t.TempDir(), "no-ssh")}
	if err := StopControlMaster(context.Background(), "", "wsl:Ubuntu", opts); err != nil {
		t.Errorf("StopControlMaster(wsl) = %v", err)
	}
}
