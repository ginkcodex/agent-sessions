//go:build windows

package main

import (
	"fmt"
	"runtime"
	"strings"
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

// Read-only MSI database access: no installation, registry changes, or custom
// actions. ALLUSERS must be absent/empty, matching this project's perUser MSI.
func validateMSI(path, expectedVersion string) error {
	var db uintptr
	status, _, _ := msiOpenDatabase.Call(uintptr(unsafe.Pointer(wide(path))), 0, uintptr(unsafe.Pointer(&db)))
	if status != 0 {
		return msiError("open embedded MSI read-only", status)
	}
	defer msiCloseHandle.Call(db)
	properties := make(map[string]string)
	for _, name := range []string{"ProductCode", "UpgradeCode", "ProductVersion", "ALLUSERS", "MSIINSTALLPERUSER"} {
		value, err := databaseProperty(db, name)
		if err != nil {
			return err
		}
		properties[name] = value
	}
	if !validGUID(properties["ProductCode"]) || !strings.EqualFold(properties["UpgradeCode"], upgradeCode) {
		return fmt.Errorf("embedded MSI has an invalid ProductCode or does not belong to Agent Sessions")
	}
	if properties["ALLUSERS"] != "" || (properties["MSIINSTALLPERUSER"] != "" && properties["MSIINSTALLPERUSER"] != "1") {
		return fmt.Errorf("embedded MSI is not an unmanaged per-user package")
	}
	version, err := parseVersion(properties["ProductVersion"])
	if err != nil {
		return err
	}
	expected, err := parseVersion(expectedVersion)
	if err != nil {
		return err
	}
	if version != expected {
		return fmt.Errorf("embedded MSI version %s differs from bundled version %s", properties["ProductVersion"], expectedVersion)
	}
	return nil
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

func installedProducts() ([]product, error) {
	// Both MSI enumerators require successive calls from the same thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var products []product
	seen := make(map[string]bool)
	for index := uint32(0); ; index++ {
		var code [39]uint16
		status, _, _ := msiEnumRelated.Call(uintptr(unsafe.Pointer(wide(upgradeCode))), 0, uintptr(index), uintptr(unsafe.Pointer(&code[0])))
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
			products = append(products, product{Code: id, Version: version, SID: userSID, Context: context})
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

// Called only after full metadata validation, to verify the exact registration
// after installation, not merely another member of the same product family.
func payloadProductCode(path string) (string, error) {
	var db uintptr
	status, _, _ := msiOpenDatabase.Call(uintptr(unsafe.Pointer(wide(path))), 0, uintptr(unsafe.Pointer(&db)))
	if status != 0 {
		return "", msiError("open payload identity", status)
	}
	defer msiCloseHandle.Call(db)
	return databaseProperty(db, "ProductCode")
}
