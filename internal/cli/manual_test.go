package cli

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/harveyxiacn/cc-router/internal/usage"
)

func TestManualUsageCommandsRecordShowClear(t *testing.T) {
	a := fixtureApp(t)
	command(t, a, "account", "add", "a")
	reset := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	command(t, a, "usage", "record", "a", "--five-hour", "95", "--five-reset", reset)
	r, _ := a.Store.Load()
	account, _ := r.Find("a")
	manual, err := usage.LoadManual(a.Store, account.ID)
	if err != nil || manual == nil || manual.Source != "manual" || manual.FiveHour.UsedPercentage != 95 {
		t.Fatalf("%+v %v", manual, err)
	}
	out := &bytes.Buffer{}
	a.Out = out
	command(t, a, "usage", "show", "a")
	var report struct {
		Official *usage.Snapshot     `json:"official"`
		Manual   *usage.ManualRecord `json:"manual"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.Official != nil || report.Manual == nil {
		t.Fatalf("%s %v", out, err)
	}
	command(t, a, "usage", "clear", "a")
	if record, _ := usage.LoadManual(a.Store, account.ID); record != nil {
		t.Fatal("manual not cleared")
	}
}

func TestManualUsageCLIRejectsIncompleteAndDuplicateFlags(t *testing.T) {
	a := fixtureApp(t)
	command(t, a, "account", "add", "a")
	for _, flags := range [][]string{{"--five-hour", "95"}, {"--five-reset", time.Now().Add(time.Hour).Format(time.RFC3339)}, {"--limited-until", "bad"}, {"--limited-until", "2030-01-01T00:00:00Z", "--limited-until", "2030-01-02T00:00:00Z"}, {"--seven-day", "NaN", "--seven-reset", time.Now().Add(time.Hour).Format(time.RFC3339)}} {
		if _, err := a.Execute(append([]string{"usage", "record", "a"}, flags...)); err == nil {
			t.Fatal("accepted invalid manual flags")
		}
	}
}

func TestBackupCLIRequiresReviewedDigestAndRestoresBindings(t *testing.T) {
	a := fixtureApp(t)
	command(t, a, "account", "add", "a")
	command(t, a, "use", "a")
	command(t, a, "bind", "a")
	out := &bytes.Buffer{}
	a.Out = out
	command(t, a, "backup", "create")
	list, err := a.Store.ListBackups()
	if err != nil || len(list) != 1 {
		t.Fatalf("%+v %v", list, err)
	}
	preview, err := a.Store.PreviewBackup(list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	command(t, a, "account", "rename", "a", "renamed")
	if _, err := a.Execute([]string{"backup", "restore", list[0].ID}); err == nil {
		t.Fatal("unreviewed restore accepted")
	}
	command(t, a, "backup", "restore", list[0].ID, "--reviewed-digest", preview.Digest)
	r, err := a.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Find("a"); err != nil {
		t.Fatal("restore failed")
	}
}
