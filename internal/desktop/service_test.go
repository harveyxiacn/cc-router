package desktop

import (
	"github.com/harveyxiacn/cc-router/internal/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testService(t *testing.T) *Service {
	t.Helper()
	s, err := state.Open(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	return &Service{Store: s, CLIPath: os.Args[0]}
}

func TestDesktopAccountActionsAndBinding(t *testing.T) {
	s := testService(t)
	project := t.TempDir()
	if err := s.CreateAccount("a", "Personal"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetDefault("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Bind(project, "a"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.GetSnapshot(project)
	if err != nil || len(snapshot.Accounts) != 1 || snapshot.BoundAccount != "a" || !snapshot.Accounts[0].IsDefault || snapshot.Accounts[0].Usage != nil {
		t.Fatalf("%+v %v", snapshot, err)
	}
	unlock, err := s.Store.LockAccount(snapshot.Accounts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ = s.GetSnapshot(project)
	if !snapshot.Accounts[0].Active {
		t.Fatal("missing active state")
	}
	if err := s.RemoveAccount("a"); err == nil {
		t.Fatal("removed active account")
	}
	unlock()
	if err := s.RenameAccount("a", "b", "Work"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveAccount("b"); err != nil {
		t.Fatal(err)
	}
}

func TestDesktopHandoffSaveDetectsChanges(t *testing.T) {
	s := testService(t)
	project := t.TempDir()
	h, err := s.CreateHandoff(project)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveHandoff(project, "# My notes\n", h.Digest)
	if err != nil || saved.Content != "# My notes\n" {
		t.Fatalf("%+v %v", saved, err)
	}
	if _, err := s.SaveHandoff(project, "overwrite", h.Digest); err == nil {
		t.Fatal("stale editor overwrote newer notes")
	}
	content, _ := os.ReadFile(saved.Path)
	if string(content) != "# My notes\n" {
		t.Fatal("lost notes")
	}
}

func TestDesktopUsageSetupPreservesSettingsAndRefusesOverwrite(t *testing.T) {
	s := testService(t)
	if err := s.CreateAccount("a", "A"); err != nil {
		t.Fatal(err)
	}
	r, _ := s.Store.Load()
	account, _ := r.Find("a")
	path := filepath.Join(s.Store.ProfileDir(account), "settings.json")
	if err := os.WriteFile(path, []byte(`{"model":"sonnet"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.InstallUsage("a", 90, 95); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "sonnet") || !strings.Contains(string(content), account.ID) || !strings.Contains(string(content), "--switch-at 95") {
		t.Fatal(string(content))
	}
	if err := s.InstallUsage("a", 90, 95); err == nil {
		t.Fatal("overwrote existing statusline")
	}
}

func TestDesktopUsageSetupRejectsReplacedProfileLink(t *testing.T) {
	s := testService(t)
	if err := s.CreateAccount("a", "A"); err != nil {
		t.Fatal(err)
	}
	r, _ := s.Store.Load()
	a, _ := r.Find("a")
	profile := s.Store.ProfileDir(a)
	outside := t.TempDir()
	settings := filepath.Join(outside, "settings.json")
	if err := os.WriteFile(settings, []byte(`{"model":"preserve"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(profile, profile+"-retained"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, profile); err != nil {
		t.Skipf("symlink permission unavailable: %v", err)
	}
	if err := s.InstallUsage("a", 90, 95); err == nil {
		t.Fatal("updated linked profile settings")
	}
	data, _ := os.ReadFile(settings)
	if string(data) != `{"model":"preserve"}` {
		t.Fatal("modified external settings")
	}
}

func TestDesktopLaunchUsesValidatedNamesAndDirectory(t *testing.T) {
	s := testService(t)
	if err := s.CreateAccount("a", "A"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	called := false
	s.OpenTerminal = func(executable, project string, args []string) error {
		called = true
		r, _ := s.Store.Load()
		account, _ := r.Find("a")
		if executable != s.CLIPath || len(args) != 6 || args[0] != "--data-dir" || args[1] != s.Store.Root || args[2] != "--expect-account-id" || args[3] != account.ID || args[4] != "run" || args[5] != "a" {
			t.Fatalf("unsafe launch: %s %s %v", executable, project, args)
		}
		return nil
	}
	if err := s.Launch("a", dir, "run", false); err != nil || !called {
		t.Fatalf("%v %v", called, err)
	}
	called = false
	if err := s.Launch("a", dir, "switch", false); err == nil || called {
		t.Fatal("switch skipped review")
	}
	if err := s.Launch("a", dir, "shell", true); err == nil {
		t.Fatal("unrestricted mode")
	}
	t.Setenv("ANTHROPIC_API_KEY", "must-not-display")
	if err := s.Launch("a", dir, "run", false); err == nil || called || strings.Contains(err.Error(), "must-not-display") {
		t.Fatalf("conflicting authentication opened a terminal or leaked: %v", err)
	}
}

func TestTerminalCommandQuoting(t *testing.T) {
	if posixQuote("a'b $x") != "'a'\\''b $x'" {
		t.Fatal("unsafe quoting")
	}
	if _, err := statuslineCommand("/path/with\nnewline", strings.Repeat("a", 32), 90, 95); err == nil {
		t.Fatal("unsafe executable path")
	}
}

func TestDesktopMetadataRoundTripOmitsPrivatePaths(t *testing.T) {
	s := testService(t)
	if err := s.CreateAccount("personal", "Private account"); err != nil {
		t.Fatal(err)
	}
	if err := s.Bind(t.TempDir(), "personal"); err != nil {
		t.Fatal(err)
	}
	data, err := s.ExportMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(data, s.Store.Root) || strings.Contains(data, "bindings") || strings.Contains(data, "profiles") {
		t.Fatal("private metadata exported")
	}
	other := testService(t)
	if err := other.ImportMetadata(data); err != nil {
		t.Fatal(err)
	}
	snapshot, err := other.GetSnapshot("")
	if err != nil || len(snapshot.Accounts) != 1 || snapshot.Accounts[0].Name != "personal" {
		t.Fatalf("%+v %v", snapshot, err)
	}
	if err := other.ImportMetadata(`{"access_token":"secret"}`); err == nil {
		t.Fatal("accepted non-metadata")
	}
}
