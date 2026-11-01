//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var messageBox = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")

type windowsPlatform struct {
	sid, systemInstaller string
	directory            string
	log                  *os.File
}

func dialog(text string, flags uintptr) uintptr {
	result, _, _ := messageBox.Call(0, uintptr(unsafe.Pointer(wide(text))), uintptr(unsafe.Pointer(wide("Agent Sessions Setup"))), flags|0x10000)
	return result
}

func main() {
	os.Exit(setupMain())
}

func setupMain() int {
	if len(payloadMSI) == 0 {
		dialog("This setup executable was built without a bundled MSI. Build it using the Windows setup packaging target.", 0x10)
		return 1
	}
	sid, err := currentIdentity()
	if err != nil {
		dialog(err.Error(), 0x10)
		return 1
	}
	mutex, err := acquireSetupMutex(sid)
	if err != nil {
		dialog(err.Error(), 0x10)
		return 1
	}
	defer windows.CloseHandle(mutex)
	installer, err := systemMSIExec()
	if err != nil {
		dialog(err.Error(), 0x10)
		return 1
	}
	p := &windowsPlatform{sid: sid, systemInstaller: installer}
	out := runSetup(p)
	if p.log != nil {
		defer p.log.Close()
	}
	text := fmt.Sprintf("%s\n\nOperation: %s\nWindows Installer status: %d", out.Message, out.Operation, out.Status)
	if p.directory != "" {
		text += "\n\nLogs and recovery MSI (retained):\n" + p.directory
		if !out.Success {
			text += "\n\nIf the old app was removed, resolve the error (and restart Windows if requested), then rerun setup or manually install payload.msi from this directory. Settings and session data have not been deleted by setup."
		}
	}
	p.Trace(text)
	flags := uintptr(0x10)
	if out.Success {
		flags = 0x40
	} else if out.Status == 1602 || out.Status == 3010 || out.Status == 1641 {
		flags = 0x30
	}
	dialog(text, flags)
	if out.Success {
		return int(out.Status)
	}
	if out.Status != 0 {
		return int(out.Status)
	}
	return 1
}

func currentIdentity() (string, error) {
	token := windows.GetCurrentProcessToken()
	if token.IsElevated() {
		return "", fmt.Errorf("Do not run Agent Sessions Setup as administrator. Close setup and run it normally as the user who will use the app")
	}
	user, err := token.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("cannot determine the installing user: %w", err)
	}
	return user.User.Sid.String(), nil
}

func acquireSetupMutex(sid string) (windows.Handle, error) {
	// Global namespace prevents a second setup in another terminal session of
	// this user. No ownership/release is needed: the named object lives exactly
	// as long as one of the wrappers holds a handle.
	handle, err := windows.CreateMutex(nil, false, wide("Global\\ginkcode.AgentSessions.Setup."+sid))
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		return 0, fmt.Errorf("another Agent Sessions setup is already running for this user")
	}
	if err != nil {
		return 0, fmt.Errorf("cannot acquire setup mutex: %w", err)
	}
	return handle, nil
}

func systemMSIExec() (string, error) {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "msiexec.exe")
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cannot find system Windows Installer: %w", err)
	}
	if !filepath.IsAbs(path) || info.IsDir() {
		return "", fmt.Errorf("invalid system Windows Installer path")
	}
	return path, nil
}

func (p *windowsPlatform) CurrentSID() string { return p.sid }
func (p *windowsPlatform) Prepare() (payload, error) {
	out := payload{Version: payloadVersion}
	hash := sha256.Sum256(payloadMSI)
	if hex.EncodeToString(hash[:]) != payloadSHA256 {
		return out, fmt.Errorf("bundled MSI checksum does not match its build metadata")
	}
	local, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return out, err
	}
	root := filepath.Join(local, "ginkcode", "Agent Sessions", "Setup")
	if err := os.MkdirAll(root, 0700); err != nil {
		return out, err
	}
	directory, err := os.MkdirTemp(root, "run-")
	if err != nil {
		return out, err
	}
	p.directory = directory
	out.Directory = directory
	p.log, err = os.OpenFile(p.LogPath("setup.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return out, err
	}
	out.Path = filepath.Join(directory, "payload.msi")
	file, err := os.OpenFile(out.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return out, err
	}
	_, writeErr := file.Write(payloadMSI)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return out, err
	}
	diskPayload, err := os.ReadFile(out.Path)
	if err != nil {
		return out, err
	}
	diskHash := sha256.Sum256(diskPayload)
	if diskHash != hash {
		return out, fmt.Errorf("extracted MSI checksum does not match the bundle")
	}
	p.Trace("Recovery MSI: " + out.Path + "; SHA256: " + payloadSHA256 + "; MSI version: " + payloadVersion)
	if err := validateMSI(out.Path, payloadVersion); err != nil {
		return out, err
	}
	out.Code, err = payloadProductCode(out.Path)
	return out, err
}

