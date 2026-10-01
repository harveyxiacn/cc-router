package update

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/harveyxiacn/cc-router/internal/localdata"
)

var ErrBusy = errors.New("another application session or update is using this installation")
var ErrReadOnly = errors.New("automatic updates require a writable portable installation")

// AcquireLease takes a shared application lease or an exclusive installation lease.
func AcquireLease(directory string, exclusive bool) (func(), error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	const name = ".cc-router-update.lock"
	if st, err := root.Lstat(name); err == nil {
		if !st.Mode().IsRegular() {
			return nil, errors.New("installation lease must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := localdata.OpenLockFile(root, name)
	if err != nil && !exclusive {
		f, err = root.Open(name)
	}
	if err != nil {
		return nil, ErrReadOnly
	}
	if err := tryLease(f, exclusive); err != nil {
		f.Close()
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { releaseLease(f); f.Close() }) }, nil
}

type Layout struct{ Root, GUI, CLI string }

func Installation(gui, cli string) (Layout, error) {
	var result Layout
	if !filepath.IsAbs(gui) || !filepath.IsAbs(cli) {
		return result, errors.New("update paths must be absolute")
	}
	gui = filepath.Clean(gui)
	cli = filepath.Clean(cli)
	name := "cc-router-desktop"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if filepath.Base(gui) != name {
		return result, errors.New("automatic updates are available in the packaged desktop app")
	}
	root := filepath.Dir(gui)
	if runtime.GOOS == "darwin" {
		if filepath.Base(root) != "MacOS" || filepath.Base(filepath.Dir(root)) != "Contents" {
			return result, errors.New("macOS updates require the portable .app bundle")
		}
		bundle := filepath.Dir(filepath.Dir(root))
		if !strings.HasSuffix(bundle, ".app") {
			return result, errors.New("invalid macOS bundle")
		}
		root = filepath.Dir(bundle)
	}
	g, err := filepath.Rel(root, gui)
	if err != nil {
		return result, err
	}
	c, err := filepath.Rel(root, cli)
	if err != nil {
		return result, err
	}
	if c == ".." || strings.HasPrefix(c, ".."+string(filepath.Separator)) {
		return result, errors.New("the companion CLI must be in the portable desktop installation")
	}
	return Layout{Root: root, GUI: filepath.ToSlash(g), CLI: filepath.ToSlash(c)}, nil
}

// CompanionLease protects installed files for ordinary CLI invocations. Standalone
// CLI packages have no desktop to update and need no installation lease.
func CompanionLease() (func(), error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	gui := "cc-router-desktop"
	if runtime.GOOS == "windows" {
		gui += ".exe"
	}
	path := filepath.Join(filepath.Dir(exe), gui)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return func() {}, nil
	} else if err != nil {
		return nil, err
	}
	layout, err := Installation(path, exe)
	if err != nil {
		return nil, err
	}
	unlock, err := AcquireLease(layout.Root, false)
	if errors.Is(err, ErrReadOnly) {
		return func() {}, nil
	} // This location cannot be auto-updated either.
	return unlock, err
}
