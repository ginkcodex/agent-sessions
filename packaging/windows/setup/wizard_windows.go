//go:build windows

package main

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// All window handles and GDI objects belong to the locked UI thread. Worker
// events stay in Go memory: WM_APP carries only an integer wakeup, never a Go
// pointer. The timer also drains the queue if PostMessage's quota is exhausted.
var (
	wizardUser   = windows.NewLazySystemDLL("user32.dll")
	wizardGDI    = windows.NewLazySystemDLL("gdi32.dll")
	wizardComctl = windows.NewLazySystemDLL("comctl32.dll")
)

//go:uintptrescapes
func wizardCall(name string, args ...uintptr) uintptr {
	r, _, _ := wizardUser.NewProc(name).Call(args...)
	return r
}

//go:uintptrescapes
func wizardGDICall(name string, args ...uintptr) uintptr {
	r, _, _ := wizardGDI.NewProc(name).Call(args...)
	return r
}

const (
	wizardWake   = 0x8000 + 37
	wizardNext   = 1
	wizardCancel = 2
	wizardBack   = 3
)

type wizardPoint struct{ X, Y int32 }
type wizardRect struct{ Left, Top, Right, Bottom int32 }
type wizardMsg struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          wizardPoint
	Private        uint32
}
type wizardClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}
type wizardPaint struct {
	DC                 uintptr
	Erase              int32
	Rect               wizardRect
	Restore, IncUpdate int32
	Reserved           [32]byte
}
type wizardContrast struct {
	Size, Flags uint32
	Scheme      *uint16
}
type wizardEvent struct {
	title, body string
	question    bool
	kind        wizardPrompt
	answer      chan bool
	result      *outcome
}
type nativeWizard struct {
	mu     sync.Mutex
	events []wizardEvent
	closed bool
	window uintptr // published before starting the worker, immutable until it exits
	work   func(wizardView) outcome
	done   chan struct{}
	wizardNavigation
	pending                                         *wizardEvent
	out                                             outcome
	title, body, back, next, cancel, progress       uintptr
	font, headingFont, icon, smallIcon, sidebarIcon uintptr
	contentBrush, sidebarBrush                      uintptr
	contentColor, textColor, sidebarColor           uint32
	dpi                                             int
	instance                                        uintptr
}

func (u *nativeWizard) post(event wizardEvent) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return false
	}
	u.events = append(u.events, event)
	wizardCall("PostMessageW", u.window, wizardWake, 0, 0)
	return true
}
func (u *nativeWizard) Show(title, body string) { u.post(wizardEvent{title: title, body: body}) }
func (u *nativeWizard) Ask(kind wizardPrompt, text string) bool {
	answer := make(chan bool, 1)
	if !u.post(wizardEvent{question: true, kind: kind, body: text, answer: answer}) {
		return false
	}
	return <-answer
}
func (u *nativeWizard) drain() {
	u.mu.Lock()
	events := u.events
	u.events = nil
	u.mu.Unlock()
	for _, event := range events {
		switch {
		case event.result != nil:
			u.finished = true
			u.out = *event.result
			u.page = wizardResult
			u.display(resultTitle(u.out), finishText(u.out))
		case event.question:
			q := event
			u.pending = &q
			u.showQuestion()
		default:
			u.page = wizardWorking
			u.display(event.title, event.body)
		}
	}
}