func (p *windowsPlatform) Prerequisite() error {
	checks := []struct {
		root   registry.Key
		path   string
		access uint32
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`, registry.QUERY_VALUE | registry.WOW64_64KEY},
		{registry.CURRENT_USER, `Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`, registry.QUERY_VALUE},
	}
	for _, check := range checks {
		key, err := registry.OpenKey(check.root, check.path, check.access)
		if err != nil {
			continue
		}
		version, _, err := key.GetStringValue("pv")
		key.Close()
		if err == nil && version != "" && version != "0.0.0.0" {
			return nil
		}
	}
	return fmt.Errorf("Agent Sessions needs the Microsoft Edge WebView2 Runtime. Install it from https://go.microsoft.com/fwlink/p/?LinkId=2124703 and rerun setup; nothing has been uninstalled")
}
func (p *windowsPlatform) Discover() ([]product, error) { return installedProducts() }
func (p *windowsPlatform) Confirm(text string) bool     { return dialog(text, 0x01|0x30|0x100) == 1 }
func (p *windowsPlatform) RetryClose() bool {
	return dialog("Close Agent Sessions normally before continuing. Setup will not terminate it.\n\nClick Retry after closing every Agent Sessions window, or Cancel to stop setup.", 0x05|0x30|0x100) == 4
}
func (p *windowsPlatform) Running() (bool, error)     { return appRunning(p.sid) }
func (p *windowsPlatform) LogPath(name string) string { return filepath.Join(p.directory, name) }
func (p *windowsPlatform) Trace(message string) {
	if p.log != nil {
		fmt.Fprintf(p.log, "%s %s\r\n", time.Now().Format(time.RFC3339), message)
		_ = p.log.Sync()
	}
}
func (p *windowsPlatform) Execute(operation, target, logPath string) (uint32, error) {
	args := installerArgs(operation, target, logPath)
	p.Trace("Starting " + operation + " " + target + "; verbose log: " + logPath)
	cmd := exec.Command(p.systemInstaller, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err := cmd.Run() // Run waits for this separate MSI transaction to finish.
	if err == nil {
		p.Trace("Windows Installer returned 0")
		return 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		status := uint32(exit.ExitCode())
		p.Trace(fmt.Sprintf("Windows Installer returned %d", status))
		return status, nil
	}
	return 0, err
}

func appRunning(sid string) (bool, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return false, err
	}
	for {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "agent-sessions.exe") {
			running, err := inspectAppProcess(entry.ProcessID, sid)
			if err != nil {
				return false, err
			}
			if running {
				return true, nil
			}
		}
		err := windows.Process32Next(snapshot, &entry)
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}

func inspectAppProcess(pid uint32, sid string) (bool, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return false, nil
	} // exited
	if err != nil {
		return false, err
	} // uncertain ownership must fail closed
	defer windows.CloseHandle(handle)
	var token windows.Token
	if err := windows.OpenProcessToken(handle, windows.TOKEN_QUERY, &token); err != nil {
		return false, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return false, err
	}
	if user.User.Sid.String() != sid {
		return false, nil
	}
	path := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(path))
	if err := windows.QueryFullProcessImageName(handle, 0, &path[0], &n); err != nil {
		return false, err
	}
	return strings.EqualFold(filepath.Base(windows.UTF16ToString(path[:n])), "agent-sessions.exe"), nil
}
