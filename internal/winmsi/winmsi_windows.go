//go:build windows

package winmsi

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	msiDLL              = windows.NewLazySystemDLL("msi.dll")
	msiOpenDatabase     = msiDLL.NewProc("MsiOpenDatabaseW")
	msiDatabaseOpenView = msiDLL.NewProc("MsiDatabaseOpenViewW")
	msiViewExecute      = msiDLL.NewProc("MsiViewExecute")
	msiViewFetch        = msiDLL.NewProc("MsiViewFetch")
	msiRecordGetString  = msiDLL.NewProc("MsiRecordGetStringW")
	msiCloseHandle      = msiDLL.NewProc("MsiCloseHandle")
	msiEnumRelated      = msiDLL.NewProc("MsiEnumRelatedProductsW")
	msiEnumProductsEx   = msiDLL.NewProc("MsiEnumProductsExW")
	msiGetProductInfoEx = msiDLL.NewProc("MsiGetProductInfoExW")
)

func wide(s string) *uint16 { return windows.StringToUTF16Ptr(s) }
func msiError(operation string, status uintptr) error {
	return fmt.Errorf("%s: Windows Installer status %d", operation, status)
}

// ReadInfo reads an MSI's identity from its Property table. It opens the
// database read-only: nothing is installed, no registry is changed and no
// custom action runs.
func ReadInfo(path string) (Info, error) {
	var db uintptr
	status, _, _ := msiOpenDatabase.Call(uintptr(unsafe.Pointer(wide(path))), 0, uintptr(unsafe.Pointer(&db)))
	if status != 0 {
		return Info{}, msiError("open MSI read-only", status)
	}
	defer msiCloseHandle.Call(db)
	var info Info
	for name, field := range map[string]*string{
		"ProductCode": &info.ProductCode, "UpgradeCode": &info.UpgradeCode, "ProductName": &info.ProductName,
		"ProductVersion": &info.ProductVersion, "ALLUSERS": &info.AllUsers, "MSIINSTALLPERUSER": &info.PerUser,
	} {
		value, err := databaseProperty(db, name)
		if err != nil {
			return Info{}, err
		}
		*field = value
	}
	return info, nil
}

// Validate checks that the MSI at path is an unmanaged per-user Agent
// Sessions package of the expected MSI version.
func Validate(path, expectedVersion string) error {
	info, err := ReadInfo(path)
	if err != nil {
		return err
	}
	return info.Check(expectedVersion)
}

func databaseProperty(db uintptr, name string) (string, error) {
	// name comes only from the fixed property allowlist above.
	query := "SELECT `Value` FROM `Property` WHERE `Property`='" + name + "'"
	var view uintptr
	status, _, _ := msiDatabaseOpenView.Call(db, uintptr(unsafe.Pointer(wide(query))), uintptr(unsafe.Pointer(&view)))
	if status != 0 {
		return "", msiError("open property query", status)
	}
	defer msiCloseHandle.Call(view)
	status, _, _ = msiViewExecute.Call(view, 0)
	if status != 0 {
		return "", msiError("execute property query", status)
	}
	var record uintptr
	status, _, _ = msiViewFetch.Call(view, uintptr(unsafe.Pointer(&record)))
	if status == 259 {
		return "", nil
	}
	if status != 0 {
		return "", msiError("fetch property", status)
	}
	defer msiCloseHandle.Call(record)
	return msiString(func(buf *uint16, n *uint32) uintptr {
		s, _, _ := msiRecordGetString.Call(record, 1, uintptr(unsafe.Pointer(buf)), uintptr(unsafe.Pointer(n)))
		return s
	})
}

func msiString(read func(*uint16, *uint32) uintptr) (string, error) {
	buf := make([]uint16, 256)
	for {
		n := uint32(len(buf))
		status := read(&buf[0], &n)
		if status == 234 {
			if n > 1<<20 {
				return "", fmt.Errorf("oversized MSI string")
			}
			buf = make([]uint16, n+1)
			continue
		}
		if status != 0 {
			return "", msiError("read MSI string", status)
		}
		return windows.UTF16ToString(buf), nil
	}
}

