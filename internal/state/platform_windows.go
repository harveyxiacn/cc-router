package state

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var lockFileEx = kernel32.NewProc("LockFileEx")
var unlockFileEx = kernel32.NewProc("UnlockFileEx")
var localFree = kernel32.NewProc("LocalFree")
var advapi32 = syscall.NewLazyDLL("advapi32.dll")
var convertSecurityDescriptor = advapi32.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
var getDACL = advapi32.NewProc("GetSecurityDescriptorDacl")
var setSecurity = advapi32.NewProc("SetNamedSecurityInfoW")

func lockFile(f *os.File) error {
	var overlapped syscall.Overlapped
	result, _, err := lockFileEx.Call(f.Fd(), 2, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	runtime.KeepAlive(f)
	if result == 0 {
		return err
	}
	return nil
}
func tryLockFile(f *os.File) error {
	var overlapped syscall.Overlapped
	result, _, err := lockFileEx.Call(f.Fd(), 3, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	runtime.KeepAlive(f)
	if result == 0 {
		if errors.Is(err, syscall.Errno(33)) {
			return ErrBusy
		}
		return err
	}
	return nil
}
func unlockFile(f *os.File) {
	var overlapped syscall.Overlapped
	unlockFileEx.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	runtime.KeepAlive(f)
}

// Give the current Windows user full control and disable inherited permissions.
// The directory ACL is inheritable so official profile files remain private.
func privatePath(path string, directory bool) error {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	sid, err := user.User.Sid.String()
	if err != nil {
		return err
	}
	inherit := ""
	if directory {
		inherit = "OICI"
	}
	sddl, err := syscall.UTF16PtrFromString("D:P(A;" + inherit + ";FA;;;" + sid + ")")
	if err != nil {
		return err
	}
	var descriptor uintptr
	ok, _, callErr := convertSecurityDescriptor.Call(uintptr(unsafe.Pointer(sddl)), 1, uintptr(unsafe.Pointer(&descriptor)), 0)
	if ok == 0 {
		return callErr
	}
	defer localFree.Call(descriptor)
	var present, defaulted uint32
	var dacl uintptr
	ok, _, callErr = getDACL.Call(descriptor, uintptr(unsafe.Pointer(&present)), uintptr(unsafe.Pointer(&dacl)), uintptr(unsafe.Pointer(&defaulted)))
	if ok == 0 {
		return callErr
	}
	if present == 0 || dacl == 0 {
		return errors.New("private ACL is missing")
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	code, _, _ := setSecurity.Call(uintptr(unsafe.Pointer(name)), 1, 0x80000004, 0, 0, dacl, 0)
	runtime.KeepAlive(name)
	runtime.KeepAlive(sddl)
	if code != 0 {
		return syscall.Errno(code)
	}
	return nil
}
func syncRoot(root *os.Root) error { return nil }
