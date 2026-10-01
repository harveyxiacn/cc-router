// Package releasekey stores the maintainer's signing key outside the source tree.
package releasekey

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"github.com/harveyxiacn/cc-router/internal/localdata"
	"github.com/harveyxiacn/cc-router/internal/state"
	"os"
	"path/filepath"
)

func Key(create bool) (ed25519.PrivateKey, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	s, err := state.Open(filepath.Join(base, "cc-router-maintainer", "release-signing"))
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, err := localdata.Read(root, "private-key.protected", 4096)
	if errors.Is(err, os.ErrNotExist) && create {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		data, err = protect(key)
		if err != nil {
			return nil, err
		}
		// Exclusive creation prevents accidental rotation by concurrent generators.
		f, err := root.OpenFile("private-key.protected", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		_, err = f.Write(data)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		return key, nil
	}
	if err != nil {
		return nil, err
	}
	plain, err := unprotect(data)
	if err != nil {
		return nil, err
	}
	if len(plain) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid protected release key")
	}
	return ed25519.PrivateKey(plain), nil
}
