//go:build windows && desktop

package main

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"

	"github.com/ginkcode/agent-sessions/internal/touchswipe"
)

// Two-finger touchpad swipes on Windows. WebView2 runs in another process,
// so the app's window never sees its scroll messages, and the page's wheel
// events change shape with the renderer's load (SN-09). So a message-only
// window on its own thread reads the Precision Touchpad's contacts through
// Raw Input; internal/touchswipe turns them into one swipe per touch. The
// mouse's side buttons need nothing: WebView2 reports them to the page as
// buttons 3 and 4.

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	hidDLL   = windows.NewLazySystemDLL("hid.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW        = user32.NewProc("RegisterClassExW")
	procCreateWindowExW         = user32.NewProc("CreateWindowExW")
	procDefWindowProcW          = user32.NewProc("DefWindowProcW")
	procGetMessageW             = user32.NewProc("GetMessageW")
	procDispatchMessageW        = user32.NewProc("DispatchMessageW")
	procRegisterRawInputDevices = user32.NewProc("RegisterRawInputDevices")
	procGetRawInputData         = user32.NewProc("GetRawInputData")
	procGetRawInputDeviceInfoW  = user32.NewProc("GetRawInputDeviceInfoW")
	procGetCursorPos            = user32.NewProc("GetCursorPos")
	procWindowFromPoint         = user32.NewProc("WindowFromPoint")
	procGetAncestor             = user32.NewProc("GetAncestor")
	procGetModuleHandleW        = kernel32.NewProc("GetModuleHandleW")

	procHidPGetCaps       = hidDLL.NewProc("HidP_GetCaps")
	procHidPGetValueCaps  = hidDLL.NewProc("HidP_GetValueCaps")
	procHidPGetUsageValue = hidDLL.NewProc("HidP_GetUsageValue")
	procHidPGetUsages     = hidDLL.NewProc("HidP_GetUsages")
)

const (
	wmInput             = 0x00FF
	wmInputDeviceChange = 0x00FE
	gidcRemoval         = 2
	hwndMessage         = ^uintptr(2) // HWND_MESSAGE, (HWND)-3
	gaRoot              = 2
	ridevInputSink      = 0x00000100
	ridevDevNotify      = 0x00002000
	ridInput            = 0x10000003
	rimTypeHID          = 2
	ridiPreparsedData   = 0x20000005
	hidpInput           = 0
	hidpStatusSuccess   = 0x00110000

	pageGenericDesktop = 0x01
	pageDigitizer      = 0x0D
	usageTouchPad      = 0x05
	usageX             = 0x30
	usageY             = 0x31
	usageTipSwitch     = 0x42
	usageContactID     = 0x51
	usageContactCount  = 0x54
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type winMsg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
	Private uint32
}

type rawInputDevice struct {
	UsagePage uint16
	Usage     uint16
	Flags     uint32
	Target    uintptr
}

type rawInputHeader struct {
	Type   uint32
	Size   uint32
	Device uintptr
	WParam uintptr
}

// hidpCaps is HIDP_CAPS.
type hidpCaps struct {
	Usage                     uint16
	UsagePage                 uint16
	InputReportByteLength     uint16
	OutputReportByteLength    uint16
	FeatureReportByteLength   uint16
	Reserved                  [17]uint16
	NumberLinkCollectionNodes uint16
	NumberInputButtonCaps     uint16
	NumberInputValueCaps      uint16
	NumberInputDataIndices    uint16
	NumberOutputButtonCaps    uint16
	NumberOutputValueCaps     uint16
	NumberOutputDataIndices   uint16
	NumberFeatureButtonCaps   uint16
	NumberFeatureValueCaps    uint16
	NumberFeatureDataIndices  uint16
}

// hidpValueCaps is HIDP_VALUE_CAPS (72 bytes). UsageMin is NotRange.Usage
// when IsRange is 0.
type hidpValueCaps struct {
	UsagePage         uint16
	ReportID          uint8
	IsAlias           uint8
	BitField          uint16
	LinkCollection    uint16
	LinkUsage         uint16
	LinkUsagePage     uint16
	IsRange           uint8
	IsStringRange     uint8
	IsDesignatorRange uint8
	IsAbsolute        uint8
	HasNull           uint8
	Reserved          uint8
	BitSize           uint16
	ReportCount       uint16
	Reserved2         [5]uint16
	UnitsExp          uint32
	Units             uint32
	LogicalMin        int32
	LogicalMax        int32
	PhysicalMin       int32
	PhysicalMax       int32
	UsageMin          uint16
	UsageMax          uint16
	_                 [12]byte
}

