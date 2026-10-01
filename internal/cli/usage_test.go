package cli

import (
	"bytes"
	"fmt"
	"github.com/harveyxiacn/cc-router/internal/usage"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUsageThresholdUsesEitherWindow(t *testing.T) {
	now := time.Now().Unix()
	for _, tc := range []struct {
		five, seven float64
		want        string
	}{{94, 95, "SWITCH RECOMMENDED"}, {95, 20, "SWITCH RECOMMENDED"}, {90, 20, "PREPARE HANDOFF"}, {20, 30, "5h 20.0%"}} {
		input := fmt.Sprintf(`{"rate_limits":{"five_hour":{"used_percentage":%v,"resets_at":%d},"seven_day":{"used_percentage":%v,"resets_at":%d}},"transcript_path":"DO_NOT_READ","unknown":{"token":"DO_NOT_PRINT"}}`, tc.five, now+3600, tc.seven, now+86400)
		out := &bytes.Buffer{}
		a := &App{In: strings.NewReader(input), Out: out}
		code, err := a.Execute([]string{"usage", "statusline"})
		if err != nil || code != 0 || !strings.Contains(out.String(), tc.want) || strings.Contains(out.String(), "DO_NOT") {
			t.Fatalf("usage %d %v %s", code, err, out.String())
		}
	}
}

func TestStatuslineRecordsSanitizedAccountObservation(t *testing.T) {
	a := fixtureApp(t)
	command(t, a, "account", "add", "a")
	r, _ := a.Store.Load()
	account, _ := r.Find("a")
	t.Setenv("CCR_HOME", a.Store.Root)
	t.Setenv("CLAUDE_CONFIG_DIR", a.Store.ProfileDir(account))
	a.In = strings.NewReader(fmt.Sprintf(`{"rate_limits":{"five_hour":{"used_percentage":95,"resets_at":%d}},"transcript_path":"SECRET"}`, time.Now().Unix()+3600))
	command(t, a, "usage", "statusline", "--account-id", account.ID)
	snapshot, err := usage.Load(a.Store, account.ID)
	if err != nil || snapshot == nil || snapshot.FiveHour == nil {
		t.Fatalf("%+v %v", snapshot, err)
	}
	b, _ := os.ReadFile(filepath.Join(a.Store.Root, "usage", account.ID+".json"))
	if strings.Contains(string(b), "SECRET") {
		t.Fatal("raw telemetry persisted")
	}
}

func TestUsageUnknownNeverMeansZero(t *testing.T) {
	for _, input := range []string{`{}`, `{"rate_limits":null}`, `{"rate_limits":{"five_hour":{"used_percentage":null,"resets_at":9999999999}}}`, `{"rate_limits":{"five_hour":{"used_percentage":95,"resets_at":1}}}`, `{"rate_limits":{"five_hour":{"used_percentage":105,"resets_at":9999999999}}}`} {
		out := &bytes.Buffer{}
		a := &App{In: strings.NewReader(input), Out: out}
		_, err := a.Execute([]string{"usage", "statusline"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "unknown") || strings.Contains(out.String(), "0.0%") {
			t.Fatalf("false quota: %s", out.String())
		}
	}
}

func TestUsageInputLimitsAndCustomThreshold(t *testing.T) {
	for _, input := range []string{`{broken secret`, strings.Repeat(" ", 1024*1024+1), `{} {}`} {
		out := &bytes.Buffer{}
		a := &App{In: strings.NewReader(input), Out: out}
		if _, err := a.Execute([]string{"usage", "statusline"}); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("expected redacted parse error")
		}
	}
	now := time.Now().Unix()
	out := &bytes.Buffer{}
	a := &App{In: strings.NewReader(fmt.Sprintf(`{"rate_limits":{"five_hour":{"used_percentage":92,"resets_at":%d}}}`, now+3600)), Out: out}
	if _, err := a.Execute([]string{"usage", "statusline", "--switch-at", "92", "--prepare-at", "85"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "SWITCH RECOMMENDED") {
		t.Fatal(out.String())
	}
	a.In = strings.NewReader(`{}`)
	if _, err := a.Execute([]string{"usage", "statusline", "--switch-at", "90", "--prepare-at", "95"}); err == nil {
		t.Fatal("invalid threshold order")
	}
}
