package state

import (
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"
)

func TestExistingProfileACLIsNotChangedByMetadataUpdates(t *testing.T) {
	s := testStore(t)
	a := addAccount(t, s, "one")
	profile := s.ProfileDir(a)
	// Model an official profile directory whose ACL has been independently set.
	if err := privatePath(profile, false); err != nil {
		t.Fatal(err)
	}
	before := directorySDDL(t, profile)
	if err := s.Update(func(r *Registry) error { return r.Rename("one", "two", "") }); err != nil {
		t.Fatal(err)
	}
	if after := directorySDDL(t, profile); before != after {
		t.Fatal("metadata update changed official profile directory ACL")
	}
	if err := s.Update(func(r *Registry) error { return r.Remove("two") }); err != nil {
		t.Fatal(err)
	}
	if after := directorySDDL(t, filepath.Join(s.Root, "profiles", a.ID)); before != after {
		t.Fatal("remove changed official profile directory ACL")
	}
}

func directorySDDL(t *testing.T, path string) string {
	t.Helper()
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	var descriptor uintptr
	code, _, _ := advapi32.NewProc("GetNamedSecurityInfoW").Call(uintptr(unsafe.Pointer(name)), 1, 4, 0, 0, 0, 0, uintptr(unsafe.Pointer(&descriptor)))
	if code != 0 {
		t.Fatal(syscall.Errno(code))
	}
	defer localFree.Call(descriptor)
	var text *uint16
	ok, _, err := advapi32.NewProc("ConvertSecurityDescriptorToStringSecurityDescriptorW").Call(descriptor, 1, 4, uintptr(unsafe.Pointer(&text)), 0)
	if ok == 0 {
		t.Fatal(err)
	}
	defer localFree.Call(uintptr(unsafe.Pointer(text)))
	var chars []uint16
	for i := uintptr(0); ; i++ {
		ch := *(*uint16)(unsafe.Add(unsafe.Pointer(text), i*2))
		if ch == 0 {
			break
		}
		chars = append(chars, ch)
	}
	return syscall.UTF16ToString(chars)
}
