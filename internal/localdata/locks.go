package localdata

import (
	"errors"
	"os"
)

// OpenLockFile opens a persistent regular lock file without truncating it.
// Exclusive first creation avoids Darwin's concurrent O_CREAT/non-O_EXCL race
// (golang/go#81246). Once a creator wins, all other callers use ordinary opens.
func OpenLockFile(root *os.Root, name string) (*os.File, error) {
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		return f, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("lock must be a regular file")
	}
	return root.OpenFile(name, os.O_RDWR, 0600)
}
