package update

import (
	"errors"
	"testing"
)

func TestInstallationLeaseAllowsSessionsAndExcludesUpdater(t *testing.T) {
	root := t.TempDir()
	a, err := AcquireLease(root, false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := AcquireLease(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLease(root, true); !errors.Is(err, ErrBusy) {
		t.Fatalf("update did not wait for readers: %v", err)
	}
	a()
	b()
	w, err := AcquireLease(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLease(root, false); !errors.Is(err, ErrBusy) {
		t.Fatalf("new session entered installation: %v", err)
	}
	w()
	r, err := AcquireLease(root, false)
	if err != nil {
		t.Fatal(err)
	}
	r()
}