func runWizard(work func(wizardView) outcome) (outcome, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// A manifest normally supplies this. Older Windows versions may not export
	// it, and ERROR_ACCESS_DENIED simply means awareness was already established.
	if p := wizardUser.NewProc("SetProcessDpiAwarenessContext"); p.Find() == nil {
		p.Call(^uintptr(3))
	}
	initControls := struct{ Size, Classes uint32 }{8, 0x20}
	r, _, err := wizardComctl.NewProc("InitCommonControlsEx").Call(uintptr(unsafe.Pointer(&initControls)))
	if r == 0 {
		return outcome{}, fmt.Errorf("initialize setup controls: %w", err)
	}
	var instance windows.Handle
	if err := windows.GetModuleHandleEx(2, nil, &instance); err != nil {
		return outcome{}, fmt.Errorf("get setup module: %w", err)
	}
	u := &nativeWizard{work: work, done: make(chan struct{}), dpi: 96, instance: uintptr(instance), wizardNavigation: wizardNavigation{page: wizardWelcome}}
	if p := wizardUser.NewProc("GetDpiForSystem"); p.Find() == nil {
		if d, _, _ := p.Call(); d != 0 {
			u.dpi = int(d)
		}
	}
	name := wide(fmt.Sprintf("AgentSessionsSetupWizard.%d", windows.GetCurrentThreadId()))
	class := wizardClass{Size: uint32(unsafe.Sizeof(wizardClass{})), Instance: u.instance, Name: name,
		Cursor: wizardCall("LoadCursorW", 0, 32512), Proc: windows.NewCallback(u.windowProc)}
	atom, _, err := wizardUser.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return outcome{}, fmt.Errorf("register setup window: %w", err)
	}
	defer wizardCall("UnregisterClassW", uintptr(unsafe.Pointer(name)), u.instance)
	defer u.releaseGraphics()
	// Caption + system menu + minimize, deliberately not resizable/maximizable.
	const style = 0x00C00000 | 0x00080000 | 0x00020000 | 0x02000000
	rect := wizardRect{Right: int32(u.scale(600)), Bottom: int32(u.scale(440))}
	wizardCall("AdjustWindowRectEx", uintptr(unsafe.Pointer(&rect)), style, 0, 0x10000)
	var area wizardRect
	wizardCall("SystemParametersInfoW", 0x30, 0, uintptr(unsafe.Pointer(&area)), 0)
	width, height := rect.Right-rect.Left, rect.Bottom-rect.Top
	x, y := area.Left+(area.Right-area.Left-width)/2, area.Top+(area.Bottom-area.Top-height)/2
	hwnd, _, err := wizardUser.NewProc("CreateWindowExW").Call(0x10000, uintptr(unsafe.Pointer(name)),
		uintptr(unsafe.Pointer(wide("Agent Sessions Setup"))), style, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0, 0, u.instance, 0)
	if hwnd == 0 {
		return outcome{}, fmt.Errorf("create setup window: %w", err)
	}
	u.window = hwnd
	defer func() {
		// Even on an unexpected message-loop error, release a pending decision
		// and wait for all MSI work. Never drop the log/mutex under a worker.
		u.mu.Lock()
		u.closed = true
		events := u.events
		u.events = nil
		u.mu.Unlock()
		if u.pending != nil {
			u.pending.answer <- false
			u.pending = nil
		}
		for _, e := range events {
			if e.answer != nil {
				e.answer <- false
			}
		}
		if u.started {
			<-u.done
		}
		wizardCall("DestroyWindow", hwnd)
	}()
	if err := u.createControls(); err != nil {
		return outcome{}, err
	}
	u.refreshGraphics()
	u.welcome()
	if wizardCall("SetTimer", hwnd, 1, 100, 0) == 0 {
		return outcome{}, fmt.Errorf("cannot create setup event timer")
	}
	defer wizardCall("KillTimer", hwnd, 1)
	wizardCall("ShowWindow", hwnd, 5)
	wizardCall("UpdateWindow", hwnd)
	var msg wizardMsg
	for {
		r, _, err := wizardUser.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) == -1 {
			return outcome{}, fmt.Errorf("read setup window events: %w", err)
		}
		if r == 0 {
			break
		}
		if msg.Message == 0x100 && msg.WParam == 0x1B {
			u.cancelAction()
			continue
		}
		if wizardCall("IsDialogMessageW", hwnd, uintptr(unsafe.Pointer(&msg))) != 0 {
			continue
		}
		wizardCall("TranslateMessage", uintptr(unsafe.Pointer(&msg)))
		wizardCall("DispatchMessageW", uintptr(unsafe.Pointer(&msg)))
	}
	return u.out, nil
}

