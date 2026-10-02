package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	service "github.com/harveyxiacn/cc-router/internal/desktop"
	"github.com/harveyxiacn/cc-router/internal/usage"
)

func TestNativeBindingsRecoverRegistryAndKeepManualUsageIndependent(t *testing.T) {
	t.Setenv("CCR_HOME", t.TempDir())
	a := NewApp(filepath.Join(t.TempDir(), "unused-cli"))
	if err := a.CreateAccount("personal", "个人"); err != nil {
		t.Fatal(err)
	}
	input := service.ManualInput{FiveHour: &usage.Window{UsedPercentage: 95, ResetsAt: time.Now().Add(time.Hour).Unix()}}
	if err := a.RecordManualUsage("personal", input); err != nil {
		t.Fatal(err)
	}
	snapshot, err := a.GetSnapshot("")
	if err != nil || len(snapshot.Accounts) != 1 || snapshot.Accounts[0].ManualUsage == nil || snapshot.Accounts[0].Usage != nil {
		t.Fatalf("manual/official separation: %+v %v", snapshot, err)
	}
	id := snapshot.Accounts[0].ID
	backup, err := a.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.backend.Store.Root, "accounts.json"), []byte("damaged registry"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetSnapshot(""); err == nil {
		t.Fatal("damaged registry unexpectedly loaded")
	}
	backups, err := a.ListBackups()
	if err != nil || len(backups) != 1 {
		t.Fatalf("recovery listing unavailable: %+v %v", backups, err)
	}
	preview, err := a.PreviewBackup(backup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.RestoreBackup(backup.ID, preview.Digest, false); err == nil {
		t.Fatal("restored without explicit review")
	}
	before, err := a.RestoreBackup(backup.ID, preview.Digest, true)
	if err != nil || before.Restorable {
		t.Fatalf("corrupt pre-restore evidence: %+v %v", before, err)
	}
	snapshot, err = a.GetSnapshot("")
	if err != nil || len(snapshot.Accounts) != 1 || snapshot.Accounts[0].ID != id || snapshot.Accounts[0].ManualUsage == nil {
		t.Fatalf("restored registry/retained observation: %+v %v", snapshot, err)
	}
	if err := a.ClearManualUsage("personal"); err != nil {
		t.Fatal(err)
	}
	snapshot, err = a.GetSnapshot("")
	if err != nil || snapshot.Accounts[0].ManualUsage != nil {
		t.Fatalf("clear manual record: %+v %v", snapshot, err)
	}
}
