package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateMethodsRequireAvailableBackend(t *testing.T) {
	a := &App{initializationError: "startup failed"}
	if _, err := a.GetUpdateStatus(); err == nil {
		t.Fatal("missing update initialization error")
	}
	if _, err := a.CheckForUpdates(); err == nil {
		t.Fatal("missing check initialization error")
	}
	for _, method := range []func() error{func() error { return a.SetAutoUpdate(false) }, func() error { return a.SetUpdateIdle(true) }, a.PrepareUpdate, a.ApplyUpdate, a.RollbackUpdate, a.ReportReady} {
		if err := method(); err == nil {
			t.Fatal("update operation did not report unavailable backend")
		}
	}
}
func TestReleasePageOnlyOpensFixedTrustedRepository(t *testing.T) {
	a := &App{}
	if err := a.OpenReleasePage(); err == nil {
		t.Fatal("missing native context error")
	}
	var opened string
	a.ctx = context.Background()
	a.openBrowser = func(_ context.Context, url string) { opened = url }
	if err := a.OpenReleasePage(); err != nil {
		t.Fatal(err)
	}
	if opened != "https://github.com/harveyxiacn/cc-router/releases" {
		t.Fatalf("unexpected release URL %q", opened)
	}
}

func TestCLIOverrideMustBeAbsolute(t *testing.T) {
	t.Setenv("CC_ROUTER_CLI", "relative.exe")
	if _, err := resolveCLIPath(); err == nil {
		t.Fatal("accepted relative override")
	}
	path := filepath.Join(t.TempDir(), "cc-router.exe")
	t.Setenv("CC_ROUTER_CLI", path)
	got, err := resolveCLIPath()
	if err != nil || got != path {
		t.Fatalf("override=%q %v", got, err)
	}
}
func TestUnavailableBackendReportsInitializationError(t *testing.T) {
	a := &App{initializationError: "测试初始化错误"}
	if _, err := a.GetSnapshot(""); err == nil || !strings.Contains(err.Error(), "测试初始化错误") {
		t.Fatal("missing actionable startup error")
	}
}
func TestMetadataExportIsAtomicAndRejectsLinkedTargets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")
	data := `{"schemaVersion":1,"accounts":[]}`
	if err := writeMetadata(path, data); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != data {
		t.Fatal("wrong export bytes")
	}
	if err := writeMetadata(path, data+"\n"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "preserve.json")
	if err := os.WriteFile(outside, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked.json")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink permission unavailable: %v", err)
	}
	if err := writeMetadata(link, data); err == nil {
		t.Fatal("followed linked export target")
	}
	got, err = os.ReadFile(outside)
	if err != nil || string(got) != "preserve" {
		t.Fatal("external target modified")
	}
}

func TestMetadataExportCannotReplaceRegistryOrOfficialProfile(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "profiles", "abc")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "accounts.json"), filepath.Join(profile, "settings.json"), filepath.Join(t.TempDir(), ".credentials.json")} {
		if err := validateMetadataDestination(path, root); err == nil {
			t.Fatalf("accepted protected export target %s", path)
		}
	}
	if err := validateMetadataDestination(filepath.Join(t.TempDir(), "export.json"), root); err != nil {
		t.Fatal(err)
	}
}
