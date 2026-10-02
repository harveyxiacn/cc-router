package update

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installerTerminalFixture(t *testing.T, phase string) (string, *transaction) {
	t.Helper()
	install, stage := t.TempDir(), t.TempDir()
	fixtureFile(t, install, "bin/cc-router", "old CLI")
	fixtureFile(t, install, "cc-router-desktop", "old GUI")
	files := []File{fixtureFile(t, stage, "bin/cc-router", "new CLI"), fixtureFile(t, stage, "cc-router-desktop", "new GUI")}
	txn, err := beginTransaction(install, strings.Repeat("1", 32), stage, files)
	if err != nil {
		t.Fatal(err)
	}
	if err := txn.install(); err != nil {
		t.Fatal(err)
	}
	if phase == "complete" {
		if err := txn.finish(func() error { return nil }); err != nil {
			t.Fatal(err)
		}
	} else if err := txn.rollback(); err != nil {
		t.Fatal(err)
	}
	return install, txn
}

func TestRetireInstallerStateRemovesOnlyTerminalWorkData(t *testing.T) {
	for _, phase := range []string{"complete", "rolled-back"} {
		t.Run(phase, func(t *testing.T) {
			install, txn := installerTerminalFixture(t, phase)
			data := t.TempDir()
			sentinel := filepath.Join(data, "profile-sentinel")
			if err := os.WriteFile(sentinel, []byte("private data"), 0600); err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(filepath.Join(install, workDirectory))
			if err != nil {
				t.Fatal(err)
			}
			err = jsonWrite(root, "active.json", reference{ID: txn.ID, DataRoot: data})
			root.Close()
			if err != nil {
				t.Fatal(err)
			}
			app, err := os.ReadFile(filepath.Join(install, "bin", "cc-router"))
			if err != nil {
				t.Fatal(err)
			}
			fixtureFile(t, install, "unrelated/user-file", "keep")
			if err := RetireInstallerState(install); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(filepath.Join(install, workDirectory)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("work directory retained: %v", err)
			}
			got, err := os.ReadFile(filepath.Join(install, "bin", "cc-router"))
			if err != nil || !bytes.Equal(got, app) {
				t.Fatal("application binary changed")
			}
			got, err = os.ReadFile(sentinel)
			if err != nil || string(got) != "private data" {
				t.Fatal("data root touched")
			}
			got, err = os.ReadFile(filepath.Join(install, "unrelated", "user-file"))
			if err != nil || string(got) != "keep" {
				t.Fatal("unrelated installation files changed")
			}
			if err := RetireInstallerState(install); err != nil {
				t.Fatal("missing work directory must be no-op:", err)
			}
		})
	}
}

func TestRetireInstallerStateRequiresOwnExclusiveLease(t *testing.T) {
	for _, exclusive := range []bool{false, true} {
		t.Run(map[bool]string{false: "reader", true: "writer"}[exclusive], func(t *testing.T) {
			install, _ := installerTerminalFixture(t, "complete")
			release, err := AcquireLease(install, exclusive)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			if err := RetireInstallerState(install); !errors.Is(err, ErrBusy) {
				t.Fatalf("retired with held lease: %v", err)
			}
			if _, err := os.Stat(filepath.Join(install, workDirectory)); err != nil {
				t.Fatal("busy retirement removed work")
			}
		})
	}
}

