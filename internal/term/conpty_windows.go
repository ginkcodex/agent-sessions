//go:build windows

package term

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows' own ConPTY swallows the OSC 10/11 color queries and their
// replies, so a TUI such as OpenCode never learns xterm.js's colors and
// assumes a dark background. The app ships the newer ConPTY of Microsoft's
// Microsoft.Windows.Console.ConPTY package in conpty\ next to its exe
// (`make conpty`), which passes them through. Without it, the terminal
// uses Windows' own.

// conptyAPI is a ConPTY implementation.
type conptyAPI struct {
	create func(size windows.Coord, in, out windows.Handle, flags uint32, console *windows.Handle) error
	resize func(console windows.Handle, size windows.Coord) error
	close  func(console windows.Handle)
}

var systemConpty = conptyAPI{
	create: windows.CreatePseudoConsole,
	resize: windows.ResizePseudoConsole,
	close:  windows.ClosePseudoConsole,
}

// bundledConpty is the app's ConPTY if it has one, else Windows' own.
var bundledConpty = sync.OnceValue(func() *conptyAPI {
	exe, err := os.Executable()
	if err != nil {
		return &systemConpty
	}
	dir := filepath.Join(filepath.Dir(exe), "conpty")
	// conpty.dll runs OpenConsole.exe from its own directory.
	if _, err := os.Stat(filepath.Join(dir, "OpenConsole.exe")); err != nil {
		return &systemConpty
	}
	dll := windows.NewLazyDLL(filepath.Join(dir, "conpty.dll"))
	create := dll.NewProc("CreatePseudoConsole")
	resize := dll.NewProc("ResizePseudoConsole")
	closeConsole := dll.NewProc("ClosePseudoConsole")
	for _, p := range []*windows.LazyProc{create, resize, closeConsole} {
		if p.Find() != nil {
			return &systemConpty
		}
	}
	return &conptyAPI{
		create: func(size windows.Coord, in, out windows.Handle, flags uint32, console *windows.Handle) error {
			return hresult(create.Call(coord(size), uintptr(in), uintptr(out), uintptr(flags), uintptr(unsafe.Pointer(console))))
		},
		resize: func(console windows.Handle, size windows.Coord) error {
			return hresult(resize.Call(uintptr(console), coord(size)))
		},
		close: func(console windows.Handle) { _, _, _ = closeConsole.Call(uintptr(console)) },
	}
})

// coord passes size by value, as the ConPTY functions take it.
func coord(size windows.Coord) uintptr { return uintptr(*(*uint32)(unsafe.Pointer(&size))) }

func hresult(r, _ uintptr, _ error) error {
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

var errConsoleClosed = errors.New("pseudo console is closed")

// conpty is a console in a ConPTY.
type conpty struct {
	api *conptyAPI
	// in and out are this end of the console's input and output.
	in, out *os.File
	process *os.Process

	mu sync.RWMutex
	// handle is 0 once the console is closed.
	handle windows.Handle
}

// startConsole starts path, with argv as its arguments including the
// command, in a new pseudo-terminal of cols by rows.
func startConsole(path string, argv []string, dir string, env []string, cols, rows int) (console, error) {
	size := windows.Coord{X: int16(cols), Y: int16(rows)}
	api := bundledConpty()
	c, err := openConpty(api, size)
	if err != nil && api != &systemConpty {
		c, err = openConpty(&systemConpty, size)
	}
	if err != nil {
		return nil, err
	}
	if err := c.start(path, argv, dir, env); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func openConpty(api *conptyAPI, size windows.Coord) (*conpty, error) {
	conIn, in, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	out, conOut, err := os.Pipe()
	if err != nil {
		_, _ = conIn.Close(), in.Close()
		return nil, err
	}
	var h windows.Handle
	err = api.create(size, windows.Handle(conIn.Fd()), windows.Handle(conOut.Fd()), 0, &h)
	// The console keeps its own copies of its ends.
	_, _ = conIn.Close(), conOut.Close()
	if err != nil {
		_, _ = in.Close(), out.Close()
		return nil, fmt.Errorf("could not open a terminal: %w", err)
	}
	return &conpty{api: api, in: in, out: out, handle: h}, nil
}

func (c *conpty) start(path string, argv []string, dir string, env []string) error {
	app, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(argv))
	if err != nil {
		return err
	}
	var dirp *uint16
	if dir != "" {
		if dirp, err = windows.UTF16PtrFromString(dir); err != nil {
			return err
		}
	}
	block, err := envBlock(env)
	if err != nil {
		return err
	}
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return err
	}
	defer attrs.Delete()
	// The attribute's value is the console handle itself.
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&c.handle)), unsafe.Sizeof(c.handle)); err != nil {
		return err
	}
	si := &windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(*si))
	// Empty standard handles: otherwise the process would get this one's
	// redirected handles rather than the console's.
	si.Flags = windows.STARTF_USESTDHANDLES
	var pi windows.ProcessInformation
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT | windows.EXTENDED_STARTUPINFO_PRESENT)
	if err := windows.CreateProcess(app, cmdline, nil, nil, false, flags, block, dirp, &si.StartupInfo, &pi); err != nil {
		return err
	}
	_ = windows.CloseHandle(pi.Thread)
	// pi.Process keeps the id from being reused until os.FindProcess has a
	// handle of its own.
	defer func() { _ = windows.CloseHandle(pi.Process) }()
	if c.process, err = os.FindProcess(int(pi.ProcessId)); err != nil {
		_ = windows.TerminateProcess(pi.Process, 1)
		return err
	}
	return nil
}

func (c *conpty) Read(b []byte) (int, error) { return c.out.Read(b) }

func (c *conpty) Write(b []byte) (int, error) { return c.in.Write(b) }

func (c *conpty) Resize(cols, rows int) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.handle == 0 {
		return errConsoleClosed
	}
	return c.api.resize(c.handle, windows.Coord{X: int16(cols), Y: int16(rows)})
}

// Close closes the console, which ends the processes attached to it. It
// returns once the console's output has been read.
func (c *conpty) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.handle == 0 {
		return errConsoleClosed
	}
	c.api.close(c.handle)
	c.handle = 0
	return errors.Join(c.in.Close(), c.out.Close())
}

func (c *conpty) Process() *os.Process { return c.process }

func (c *conpty) Wait() (*os.ProcessState, error) { return c.process.Wait() }

// envBlock is env as CreateProcess takes it. As in os/exec, a variable
// replaces an earlier one whose name differs only in case, and SYSTEMROOT,
// which many programs need, is added if missing.
func envBlock(env []string) (*uint16, error) {
	index := map[string]int{}
	vars := make([]string, 0, len(env)+1)
	for _, kv := range env {
		// Names such as "=C:" start with "=".
		name, _, ok := strings.Cut(kv[min(1, len(kv)):], "=")
		if !ok {
			continue
		}
		name = strings.ToUpper(kv[:min(1, len(kv))] + name)
		if i, ok := index[name]; ok {
			vars[i] = kv
			continue
		}
		index[name] = len(vars)
		vars = append(vars, kv)
	}
	if _, ok := index["SYSTEMROOT"]; !ok {
		vars = append(vars, "SYSTEMROOT="+os.Getenv("SYSTEMROOT"))
	}
	var block []uint16
	for _, kv := range vars {
		u, err := windows.UTF16FromString(kv)
		if err != nil {
			return nil, err
		}
		block = append(block, u...)
	}
	block = append(block, 0)
	return &block[0], nil
}
