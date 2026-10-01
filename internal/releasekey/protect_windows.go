package releasekey

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

type blob struct {
	Size uint32
	Data *byte
}

func crypt(data []byte, decrypt bool) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty release key")
	}
	in := blob{uint32(len(data)), &data[0]}
	var out blob
	name := "CryptProtectData"
	if decrypt {
		name = "CryptUnprotectData"
	}
	proc := syscall.NewLazyDLL("crypt32.dll").NewProc(name)
	ok, _, _ := proc.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(data)
	if ok == 0 {
		return nil, errors.New("Windows could not protect/unprotect the release signing key for this user")
	}
	defer syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree").Call(uintptr(unsafe.Pointer(out.Data)))
	if out.Size > 4096 {
		return nil, errors.New("invalid DPAPI key length")
	}
	return append([]byte{}, unsafe.Slice(out.Data, int(out.Size))...), nil
}
func protect(b []byte) ([]byte, error)   { return crypt(b, false) }
func unprotect(b []byte) ([]byte, error) { return crypt(b, true) }
