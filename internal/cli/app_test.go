package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/harveyxiacn/cc-router/internal/state"
)

func fixtureApp(t *testing.T) *App {
	t.Helper()
	s, err := state.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	return &App{Store: s, Dir: t.TempDir(), In: strings.NewReader(""), Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
}
func command(t *testing.T, a *App, args ...string) {
	t.Helper()
	code, err := a.Execute(args)
	if err != nil || code != 0 {
		t.Fatalf("%v => %d %v", args, code, err)
	}
}

func TestAccountCommandsAndSelection(t *testing.T) {
	a := fixtureApp(t)
	command(t, a, "init")
	command(t, a, "account", "add", "a", "--label", "Personal")
	command(t, a, "account", "add", "b")
	command(t, a, "use", "a")
	command(t, a, "bind", "b")
	var gotProfile string
	var gotArgs []string
	a.Launch = func(profile, dir string, args []string) (int, error) {
		gotProfile = profile
		gotArgs = append([]string{}, args...)
		if dir != a.Dir {
			t.Error("changed cwd")
		}
		return 17, nil
	}
	r, _ := a.Store.Load()
	b, _ := r.Find("b")
	first, _ := r.Find("a")
	code, err := a.Execute([]string{"run", "--", "--continue", "a b", "中文", "$(not-a-shell)"})
	if err != nil || code != 17 || gotProfile != a.Store.ProfileDir(b) || !reflect.DeepEqual(gotArgs, []string{"--continue", "a b", "中文", "$(not-a-shell)"}) {
		t.Fatalf("launch: %d %v %s %v", code, err, gotProfile, gotArgs)
	}
	code, err = a.Execute([]string{"run", "a", "--", "--resume"})
	if err != nil || code != 17 || gotProfile != a.Store.ProfileDir(first) {
		t.Fatal("explicit selection did not win")
	}
	command(t, a, "account", "rename", "a", "personal", "--label", "Private")
	r, _ = a.Store.Load()
	renamed, _ := r.Find("personal")
	if first.ID != renamed.ID {
		t.Fatal("rename changed identity")
	}
	command(t, a, "unbind")
	command(t, a, "account", "list")
	command(t, a, "account", "remove", "b")
	if _, err := os.Stat(a.Store.ProfileDir(b)); err != nil {
		t.Fatal("removed official profile")
	}
}

func TestDesktopLaunchRejectsRecreatedAccountName(t *testing.T) {
	a := fixtureApp(t)
	command(t, a, "account", "add", "a")
	r, _ := a.Store.Load()
	original, _ := r.Find("a")
	a.ExpectedAccountID = original.ID
	command(t, a, "account", "remove", "a")
	command(t, a, "account", "add", "a")
	called := false
	a.Launch = func(string, string, []string) (int, error) { called = true; return 0, nil }
	if _, err := a.Execute([]string{"run", "a"}); err == nil || called {
		t.Fatal("launched a different account with reused name")
	}
}

func TestSelectionPromptDoesNotChangeGlobalDefault(t *testing.T) {
	a := fixtureApp(t)
	command(t, a, "account", "add", "a")
	command(t, a, "account", "add", "b")
	a.Interactive = true
	a.In = strings.NewReader("2\n")
	a.Launch = func(profile, dir string, args []string) (int, error) {
		r, _ := a.Store.Load()
		b, _ := r.Find("b")
		if profile != a.Store.ProfileDir(b) {
			t.Error("wrong selection")
		}
		return 0, nil
	}
	command(t, a, "run")
	r, _ := a.Store.Load()
	if r.DefaultID != "" {
		t.Fatal("prompt changed default")
	}
	a.Interactive = false
	if _, err := a.Execute([]string{"run"}); err == nil {
		t.Fatal("noninteractive launch guessed account")
	}
}

func TestLoginUsesOfficialClaudeAIFlow(t *testing.T) {
	a := fixtureApp(t)
	command(t, a, "account", "add", "a")
	a.Launch = func(profile, dir string, args []string) (int, error) {
		if !reflect.DeepEqual(args, []string{"auth", "login", "--claudeai"}) {
			t.Errorf("login args %v", args)
		}
		return 0, nil
	}
	command(t, a, "login", "a")
	if _, err := a.Execute([]string{"login", "a", "--console"}); err == nil {
		t.Fatal("allowed alternate billing")
	}
}

func TestSwitchRequiresHandoffAndNewSession(t *testing.T) {
	a := fixtureApp(t)
	command(t, a, "account", "add", "a")
	called := false
	a.Launch = func(profile, dir string, args []string) (int, error) {
		called = true
		if len(args) != 0 {
			t.Error("replayed args")
		}
		return 0, nil
	}
	if _, err := a.Execute([]string{"switch", "a"}); err == nil || called {
		t.Fatal("switch skipped handoff acknowledgment")
	}
	path := filepath.Join(a.Dir, ".cc-router", "handoff.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatal("missing template")
	}
	command(t, a, "switch", "a", "--handoff-reviewed")
	if !called {
		t.Fatal("did not start new session")
	}
	if _, err := a.Execute([]string{"switch", "a", "--handoff-reviewed", "--", "--resume"}); err == nil {
		t.Fatal("cross-profile resume allowed by switch")
	}
}

func TestExportImportAndInvalidArguments(t *testing.T) {
	a := fixtureApp(t)
	command(t, a, "account", "add", "a")
	command(t, a, "use", "a")
	a.Out = &bytes.Buffer{}
	command(t, a, "export")
	exported := a.Out.(*bytes.Buffer).String()
	if strings.Contains(exported, a.Store.Root) {
		t.Fatal("export leaked path")
	}
	other := fixtureApp(t)
	other.In = strings.NewReader(exported)
	command(t, other, "import", "-")
	r, _ := other.Store.Load()
	if _, err := r.Find("a"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"unknown"}, {"run", "a", "--resume"}, {"account", "add", "bad", "--unknown", "v"}, {"use"}, {"export", "unexpected"}, {"init", "extra"}} {
		if _, err := a.Execute(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	a.Launch = func(string, string, []string) (int, error) { return 0, errors.New("test launch failure") }
	if _, err := a.Execute([]string{"run", "a"}); err == nil {
		t.Fatal("hid launch failure")
	}
}

func TestActiveSessionsPreventRemoveAndConcurrentLaunch(t *testing.T) {
	a := fixtureApp(t)
	command(t, a, "account", "add", "a")
	command(t, a, "account", "add", "b")
	r, _ := a.Store.Load()
	account, _ := r.Find("a")
	unlock, err := a.Store.LockAccount(account.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"account", "remove", "a"}, {"account", "rename", "a", "other"}, {"run", "a"}} {
		if _, err := a.Execute(args); !errors.Is(err, state.ErrBusy) {
			t.Fatalf("%v: expected busy, got %v", args, err)
		}
	}
	unlock()
	projectUnlock, err := a.Store.LockProject(a.Dir)
	if err != nil {
		t.Fatal(err)
	}
	defer projectUnlock()
	for _, args := range [][]string{{"run", "a"}, {"run", "b"}, {"handoff"}, {"switch", "b", "--handoff-reviewed"}} {
		if _, err := a.Execute(args); !errors.Is(err, state.ErrBusy) {
			t.Fatalf("%v: expected project busy, got %v", args, err)
		}
	}
}
