//go:build windows && desktop

package main

import (
	"runtime"
	"testing"
	"unsafe"
)

// TestTouchpadLayouts reads the report layout of each Precision Touchpad on
// this machine, as the swipe reader does. It skips where there is none.
func TestTouchpadLayouts(t *testing.T) {
	type rawInputDeviceList struct {
		Device uintptr
		Type   uint32
	}
	type ridDeviceInfoHID struct {
		Size, Type                         uint32
		VendorID, ProductID, VersionNumber uint32
		UsagePage, Usage                   uint16
		_                                  [8]byte // RID_DEVICE_INFO's union is 24 bytes
	}
	list := user32.NewProc("GetRawInputDeviceList")
	var n uint32
	entry := unsafe.Sizeof(rawInputDeviceList{})
	list.Call(0, uintptr(unsafe.Pointer(&n)), entry)
	if n == 0 {
		t.Skip("no raw input devices")
	}
	devices := make([]rawInputDeviceList, n)
	if r, _, err := list.Call(uintptr(unsafe.Pointer(&devices[0])), uintptr(unsafe.Pointer(&n)), entry); int32(r) < 0 {
		t.Fatalf("GetRawInputDeviceList: %v", err)
	}
	found := 0
	for _, d := range devices[:n] {
		if d.Type != rimTypeHID {
			continue
		}
		info := ridDeviceInfoHID{Size: uint32(unsafe.Sizeof(ridDeviceInfoHID{}))}
		size := info.Size
		if r, _, _ := procGetRawInputDeviceInfoW.Call(d.Device, 0x2000000b, uintptr(unsafe.Pointer(&info)), uintptr(unsafe.Pointer(&size))); int32(r) <= 0 {
			continue
		}
		t.Logf("HID %04x:%04x usage %#02x/%#02x", info.VendorID, info.ProductID, info.UsagePage, info.Usage)
		if info.UsagePage != pageDigitizer || info.Usage != usageTouchPad {
			continue
		}
		found++
		pad := openTouchpad(d.Device)
		if pad == nil {
			t.Errorf("touchpad %#x (%04x:%04x): layout not readable", d.Device, info.VendorID, info.ProductID)
			continue
		}
		t.Logf("touchpad %#x (%04x:%04x): %d finger slots %v, contact count %v (link %d), X %d..%d, Y %d..%d, %.1f x %.1f mm",
			d.Device, info.VendorID, info.ProductID, len(pad.slots), pad.slots, pad.count, pad.countLink,
			pad.x.LogicalMin, pad.x.LogicalMax, pad.y.LogicalMin, pad.y.LogicalMax,
			float64(pad.x.LogicalMax-pad.x.LogicalMin)*pad.sx, float64(pad.y.LogicalMax-pad.y.LogicalMin)*pad.sy)
	}
	if found == 0 {
		t.Skip("no Precision Touchpad")
	}
}

// TestTouchpadListen registers the swipe reader's window for touchpad input.
func TestTouchpadListen(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := listenTouchpads(); err != nil {
		t.Fatal(err)
	}
}
