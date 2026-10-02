package usage

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/harveyxiacn/cc-router/internal/state"
)

func manualFixture(t *testing.T) (*state.Store, state.Account) {
	t.Helper()
	s, err := state.Open(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	var account state.Account
	if err = s.Update(func(r *state.Registry) error { var e error; account, e = r.Add("a", "A"); return e }); err != nil {
		t.Fatal(err)
	}
	return s, account
}

func TestUsageWritesDoNotRewriteRegistryOrInspectProfileDirectories(t *testing.T) {
	s, a := manualFixture(t)
	registryPath := filepath.Join(s.Root, "accounts.json")
	before, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Unix(1000000, 0)
	if err := os.Chtimes(registryPath, old, old); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.ProfileDir(a)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.ProfileDir(a), []byte("profile sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := SaveManual(s, a.ID, ManualInput{LimitedUntil: now.Add(time.Hour).Unix()}, now); err != nil {
		t.Fatalf("manual mutation reported failure after saving: %v", err)
	}
	if err := Save(s, a.ID, Snapshot{ObservedAt: now, FiveHour: &Window{UsedPercentage: 20, ResetsAt: now.Add(time.Hour).Unix()}}); err != nil {
		t.Fatalf("official mutation reported failure after saving: %v", err)
	}
	if err := ClearManual(s, a.ID); err != nil {
		t.Fatalf("manual deletion reported failure after removing: %v", err)
	}
	after, err := os.ReadFile(registryPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("usage mutation changed registry")
	}
	current, err := os.Stat(registryPath)
	if err != nil || !current.ModTime().Equal(info.ModTime()) {
		t.Fatal("usage mutation rewrote registry")
	}
	profile, err := os.ReadFile(s.ProfileDir(a))
	if err != nil || string(profile) != "profile sentinel" {
		t.Fatal("usage mutation touched profile path")
	}
	if manual, err := LoadManual(s, a.ID); err != nil || manual != nil {
		t.Fatal("manual clear failed")
	}
	if official, err := Load(s, a.ID); err != nil || official == nil || official.FiveHour.UsedPercentage != 20 {
		t.Fatal("official record was lost")
	}
}

func TestManualRecordIndependentOfOfficialObservation(t *testing.T) {
	s, a := manualFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	official := Snapshot{ObservedAt: now, Source: "official-statusline", FiveHour: &Window{20, now.Add(time.Hour).Unix()}}
	if err := Save(s, a.ID, official); err != nil {
		t.Fatal(err)
	}
	input := ManualInput{SevenDay: &Window{95, now.Add(24 * time.Hour).Unix()}, LimitedUntil: now.Add(time.Hour).Unix()}
	if err := SaveManual(s, a.ID, input, now); err != nil {
		t.Fatal(err)
	}
	record, err := LoadManual(s, a.ID)
	if err != nil || record == nil || record.Source != "manual" || !record.ObservedAt.Equal(now) || record.SevenDay.UsedPercentage != 95 || record.FiveHour != nil {
		t.Fatalf("%+v %v", record, err)
	}
	got, err := Load(s, a.ID)
	if err != nil || got.Source != "official-statusline" || got.FiveHour.UsedPercentage != 20 {
		t.Fatalf("official changed: %+v %v", got, err)
	}
	if err := ClearManual(s, a.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadManual(s, a.ID); err != nil || got != nil {
		t.Fatalf("clear: %+v %v", got, err)
	}
	if got, _ := Load(s, a.ID); got == nil || got.FiveHour.UsedPercentage != 20 {
		t.Fatal("clear removed official data")
	}
}

func TestManualRecordRejectsInvalidAndUnregisteredInputs(t *testing.T) {
	s, a := manualFixture(t)
	now := time.Now().UTC()
	for _, input := range []ManualInput{
		{}, {FiveHour: &Window{-1, now.Add(time.Hour).Unix()}}, {FiveHour: &Window{101, now.Add(time.Hour).Unix()}},
		{FiveHour: &Window{math.NaN(), now.Add(time.Hour).Unix()}}, {SevenDay: &Window{math.Inf(1), now.Add(time.Hour).Unix()}},
		{FiveHour: &Window{95, now.Unix()}}, {LimitedUntil: now.Unix() - 1}, {LimitedUntil: now.AddDate(2, 0, 0).Unix()},
	} {
		if err := SaveManual(s, a.ID, input, now); err == nil {
			t.Fatalf("accepted invalid input: %+v", input)
		}
	}
	valid := ManualInput{LimitedUntil: now.Add(time.Hour).Unix()}
	for _, id := range []string{"../escape", strings.Repeat("b", 32)} {
		if err := SaveManual(s, id, valid, now); err == nil {
			t.Fatal("accepted unregistered id")
		}
	}
	if err := s.Update(func(r *state.Registry) error { return r.Remove("a") }); err != nil {
		t.Fatal(err)
	}
	if err := ClearManual(s, a.ID); err == nil {
		t.Fatal("cleared removed account")
	}
}

func TestManualRecordKeepsExpiredHistoryButRejectsAmbiguousData(t *testing.T) {
	s, a := manualFixture(t)
	old := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	if err := SaveManual(s, a.ID, ManualInput{FiveHour: &Window{95, old.Add(time.Hour).Unix()}}, old); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadManual(s, a.ID); err != nil || got == nil || got.FiveHour.ResetsAt >= time.Now().Unix() {
		t.Fatalf("expired record lost: %+v %v", got, err)
	}
	p := filepath.Join(s.Root, "usage", "manual-"+a.ID+".json")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(string(b), `"manual"`, `"official-statusline"`, 1), strings.Replace(string(b), `"source":"manual"`, `"source":"manual","token":"secret"`, 1), string(b) + `{}`, strings.Replace(string(b), `"source":"manual"`, `"source":"manual","source":"manual"`, 1)} {
		if err := os.WriteFile(p, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadManual(s, a.ID); err == nil {
			t.Fatal("accepted malformed manual record")
		}
	}
}

func TestManualRecordRejectsSymlinkDirectory(t *testing.T) {
	s, a := manualFixture(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(s.Root, "usage")); err != nil {
		t.Skip("OS denied symlink creation")
	}
	now := time.Now()
	if err := SaveManual(s, a.ID, ManualInput{LimitedUntil: now.Add(time.Hour).Unix()}, now); err == nil {
		t.Fatal("followed linked directory")
	}
	if _, err := LoadManual(s, a.ID); err == nil {
		t.Fatal("read linked directory")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("wrote outside private root")
	}
}
