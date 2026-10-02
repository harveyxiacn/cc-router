package desktop

import (
	"github.com/harveyxiacn/cc-router/internal/usage"
	"testing"
	"time"
)

func TestManualRecordVisibleSeparatelyAndClearable(t *testing.T) {
	s := testService(t)
	if err := s.CreateAccount("a", "A"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordManualUsage("a", ManualInput{LimitedUntil: time.Now().Add(time.Hour).Unix()}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.GetSnapshot("")
	if err != nil {
		t.Fatal(err)
	}
	account := snapshot.Accounts[0]
	if account.Usage != nil || account.ManualUsage == nil || account.ManualUsage.Source != "manual" {
		t.Fatalf("%+v", account)
	}
	if err := s.ClearManualUsage("a"); err != nil {
		t.Fatal(err)
	}
	if got, err := usage.LoadManual(s.Store, account.ID); err != nil || got != nil {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestDesktopRestoreRequiresReviewAndRejectsActiveSession(t *testing.T) {
	s := testService(t)
	if err := s.CreateAccount("a", "A"); err != nil {
		t.Fatal(err)
	}
	info, err := s.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewBackup(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RestoreBackup(info.ID, preview.Digest, false); err == nil {
		t.Fatal("unreviewed restore accepted")
	}
	unlock, err := s.Store.LockAccount(preview.Accounts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.RestoreBackup(info.ID, preview.Digest, true)
	unlock()
	if err == nil {
		t.Fatal("restored active account")
	}
	if _, err = s.RestoreBackup(info.ID, preview.Digest, true); err != nil {
		t.Fatal(err)
	}
}
