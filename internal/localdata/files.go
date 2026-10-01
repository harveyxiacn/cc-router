// Package localdata provides bounded, atomic access to tool-owned local files.
package localdata

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
)

func Read(root *os.Root, name string, limit int64) ([]byte, error) {
	st, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > limit {
		return nil, errors.New("local file is not regular or exceeds the size limit")
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("local file exceeds size limit")
	}
	return b, nil
}

func Write(root *os.Root, name string, data []byte) error {
	if st, err := root.Lstat(name); err == nil {
		if !st.Mode().IsRegular() {
			return errors.New("local file must be regular")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(name), ".cc-router-"+hex.EncodeToString(id[:])+".tmp")
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return root.Rename(tmp, name)
}