func (u *nativeWizard) scale(n int) int { return (n*u.dpi + 48) / 96 }
func (u *nativeWizard) createControls() error {
	create := func(class, text string, style uintptr, id uintptr) (uintptr, error) {
		h, _, err := wizardUser.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(wide(class))), uintptr(unsafe.Pointer(wide(text))),
			0x40000000|0x10000000|style, 0, 0, 0, 0, u.window, id, u.instance, 0)
		if h == 0 {
			return 0, fmt.Errorf("create setup %s: %w", class, err)
		}
		return h, nil
	}
	var err error
	if u.title, err = create("STATIC", "", 0, 10); err != nil {
		return err
	}
	// A native edit allows selection/copy, keyboard scrolling and screen-reader
	// access to the full safety/recovery text, including unusually long paths.
	if u.body, err = create("EDIT", "", 0x00010000|0x00200000|0x0004|0x0040|0x0800, 11); err != nil {
		return err
	}
	if u.progress, err = create("msctls_progress32", "", 0x8, 12); err != nil {
		return err
	}
	if u.back, err = create("BUTTON", "< &Back", 0x00010000, wizardBack); err != nil {
		return err
	}
	if u.next, err = create("BUTTON", "&Next >", 0x00010000, wizardNext); err != nil {
		return err
	}
	if u.cancel, err = create("BUTTON", "Cancel", 0x00010000, wizardCancel); err != nil {
		return err
	}
	return nil
}
func (u *nativeWizard) releaseGraphics() {
	for _, h := range []uintptr{u.font, u.headingFont, u.contentBrush, u.sidebarBrush} {
		if h != 0 {
			wizardGDICall("DeleteObject", h)
		}
	}
	for _, h := range []uintptr{u.icon, u.smallIcon, u.sidebarIcon} {
		if h != 0 {
			wizardCall("DestroyIcon", h)
		}
	}
	u.font, u.headingFont, u.contentBrush, u.sidebarBrush = 0, 0, 0, 0
	u.icon, u.smallIcon, u.sidebarIcon = 0, 0, 0
}
func (u *nativeWizard) refreshGraphics() {
	u.releaseGraphics()
	u.contentColor, u.textColor, u.sidebarColor = 0x00FFFFFF, 0x00000000, 0x00904416
	hc := wizardContrast{Size: uint32(unsafe.Sizeof(wizardContrast{}))}
	wizardCall("SystemParametersInfoW", 0x42, uintptr(hc.Size), uintptr(unsafe.Pointer(&hc)), 0)
	if hc.Flags&1 != 0 {
		u.contentColor = uint32(wizardCall("GetSysColor", 5))
		u.textColor = uint32(wizardCall("GetSysColor", 8))
		u.sidebarColor = uint32(wizardCall("GetSysColor", 13))
	}
	u.contentBrush = wizardGDICall("CreateSolidBrush", uintptr(u.contentColor))
	u.sidebarBrush = wizardGDICall("CreateSolidBrush", uintptr(u.sidebarColor))
	font := func(size, weight int) uintptr {
		return wizardGDICall("CreateFontW", uintptr(-int32(u.scale(size))), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(wide("Segoe UI"))))
	}
	u.font, u.headingFont = font(13, 400), font(20, 600)
	for _, h := range []uintptr{u.body, u.back, u.next, u.cancel} {
		wizardCall("SendMessageW", h, 0x30, u.font, 1)
	}
	wizardCall("SendMessageW", u.title, 0x30, u.headingFont, 1)
	load := func(size int) uintptr {
		return wizardCall("LoadImageW", u.instance, 2, 1, uintptr(u.scale(size)), uintptr(u.scale(size)), 0)
	}
	u.icon, u.smallIcon, u.sidebarIcon = load(32), load(16), load(96)
	wizardCall("SendMessageW", u.window, 0x80, 1, u.icon)
	wizardCall("SendMessageW", u.window, 0x80, 0, u.smallIcon)
	u.layout()
	wizardCall("InvalidateRect", u.window, 0, 1)
}
func (u *nativeWizard) layout() {
	if u.title == 0 {
		return
	}
	var r wizardRect
	wizardCall("GetClientRect", u.window, uintptr(unsafe.Pointer(&r)))
	w, h := int(r.Right), int(r.Bottom)
	move := func(hwnd uintptr, x, y, width, height int) {
		wizardCall("MoveWindow", hwnd, uintptr(x), uintptr(y), uintptr(max(width, 1)), uintptr(max(height, 1)), 1)
	}
	side, pad, bottom := u.scale(150), u.scale(24), u.scale(62)
	x, width := side+pad, w-side-2*pad
	move(u.title, x, pad, width, u.scale(58))
	move(u.body, x, u.scale(90), width, h-bottom-u.scale(90)-pad-u.scale(22))
	move(u.progress, x, h-bottom-pad-u.scale(14), width, u.scale(14))
	buttonWidth, buttonHeight, gap := u.scale(84), u.scale(27), u.scale(8)
	buttonY := h - bottom + (bottom-buttonHeight)/2
	move(u.cancel, w-pad-buttonWidth, buttonY, buttonWidth, buttonHeight)
	move(u.next, w-pad-2*buttonWidth-gap, buttonY, buttonWidth, buttonHeight)
	move(u.back, w-pad-3*buttonWidth-2*gap, buttonY, buttonWidth, buttonHeight)
}
func (u *nativeWizard) display(title, body string) {
	wizardCall("SetWindowTextW", u.title, uintptr(unsafe.Pointer(wide(title))))
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n")
	wizardCall("SetWindowTextW", u.body, uintptr(unsafe.Pointer(wide(body))))
	u.buttons()
	wizardCall("InvalidateRect", u.window, 0, 1)
}
func (u *nativeWizard) welcome() {
	u.page = wizardWelcome
	u.display("Welcome to Agent Sessions Setup", "This wizard installs Agent Sessions for your Windows account.\n\nSetup checks requirements before asking for permission to remove an existing installation. Your settings and session data are kept.\n\nClick Next to review the installation details, or Cancel to leave setup.")
	wizardCall("SetFocus", u.next)
}
func (u *nativeWizard) showQuestion() {
	u.page = wizardQuestion
	// A double-click on welcome Next must not authorize removal when a fast
	// preflight presents the next page between the two clicks.
	wait := wizardCall("GetDoubleClickTime")
	u.nextAllowed = time.Now().Add(time.Duration(wait+100) * time.Millisecond)
	title := "Ready to install Agent Sessions"
	if u.pending.kind == promptClose {
		title = "Close Agent Sessions to continue"
	}
	u.display(title, u.pending.body)
	wizardCall("SetFocus", u.cancel)
}
func (u *nativeWizard) buttons() {
	question := u.page == wizardQuestion
	welcome := u.page == wizardWelcome
	result := u.page == wizardResult
	busy := !welcome && !question && !result
	back := u.pending != nil && u.canBack(true, u.pending.kind)
	wizardCall("EnableWindow", u.back, boolWord(back))
	nextText := "&Next >"
	if question {
		nextText = "&Install"
		if u.pending.kind == promptClose {
			nextText = "&Retry"
		}
	}
	if result {
		nextText = "&Finish"
	}
	wizardCall("SetWindowTextW", u.next, uintptr(unsafe.Pointer(wide(nextText))))
	wizardCall("EnableWindow", u.next, boolWord(u.canAdvance(time.Now())))
	wizardCall("EnableWindow", u.cancel, boolWord(welcome || question))
	defaultID := uintptr(wizardNext)
	if question || busy {
		defaultID = wizardCancel
	}
	for _, b := range []struct{ h, id uintptr }{{u.next, wizardNext}, {u.cancel, wizardCancel}, {u.back, wizardBack}} {
		style := uintptr(0)
		if b.id == defaultID {
			style = 1
		}
		wizardCall("SendMessageW", b.h, 0xF4, style, 1)
	}
	// Keep title-bar Close consistent with Cancel (except Finish on results).
	menu := wizardCall("GetSystemMenu", u.window, 0)
	flags := uintptr(0)
	if busy {
		flags = 1
	}
	wizardCall("EnableMenuItem", menu, 0xF060, flags)
	wizardCall("DrawMenuBar", u.window)
	show := uintptr(0)
	if busy {
		show = 5
	}
	wizardCall("ShowWindow", u.progress, show)
	wizardCall("SendMessageW", u.progress, 0x40A, boolWord(busy), 40)
}
func boolWord(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}
func (u *nativeWizard) answer(yes bool) {
	if u.pending == nil {
		return
	}
	q := u.pending
	u.pending = nil
	u.page = wizardWorking
	u.display("Preparing the next step", "Please wait while setup checks the installation state…")
	q.answer <- yes
}
func (u *nativeWizard) nextAction() {
	switch u.page {
	case wizardWelcome:
		if u.pending != nil {
			u.showQuestion()
			return
		}
		if !u.startWorker() {
			return
		}
		u.display("Preparing setup", "Checking installation requirements…")
		go func() {
			defer close(u.done)
			out := u.work(u)
			u.post(wizardEvent{result: &out})
		}()
	case wizardQuestion:
		if !time.Now().Before(u.nextAllowed) {
			u.answer(true)
		}
	case wizardResult:
		wizardCall("DestroyWindow", u.window)
	}
}
func (u *nativeWizard) cancelAction() {
	if !u.canCancel() {
		return
	}
	switch u.page {
	case wizardWelcome:
		if u.pending != nil {
			u.answer(false)
			return
		}
		if !u.started {
			u.out = outcome{Operation: "preflight", Status: 1602, Message: "Setup cancelled before removing anything."}
			wizardCall("DestroyWindow", u.window)
		}
	case wizardQuestion:
		u.answer(false)
	case wizardResult:
		wizardCall("DestroyWindow", u.window)
	}
}
func (u *nativeWizard) windowProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case 0x11: // WM_QUERYENDSESSION: never abandon a worker, including Ask.
		return boolWord(u.canEndSession())
	case 0x10: // WM_CLOSE
		u.cancelAction()
		return 0
	case 0x2: // WM_DESTROY; only our completed/cancelled paths destroy the window
		wizardCall("PostQuitMessage", 0)
		return 0
	case 0x111: // WM_COMMAND: only button notifications, not edit notifications
		if wp>>16 != 0 {
			break
		}
		switch wp & 0xFFFF {
		case wizardNext:
			u.nextAction()
		case wizardCancel:
			u.cancelAction()
		case wizardBack:
			if u.pending != nil && u.canBack(true, u.pending.kind) {
				u.welcome()
			}
		}
		return 0
	case wizardWake, 0x113: // WM_TIMER
		u.drain()
		if u.page == wizardQuestion {
			u.buttons()
		}
		return 0
	case 0x400: // DM_GETDEFID: IsDialogMessage's default Enter target
		id := uintptr(wizardNext)
		if u.page == wizardQuestion || u.page == wizardWorking {
			id = wizardCancel
		}
		return 0x534B0000 | id
	case 0x401: // DM_SETDEFID; the page, not focus changes, owns the safe default
		return 1
	case 0x5:
		u.layout()
		return 0 // WM_SIZE
	case 0x2E0: // WM_DPICHANGED
		u.dpi = int(wp & 0xFFFF)
		// lParam is an OS-owned RECT valid only during this callback. Copy it
		// through the native API rather than constructing a Go pointer from an
		// integer (which also keeps checkptr and go vet's unsafeptr happy).
		var r wizardRect
		windows.NewLazySystemDLL("ntdll.dll").NewProc("RtlMoveMemory").Call(uintptr(unsafe.Pointer(&r)), lp, unsafe.Sizeof(r))
		wizardCall("SetWindowPos", hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0x14)
		u.refreshGraphics()
		return 0
	case 0x1A, 0x31A: // settings / theme changed, including high contrast
		if u.title != 0 {
			u.refreshGraphics()
		}
	case 0x138, 0x133: // WM_CTLCOLORSTATIC / WM_CTLCOLOREDIT
		if lp == u.body || lp == u.title {
			wizardGDICall("SetTextColor", wp, uintptr(u.textColor))
			wizardGDICall("SetBkColor", wp, uintptr(u.contentColor))
			return u.contentBrush
		}
	case 0x14:
		return 1 // WM_ERASEBKGND, painted below
	case 0xF: // WM_PAINT
		var paint wizardPaint
		dc := wizardCall("BeginPaint", hwnd, uintptr(unsafe.Pointer(&paint)))
		var r wizardRect
		wizardCall("GetClientRect", hwnd, uintptr(unsafe.Pointer(&r)))
		bottom := r.Bottom - int32(u.scale(62))
		content := wizardRect{Right: r.Right, Bottom: bottom}
		wizardCall("FillRect", dc, uintptr(unsafe.Pointer(&content)), u.contentBrush)
		side := wizardRect{Right: int32(u.scale(150)), Bottom: bottom}
		wizardCall("FillRect", dc, uintptr(unsafe.Pointer(&side)), u.sidebarBrush)
		strip := wizardRect{Top: bottom, Right: r.Right, Bottom: r.Bottom}
		wizardCall("FillRect", dc, uintptr(unsafe.Pointer(&strip)), wizardCall("GetSysColorBrush", 15))
		line := wizardRect{Top: bottom, Right: r.Right, Bottom: bottom + 1}
		wizardCall("FillRect", dc, uintptr(unsafe.Pointer(&line)), wizardCall("GetSysColorBrush", 16))
		if u.sidebarIcon != 0 {
			wizardCall("DrawIconEx", dc, uintptr(u.scale(27)), uintptr(u.scale(38)), u.sidebarIcon, uintptr(u.scale(96)), uintptr(u.scale(96)), 0, 0, 3)
		}
		wizardCall("EndPaint", hwnd, uintptr(unsafe.Pointer(&paint)))
		return 0
	}
	return wizardCall("DefWindowProcW", hwnd, uintptr(msg), wp, lp)
}
