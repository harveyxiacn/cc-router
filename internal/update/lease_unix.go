//go:build linux || darwin

package update

import (
	"errors"
	"os"
	"syscall"
)

func tryLease(f *os.File, exclusive bool) error {
	flags := syscall.LOCK_SH
	if exclusive {
		flags = syscall.LOCK_EX
	}
	err := syscall.Flock(int(f.Fd()), flags|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return ErrBusy
	}
	return err
}
func releaseLease(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