// The HID structs must match hidpi.h, whatever the architecture.
var (
	_ [72 - unsafe.Sizeof(hidpValueCaps{})]byte
	_ [unsafe.Sizeof(hidpValueCaps{}) - 72]byte
	_ [64 - unsafe.Sizeof(hidpCaps{})]byte
	_ [unsafe.Sizeof(hidpCaps{}) - 64]byte
)

func (c *hidpValueCaps) covers(page, usage uint16) bool {
	if c.UsagePage != page {
		return false
	}
	if c.IsRange != 0 {
		return usage >= c.UsageMin && usage <= c.UsageMax
	}
	return c.UsageMin == usage
}

func (c *hidpValueCaps) axis() touchswipe.HIDAxis {
	return touchswipe.HIDAxis{
		LogicalMin: c.LogicalMin, LogicalMax: c.LogicalMax,
		PhysicalMin: c.PhysicalMin, PhysicalMax: c.PhysicalMax,
		Units: c.Units, UnitsExp: c.UnitsExp,
	}
}

// hookMouseNavigation starts reading touchpads; the side buttons need
// nothing on Windows.
func hookMouseNavigation() {
	if traceNavigation {
		path := filepath.Join(os.TempDir(), "agent-sessions-nav-trace.log")
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			navTraceOut = f
		}
	}
	go readTouchpads()
}

func hookWindowMouseNavigation(*application.WebviewWindow) {}

// touchpads is the reader thread's state: only its window procedure uses it.
var touchpads = struct {
	devices map[uintptr]*touchpad // nil for devices that are not touchpads
	buf     []byte
}{devices: map[uintptr]*touchpad{}}

func readTouchpads() {
	// The window, its messages and Raw Input all belong to this thread.
	runtime.LockOSThread()
	if err := listenTouchpads(); err != nil {
		traceNav("touchpad: %v", err)
		return
	}
	var m winMsg
	for {
		if r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); int32(r) <= 0 {
			return
		}
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func listenTouchpads() error {
	instance, _, _ := procGetModuleHandleW.Call(0)
	name, _ := windows.UTF16PtrFromString("AgentSessionsTouchpad")
	class := wndClassEx{WndProc: windows.NewCallback(touchpadWndProc), Instance: instance, ClassName: name}
	class.Size = uint32(unsafe.Sizeof(class))
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); r == 0 {
		return errors.Join(errors.New("register window class"), err)
	}
	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(name)), 0, 0, 0, 0, 0, 0, hwndMessage, 0, instance, 0)
	if hwnd == 0 {
		return errors.Join(errors.New("create message window"), err)
	}
	// INPUTSINK: a message-only window is never in the foreground.
	device := rawInputDevice{UsagePage: pageDigitizer, Usage: usageTouchPad, Flags: ridevInputSink | ridevDevNotify, Target: hwnd}
	if r, _, err := procRegisterRawInputDevices.Call(uintptr(unsafe.Pointer(&device)), 1, unsafe.Sizeof(device)); r == 0 {
		return errors.Join(errors.New("register for touchpad input"), err)
	}
	traceNav("touchpad: listening")
	return nil
}

func touchpadWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmInput:
		touchpadInput(lParam)
	case wmInputDeviceChange:
		if wParam == gidcRemoval {
			delete(touchpads.devices, lParam)
		}
	}
	// WM_INPUT needs DefWindowProc too, to clean up.
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func touchpadInput(handle uintptr) {
	headerSize := unsafe.Sizeof(rawInputHeader{})
	var size uint32
	procGetRawInputData.Call(handle, ridInput, 0, uintptr(unsafe.Pointer(&size)), headerSize)
	if size < uint32(headerSize)+8 {
		return
	}
	if cap(touchpads.buf) < int(size) {
		touchpads.buf = make([]byte, size)
	}
	buf := touchpads.buf[:size]
	if r, _, _ := procGetRawInputData.Call(handle, ridInput, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), headerSize); int32(r) <= 0 {
		return
	}
	header := (*rawInputHeader)(unsafe.Pointer(&buf[0]))
	if header.Type != rimTypeHID {
		return
	}
	pad, known := touchpads.devices[header.Device]
	if !known {
		pad = openTouchpad(header.Device)
		touchpads.devices[header.Device] = pad
	}
	if pad == nil {
		return
	}
	// RAWHID: dwSizeHid, dwCount, then dwCount reports.
	body := buf[headerSize:]
	each := int(binary.LittleEndian.Uint32(body))
	count := int(binary.LittleEndian.Uint32(body[4:]))
	reports := body[8:]
	for i := 0; i < count && each > 0 && (i+1)*each <= len(reports); i++ {
		pad.report(reports[i*each : (i+1)*each])
	}
}

// touchpad is one Precision Touchpad: how to read its reports, and its
// frames and touch so far.
type touchpad struct {
	device    uintptr
	preparsed []byte
	slots     []uint16 // finger link collections, in report order
	count     bool     // whether it reports a contact count
	countLink uint16
	x, y      hidpValueCaps
	sx, sy    float64
	frames    touchswipe.Frames
	tracker   touchswipe.Tracker
	contacts  []touchswipe.Contact
}

// openTouchpad reads a device's report layout, or returns nil if it is no
// touchpad this can read.
func openTouchpad(device uintptr) *touchpad {
	var size uint32
	procGetRawInputDeviceInfoW.Call(device, ridiPreparsedData, 0, uintptr(unsafe.Pointer(&size)))
	if size == 0 {
		return nil
	}
	pad := &touchpad{device: device, preparsed: make([]byte, size)}
	if r, _, _ := procGetRawInputDeviceInfoW.Call(device, ridiPreparsedData, uintptr(unsafe.Pointer(&pad.preparsed[0])), uintptr(unsafe.Pointer(&size))); int32(r) <= 0 {
		return nil
	}
	var caps hidpCaps
	if r, _, _ := procHidPGetCaps.Call(uintptr(unsafe.Pointer(&pad.preparsed[0])), uintptr(unsafe.Pointer(&caps))); uint32(r) != hidpStatusSuccess || caps.NumberInputValueCaps == 0 {
		return nil
	}
	n := caps.NumberInputValueCaps
	values := make([]hidpValueCaps, n)
	if r, _, _ := procHidPGetValueCaps.Call(hidpInput, uintptr(unsafe.Pointer(&values[0])), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&pad.preparsed[0]))); uint32(r) != hidpStatusSuccess {
		return nil
	}
	xs, ys := map[uint16]hidpValueCaps{}, map[uint16]hidpValueCaps{}
	for _, c := range values[:n] {
		switch {
		case c.covers(pageDigitizer, usageContactID):
			pad.slots = append(pad.slots, c.LinkCollection)
		case c.covers(pageDigitizer, usageContactCount):
			pad.count, pad.countLink = true, c.LinkCollection
		case c.covers(pageGenericDesktop, usageX):
			xs[c.LinkCollection] = c
		case c.covers(pageGenericDesktop, usageY):
			ys[c.LinkCollection] = c
		}
	}
	slices.Sort(pad.slots)
	pad.slots = slices.Compact(pad.slots)
	pad.slots = slices.DeleteFunc(pad.slots, func(link uint16) bool {
		_, hasX := xs[link]
		_, hasY := ys[link]
		return !hasX || !hasY
	})
	if len(pad.slots) == 0 {
		traceNav("touchpad %#x: no fingers in its reports, ignored", device)
		return nil
	}
	pad.x, pad.y = xs[pad.slots[0]], ys[pad.slots[0]]
	pad.sx, pad.sy = touchswipe.Scales(pad.x.axis(), pad.y.axis())
	traceNav("touchpad %#x: %d fingers per report, contact count %v, %.0f x %.0f mm", device, len(pad.slots), pad.count,
		float64(pad.x.LogicalMax-pad.x.LogicalMin)*pad.sx, float64(pad.y.LogicalMax-pad.y.LogicalMin)*pad.sy)
	return pad
}

