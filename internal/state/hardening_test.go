package state

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionLocksAreExclusiveAndReleased(t *testing.T) {
	s := testStore(t)
	a := addAccount(t, s, "one")
	release, err := s.LockAccount(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LockAccount(a.ID); !errors.Is(err, ErrBusy) {
		t.Fatalf("second account lock: %v", err)
	}
	release()
	release()
	release, err = s.LockAccount(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	release()
	project := t.TempDir()
	release, err = s.LockProject(project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LockProject(filepath.Join(project, ".")); !errors.Is(err, ErrBusy) {
		t.Fatalf("second project lock: %v", err)
	}
	release()
	if _, err := s.LockAccount("../escape"); err == nil {
		t.Fatal("accepted unsafe lock ID")
	}
}

func TestSessionLockCrossProcess(t *testing.T) {
	if root := os.Getenv("CCR_SESSION_TEST_ROOT"); root != "" {
		s, err := Open(root)
		if err != nil {
			t.Fatal(err)
		}
		release, err := s.LockAccount(os.Getenv("CCR_SESSION_TEST_ID"))
		if os.Getenv("CCR_SESSION_TEST_EXPECT") == "busy" {
			if !errors.Is(err, ErrBusy) {
				if release != nil {
					release()
				}
				t.Fatalf("child did not observe busy lock: %v", err)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		release()
		return
	}
	s := testStore(t)
	a := addAccount(t, s, "one")
	release, err := s.LockAccount(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	run := func(expect string) {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestSessionLockCrossProcess$")
		cmd.Env = append(os.Environ(), "CCR_SESSION_TEST_ROOT="+s.Root, "CCR_SESSION_TEST_ID="+a.ID, "CCR_SESSION_TEST_EXPECT="+expect)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("child lock test: %v: %s", err, out)
		}
	}
	run("busy")
	release()
	run("available")
}

func TestDuplicateJSONKeysAndOversizedRegistryAreRejected(t *testing.T) {
	s := testStore(t)
	for _, data := range []string{`{"schemaVersion":1,"schemaVersion":1,"accounts":[]}`, `{"schemaVersion":1,"accounts":null}`, strings.Repeat(" ", MaxJSONSize+1)} {
		if err := os.WriteFile(filepath.Join(s.Root, "accounts.json"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Load(); err == nil {
			t.Fatal("accepted invalid registry")
		}
		if err := s.Update(func(r *Registry) error { _, err := r.Add("one", ""); return err }); err == nil {
			t.Fatal("overwrote invalid registry")
		}
		got, err := os.ReadFile(filepath.Join(s.Root, "accounts.json"))
		if err != nil || !bytes.Equal(got, []byte(data)) {
			t.Fatal("modified invalid registry")
		}
	}
}

func TestSymlinkMetadataCannotWriteOutsideRoot(t *testing.T) {
	s := testStore(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "data")
	if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(s.Root, "accounts.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := s.Update(func(r *Registry) error { _, err := r.Add("one", ""); return err }); err == nil {
		t.Fatal("accepted metadata symlink")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "preserve" {
		t.Fatal("modified external target")
	}
}

func TestProfileDirectorySymlinkCannotWriteOutsideRoot(t *testing.T) {
	s := testStore(t)
	a := addAccount(t, s, "one")
	dir := s.ProfileDir(a)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := os.Symlink(target, dir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := s.Update(func(r *Registry) error { return r.Rename("one", "two", "") }); err == nil {
		t.Fatal("accepted profile symlink")
	}
	r, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Find("one"); err != nil {
		t.Fatal("failed transaction persisted rename")
	}
}