// InstalledProducts lists the registrations of Agent Sessions that the
// current user can see, failing on any that is not fully installed.
func InstalledProducts() ([]Product, error) {
	// Both MSI enumerators require successive calls from the same thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var products []Product
	seen := make(map[string]bool)
	for index := uint32(0); ; index++ {
		var code [39]uint16
		status, _, _ := msiEnumRelated.Call(uintptr(unsafe.Pointer(wide(UpgradeCode))), 0, uintptr(index), uintptr(unsafe.Pointer(&code[0])))
		if status == 259 {
			break
		}
		if status != 0 {
			return nil, msiError("enumerate related products", status)
		}
		id := windows.UTF16ToString(code[:])
		if seen[id] {
			continue
		}
		seen[id] = true
		// NULL SID enumerates current-user and machine contexts, not an
		// arbitrary user's HKCU. Unprivileged setup cannot inspect every other
		// user's private registrations. Never request all-user enumeration or
		// elevate to gain access to them.
		for contextIndex := uint32(0); ; contextIndex++ {
			var actualCode [39]uint16
			var context uint32
			sid := make([]uint16, 256)
			n := uint32(len(sid))
			for {
				n = uint32(len(sid))
				status, _, _ = msiEnumProductsEx.Call(uintptr(unsafe.Pointer(wide(id))), 0, 7, uintptr(contextIndex), uintptr(unsafe.Pointer(&actualCode[0])), uintptr(unsafe.Pointer(&context)), uintptr(unsafe.Pointer(&sid[0])), uintptr(unsafe.Pointer(&n)))
				if status != 234 {
					break
				}
				if n > 4096 {
					return nil, fmt.Errorf("oversized product SID")
				}
				sid = make([]uint16, n+1)
			}
			if status == 259 {
				break
			}
			if status != 0 {
				return nil, msiError("enumerate product contexts", status)
			}
			userSID := windows.UTF16ToString(sid)
			var sidPtr *uint16
			if context != 4 {
				sidPtr = wide(userSID)
			}
			state, err := msiString(func(buf *uint16, size *uint32) uintptr {
				s, _, _ := msiGetProductInfoEx.Call(uintptr(unsafe.Pointer(wide(id))), uintptr(unsafe.Pointer(sidPtr)), uintptr(context), uintptr(unsafe.Pointer(wide("State"))), uintptr(unsafe.Pointer(buf)), uintptr(unsafe.Pointer(size)))
				return s
			})
			if err != nil {
				return nil, err
			}
			if state != "5" {
				return nil, fmt.Errorf("related product %s is advertised or not fully installed (state %q); remove it with its original installer first", id, state)
			}
			version, err := msiString(func(buf *uint16, size *uint32) uintptr {
				s, _, _ := msiGetProductInfoEx.Call(uintptr(unsafe.Pointer(wide(id))), uintptr(unsafe.Pointer(sidPtr)), uintptr(context), uintptr(unsafe.Pointer(wide("VersionString"))), uintptr(unsafe.Pointer(buf)), uintptr(unsafe.Pointer(size)))
				return s
			})
			if err != nil {
				return nil, err
			}
			products = append(products, Product{Code: id, Version: version, SID: userSID, Context: context})
		}
		found := false
		for _, product := range products {
			if product.Code == id {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("related product %s disappeared during context discovery; rerun setup", id)
		}
	}
	return products, nil
}

// ProductCode reads an MSI's ProductCode. Setup calls it only after full
// validation, to verify the exact registration after installation, not merely
// another member of the same product family.
func ProductCode(path string) (string, error) {
	var db uintptr
	status, _, _ := msiOpenDatabase.Call(uintptr(unsafe.Pointer(wide(path))), 0, uintptr(unsafe.Pointer(&db)))
	if status != 0 {
		return "", msiError("open payload identity", status)
	}
	defer msiCloseHandle.Call(db)
	return databaseProperty(db, "ProductCode")
}