// value reads one usage from report; false if report does not have it.
func (p *touchpad) value(report []byte, page, link, usage uint16) (uint32, bool) {
	var v uint32
	r, _, _ := procHidPGetUsageValue.Call(hidpInput, uintptr(page), uintptr(link), uintptr(usage), uintptr(unsafe.Pointer(&v)),
		uintptr(unsafe.Pointer(&p.preparsed[0])), uintptr(unsafe.Pointer(&report[0])), uintptr(len(report)))
	return v, uint32(r) == hidpStatusSuccess
}

// tip reads whether a finger slot's tip switch is on.
func (p *touchpad) tip(report []byte, link uint16) bool {
	var usages [16]uint16
	n := uint32(len(usages))
	r, _, _ := procHidPGetUsages.Call(hidpInput, pageDigitizer, uintptr(link), uintptr(unsafe.Pointer(&usages[0])), uintptr(unsafe.Pointer(&n)),
		uintptr(unsafe.Pointer(&p.preparsed[0])), uintptr(unsafe.Pointer(&report[0])), uintptr(len(report)))
	return uint32(r) == hidpStatusSuccess && slices.Contains(usages[:min(n, uint32(len(usages)))], usageTipSwitch)
}

// coordinate turns a raw X or Y into logical units, sign-extended if the
// axis can be negative.
func coordinate(raw uint32, c *hidpValueCaps) float64 {
	v := int64(raw)
	if c.LogicalMin < 0 && c.BitSize > 0 && c.BitSize < 32 {
		shift := 64 - c.BitSize
		v = v << shift >> shift
	}
	return float64(v - int64(c.LogicalMin))
}

func (p *touchpad) report(report []byte) {
	if len(report) == 0 {
		return
	}
	p.contacts = p.contacts[:0]
	for _, link := range p.slots {
		id, ok := p.value(report, pageDigitizer, link, usageContactID)
		x, okX := p.value(report, pageGenericDesktop, link, usageX)
		y, okY := p.value(report, pageGenericDesktop, link, usageY)
		if !ok || !okX || !okY {
			break
		}
		p.contacts = append(p.contacts, touchswipe.Contact{
			ID: id, Tip: p.tip(report, link),
			X: coordinate(x, &p.x) * p.sx, Y: coordinate(y, &p.y) * p.sy,
		})
	}
	count := len(p.contacts) // no contact count: each report is a whole frame
	if p.count {
		c, ok := p.value(report, pageDigitizer, p.countLink, usageContactCount)
		if !ok && len(p.contacts) == 0 {
			return // another kind of report
		}
		count = int(c)
	}
	frame, ok := p.frames.Report(count, p.contacts)
	if !ok {
		return
	}
	direction := p.tracker.Frame(time.Now(), frame)
	if down, changed := p.tracker.Down(); changed {
		traceNav("touchpad %#x: %d fingers down", p.device, down)
	}
	if direction != touchswipe.None {
		traceNav("touchpad %#x: swipe %s", p.device, direction)
		swipeUnderCursor(direction.String())
	}
}

// swipeUnderCursor sends the swipe to the app window under the cursor, as
// Windows sends the scroll there; nowhere if that is another app's window.
func swipeUnderCursor(direction string) {
	var pt struct{ X, Y int32 }
	if r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); r == 0 {
		return
	}
	// WindowFromPoint takes the POINT by value, in one register on amd64 and arm64.
	h, _, _ := procWindowFromPoint.Call(uintptr(uint32(pt.X)) | uintptr(uint32(pt.Y))<<32)
	root, _, _ := procGetAncestor.Call(h, gaRoot)
	if root == 0 {
		return
	}
	// Off this thread: emitting runs script in the web view.
	go func() {
		app := application.Get()
		if app == nil {
			return
		}
		for _, w := range app.Window.GetAll() {
			if ww, ok := w.(*application.WebviewWindow); ok && uintptr(ww.NativeWindow()) == root {
				dispatchNavigation(ww, swipeNavigateEvent, direction, true)
				return
			}
		}
		traceNav("touchpad swipe %s: no app window under the cursor", direction)
	}()
}
