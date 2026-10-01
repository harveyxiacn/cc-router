package usage

import (
	"github.com/harveyxiacn/cc-router/internal/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUsageSnapshotPersistsOnlyAllowlistedData(t *testing.T) {
	s, err := state.Open(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	var a state.Account
	if err := s.Update(func(r *state.Registry) error { var e error; a, e = r.Add("a", "A"); return e }); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1800000000, 0)
	snapshot, err := Decode([]byte(`{"rate_limits":{"five_hour":{"used_percentage":95,"resets_at":1800003600},"seven_day":{"used_percentage":30,"resets_at":1800600000}},"transcript_path":"PRIVATE","token":"PRIVATE"}`), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(s, a.ID, snapshot); err != nil {
		t.Fatal(err)
	}
	got, err := Load(s, a.ID)
	if err != nil || got.FiveHour == nil || got.FiveHour.UsedPercentage != 95 {
		t.Fatalf("%+v %v", got, err)
	}
	b, err := os.ReadFile(filepath.Join(s.Root, "usage", a.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "PRIVATE") || strings.Contains(string(b), "transcript") {
		t.Fatal("persisted raw payload")
	}
	if err := Save(s, "../escape", snapshot); err == nil {
		t.Fatal("accepted escaping account")
	}
}

func TestUsageMissingAndExpiredWindowsStayUnknown(t *testing.T) {
	now := time.Unix(1800000000, 0)
	for _, input := range []string{`{}`, `{"rate_limits":{"five_hour":{"used_percentage":null,"resets_at":1800003600}}}`, `{"rate_limits":{"five_hour":{"used_percentage":101,"resets_at":1800003600}}}`, `{"rate_limits":{"five_hour":{"used_percentage":90,"resets_at":1}}}`} {
		s, err := Decode([]byte(input), now)
		if err != nil || s.FiveHour != nil || s.SevenDay != nil {
			t.Fatalf("%+v %v", s, err)
		}
	}
	for _, input := range []string{`null`, `[]`, `{"rate_limits":{},"rate_limits":{}}`, `{} {}`, strings.Repeat(" ", 1048577)} {
		if _, err := Decode([]byte(input), now); err == nil {
			t.Fatalf("accepted malformed input")
		}
	}
}

func TestCorruptCachedUsageIsNotPresentedAsValid(t *testing.T) {
	s, err := state.Open(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 32)
	if err := os.Mkdir(filepath.Join(s.Root, "usage"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		`{"observedAt":"2026-10-01T00:00:00Z","source":"official-statusline"} {}`,
		`{"observedAt":"2026-10-01T00:00:00Z","source":"official-statusline","source":"official-statusline"}`,
	} {
		if err := os.WriteFile(filepath.Join(s.Root, "usage", id+".json"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(s, id); err == nil {
			t.Fatal("accepted ambiguous cache")
		}
	}
}
