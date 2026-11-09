package launch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPanelRemoteLocale(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executes POSIX remote bootstrap")
	}
	for _, tc := range []struct {
		name, locales, want string
		env                 []string
		failure, missing    bool
	}{
		{name: "reported Linux host", locales: "C\nC.utf8\nPOSIX\nen_US.utf8\n", want: "C.utf8||"},
		{name: "hyphenated C", locales: "en_US.UTF-8\nC.UTF-8\n", want: "C.UTF-8||"},
		{name: "English fallback", locales: "de_DE.utf8\nen_US.utf8\n", want: "en_US.utf8||"},
		{name: "other installed UTF8", locales: "C\nvi_VN.UTF-8\n", want: "vi_VN.UTF-8||"},
		{name: "no UTF8", locales: "C\nPOSIX\n", want: "||"},
		{name: "discovery failure", locales: "C.utf8\n", failure: true, want: "||"},
		{name: "missing locale", missing: true, want: "||"},
		{name: "explicit LANG", env: []string{"LANG=vi_VN.UTF-8"}, locales: "C.utf8\n", want: "vi_VN.UTF-8||"},
		{name: "explicit C", env: []string{"LANG=C"}, locales: "C.utf8\n", want: "C||"},
		{name: "explicit CTYPE", env: []string{"LC_CTYPE=POSIX"}, locales: "C.utf8\n", want: "|POSIX|"},
		{name: "explicit ALL", env: []string{"LC_ALL=C"}, locales: "C.utf8\n", want: "||C"},
		{name: "empty values", env: []string{"LANG=", "LC_CTYPE=", "LC_ALL="}, locales: "C.utf8\n", want: "C.utf8||"},
		{name: "other category", env: []string{"LC_TIME=C"}, locales: "C.utf8\n", want: "C.utf8||"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, quoted := range []bool{false, true} {
				dir := t.TempDir()
				shell := filepath.Join(dir, "login shell")
				if err := os.WriteFile(shell, []byte("#!/bin/sh\n[ \"$1\" = -l ] || exit 2\nprintf '%s|%s|%s' \"${LANG-}\" \"${LC_CTYPE-}\" \"${LC_ALL-}\"\n"), 0o700); err != nil {
					t.Fatal(err)
				}
				if !tc.missing {
					exit := "0"
					if tc.failure {
						exit = "1"
					}
					stub := "#!/bin/sh\n[ \"$1\" = -a ] && [ \"$LC_ALL\" = C ] || exit 2\nprintf '%s' " + ShellEscape(tc.locales) + "\nexit " + exit + "\n"
					if err := os.WriteFile(filepath.Join(dir, "locale"), []byte(stub), 0o700); err != nil {
						t.Fatal(err)
					}
				}
				script := PanelShellScript("")
				if quoted {
					script = JoinCommand(PosixBootstrapArgv(script))
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script)
				cmd.Env = []string{"PATH=" + dir, "HOME=" + dir, "SHELL=" + shell}
				cmd.Env = append(cmd.Env, tc.env...)
				out, err := cmd.CombinedOutput()
				cancel()
				if err != nil || string(out) != tc.want {
					t.Fatalf("quoted=%v: output %q, err %v; want %q", quoted, out, err, tc.want)
				}
			}
		})
	}
}

func TestPanelRemoteLocaleWithInstalledLocales(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executes POSIX remote bootstrap")
	}
	available, err := exec.Command("locale", "-a").Output()
	if err != nil {
		t.Skipf("locale discovery unavailable: %v", err)
	}
	lower := strings.ToLower(string(available))
	if !strings.Contains(lower, ".utf8") && !strings.Contains(lower, ".utf-8") {
		t.Skip("host has no installed UTF-8 locale")
	}
	dir := t.TempDir()
	shell := filepath.Join(dir, "login shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nlocale charmap\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// No locale variables, matching the reported SSH environment. Discovery
	// uses this host's real locale utility rather than the controlled fixtures.
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", PanelShellScript(""))
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "SHELL=" + shell}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("locale charmap: %v: %s", err, out)
	}
	charmap := strings.ToUpper(strings.TrimSpace(string(out)))
	if charmap != "UTF-8" && charmap != "UTF8" {
		t.Fatalf("bootstrap did not select an installed UTF-8 locale: %q", out)
	}
}

func TestPanelLocaleOnlyRemotePlainShell(t *testing.T) {
	if strings.Contains(panelPosixShellScript("/bin/bash"), panelRemoteLocale) {
		t.Fatal("local shell uses remote fallback")
	}
	if strings.Contains(posixPanelAgentScript([]string{"agent"}, "/tmp"), panelRemoteLocale) {
		t.Fatal("agent uses remote fallback")
	}
	if !strings.Contains(PanelShellScript(""), panelRemoteLocale) {
		t.Fatal("remote shell missing locale fallback")
	}
}
