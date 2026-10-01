package update

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fixtureFile(t *testing.T, root, name, content string) File {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	return File{Path: name, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(content)), Mode: 0755}
}
func TestTransactionRestoresBothProgramsWhenHealthCheckFails(t *testing.T) {
	install, stage := t.TempDir(), t.TempDir()
	fixtureFile(t, install, "cc-router-desktop", "old gui")
	fixtureFile(t, install, "cc-router", "old cli")
	fixtureFile(t, install, "unrelated.txt", "preserve")
	files := []File{fixtureFile(t, stage, "cc-router-desktop", "new gui"), fixtureFile(t, stage, "cc-router", "new cli"), fixtureFile(t, stage, "README.md", "new docs")}
	txn, err := beginTransaction(install, "11111111111111111111111111111111", stage, files)
	if err != nil {
		t.Fatal(err)
	}
	if err := txn.install(); err != nil {
		t.Fatal(err)
	}
	if err := txn.finish(func() error { return errors.New("startup timeout") }); err == nil {
		t.Fatal("health failure accepted")
	}
	for name, want := range map[string]string{"cc-router-desktop": "old gui", "cc-router": "old cli", "unrelated.txt": "preserve"} {
		got, _ := os.ReadFile(filepath.Join(install, name))
		if string(got) != want {
			t.Fatalf("%s=%q", name, got)
		}
	}
	if _, err := os.Stat(filepath.Join(install, "README.md")); !os.IsNotExist(err) {
		t.Fatal("new file remained after rollback")
	}
}
func TestTransactionKeepsBackupAndCanRecoverInterruptedInstall(t *testing.T) {
	install, stage := t.TempDir(), t.TempDir()
	fixtureFile(t, install, "cc-router", "old")
	files := []File{fixtureFile(t, stage, "cc-router", "new")}
	txn, err := beginTransaction(install, "22222222222222222222222222222222", stage, files)
	if err != nil {
		t.Fatal(err)
	}
	if err := txn.install(); err != nil {
		t.Fatal(err)
	}
	// Reopen from durable journal, as a helper restarted after interruption would.
	reopened, err := openTransaction(install, txn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.rollback(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(install, "cc-router"))
	if string(got) != "old" {
		t.Fatalf("recovery=%q", got)
	}
}
func TestInvalidStagedFileCannotPartiallyInstall(t *testing.T) {
	install, stage := t.TempDir(), t.TempDir()
	fixtureFile(t, install, "cc-router", "old")
	file := fixtureFile(t, stage, "cc-router", "new")
	file.SHA256 = "bad"
	if _, err := beginTransaction(install, "33333333333333333333333333333333", stage, []File{file}); err == nil {
		t.Fatal("accepted tampered staged file")
	}
	got, _ := os.ReadFile(filepath.Join(install, "cc-router"))
	if string(got) != "old" {
		t.Fatal("changed original before validation")
	}
}