func TestRetireInstallerStateRefusesUnrecognizedDataBeforeDeleting(t *testing.T) {
	attacks := []string{"incomplete", "missing-journal", "corrupt", "future-schema", "unknown-field", "duplicate-key", "trailing-json", "oversize-journal", "unsafe-path", "case-duplicate", "bad-hash", "bad-old-path", "active-invalid", "active-missing-transaction", "unexpected-root-file", "unexpected-nested-file", "unexpected-directory"}
	for _, attack := range attacks {
		t.Run(attack, func(t *testing.T) {
			install, txn := installerTerminalFixture(t, "complete")
			work := filepath.Join(install, workDirectory)
			journalPath := filepath.Join(work, txn.ID, "journal.json")
			original, err := os.ReadFile(journalPath)
			if err != nil {
				t.Fatal(err)
			}
			raw := append([]byte{}, original...)
			switch attack {
			case "incomplete":
				txn.Phase = "probation"
				raw, _ = json.Marshal(txn)
			case "missing-journal":
				if err := os.Remove(journalPath); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				raw = []byte("{broken")
			case "future-schema":
				txn.Schema = 99
				raw, _ = json.Marshal(txn)
			case "unknown-field":
				raw = append([]byte(`{"secret":"no",`), original[1:]...)
			case "duplicate-key":
				raw = append([]byte(`{"schema":1,`), original[1:]...)
			case "trailing-json":
				raw = append(raw, []byte(`{}`)...)
			case "oversize-journal":
				raw = bytes.Repeat([]byte("x"), 1024*1024+1)
			case "unsafe-path":
				txn.Entries[0].New.Path = "../escape"
				raw, _ = json.Marshal(txn)
			case "case-duplicate":
				txn.Entries = append(txn.Entries, journalEntry{New: txn.Entries[0].New})
				raw, _ = json.Marshal(txn)
			case "bad-hash":
				txn.Entries[0].New.SHA256 = "invalid"
				raw, _ = json.Marshal(txn)
			case "bad-old-path":
				txn.Entries[0].Old.Path = "unrelated"
				raw, _ = json.Marshal(txn)
			case "active-invalid":
				if err := os.WriteFile(filepath.Join(work, "active.json"), []byte(`{"id":"../bad","dataRoot":"relative"}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "active-missing-transaction":
				data, _ := json.Marshal(reference{ID: strings.Repeat("f", 32), DataRoot: t.TempDir()})
				if err := os.WriteFile(filepath.Join(work, "active.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
			case "unexpected-root-file":
				fixtureFile(t, work, "unrelated.txt", "keep")
			case "unexpected-nested-file":
				fixtureFile(t, work, txn.ID+"/backup/bin/user-file", "keep")
			case "unexpected-directory":
				if err := os.Mkdir(filepath.Join(work, txn.ID, "user-directory"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if attack != "missing-journal" {
				if err := os.WriteFile(journalPath, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(filepath.Join(work, txn.ID, "backup", "bin", "cc-router"))
			if err != nil {
				t.Fatal(err)
			}
			if err := RetireInstallerState(install); err == nil {
				t.Fatal("accepted unrecognized update data")
			}
			after, err := os.ReadFile(filepath.Join(work, txn.ID, "backup", "bin", "cc-router"))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed preflight partially removed work data")
			}
		})
	}
}

func TestRetireInstallerStateRejectsLinksAndInvalidInstallation(t *testing.T) {
	if err := RetireInstallerState("relative"); err == nil {
		t.Fatal("accepted relative installation")
	}
	for _, target := range []string{"installation", "work", "journal", "backup-directory", "backup-file"} {
		t.Run(target, func(t *testing.T) {
			install, txn := installerTerminalFixture(t, "complete")
			outside := t.TempDir()
			path := install
			destination := outside
			switch target {
			case "installation":
				path = filepath.Join(t.TempDir(), "linked-install")
				destination = install
			case "work":
				path = filepath.Join(install, workDirectory)
				if err := os.Rename(path, path+"-saved"); err != nil {
					t.Fatal(err)
				}
			case "journal":
				path = filepath.Join(install, workDirectory, txn.ID, "journal.json")
				destination = filepath.Join(outside, "journal")
				if err := os.WriteFile(destination, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "backup-directory":
				path = filepath.Join(install, workDirectory, txn.ID, "backup", "bin")
				if err := os.Rename(path, path+"-saved"); err != nil {
					t.Fatal(err)
				}
			case "backup-file":
				path = filepath.Join(install, workDirectory, txn.ID, "backup", "bin", "cc-router")
				destination = filepath.Join(outside, "file")
				if err := os.WriteFile(destination, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(destination, path); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			if target == "installation" {
				install = path
			}
			if err := RetireInstallerState(install); err == nil {
				t.Fatal("accepted symlink")
			}
		})
	}
}
