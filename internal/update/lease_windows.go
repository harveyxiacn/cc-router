package update

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

func tryLease(f *os.File, exclusive bool) error {
	flags := uintptr(1)
	if exclusive {
		flags |= 2
	}
	var ov syscall.Overlapped
	ok, _, err := syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx").Call(f.Fd(), flags, 0, 1, 0, uintptr(unsafe.Pointer(&ov)))
	runtime.KeepAlive(f)
	if ok == 0 {
		if errors.Is(err, syscall.Errno(33)) {
			return ErrBusy
		}
		return err
	}
	return nil
}
func releaseLease(f *os.File) {
	var ov syscall.Overlapped
	syscall.NewLazyDLL("kernel32.dll").NewProc("UnlockFileEx").Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&ov)))
	runtime.KeepAlive(f)
}
