package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/harveyxiacn/cc-router/internal/state"
)

func TestManagerBlocksInstallWhileUserOrSessionBusy(t *testing.T) {
	m := NewManager(t.TempDir(), "", "", "0.1.0", func() error { return errors.New("managed session active") })
	m.ready = true
	m.unlock = func() {}
	if err := m.Apply(); err == nil {
		t.Fatal("installed with active UI")
	}
	m.SetIdle(true)
	if err := m.Apply(); err == nil || err.Error() != "managed session active" {
		t.Fatalf("session guard not checked: %v", err)
	}
	m.canApply = nil
	if err := m.Apply(); err == nil {
		t.Fatal("installed without verified package")
	}
}
func TestAutomaticUpdatePreferencePersists(t *testing.T) {
	data := t.TempDir()
	if _, err := state.Open(cacheDirectory(data)); err != nil {
		t.Fatal(err)
	}
	m := NewManager(data, "", "", "0.1.0", nil)
	if !m.Status().AutoUpdate {
		t.Fatal("user-requested background updates should default on")
	}
	if err := m.SetAuto(false); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(cacheDirectory(data))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	var p preferences
	if err = jsonRead(root, "preferences.json", &p); err != nil {
		t.Fatal(err)
	}
	if p.AutoUpdate {
		t.Fatal("disable preference not persisted")
	}
}
func TestManagerRejectsArbitraryHealthEnvironment(t *testing.T) {
	install := t.TempDir()
	data := t.TempDir()
	gui, cli := "cc-router-desktop", "cc-router"
	if runtime.GOOS == "windows" {
		gui += ".exe"
		cli += ".exe"
	}
	if runtime.GOOS == "darwin" {
		gui = "CC Router.app/Contents/MacOS/" + gui
		cli = "CC Router.app/Contents/MacOS/" + cli
	}
	fixtureFile(t, install, gui, "gui")
	fixtureFile(t, install, cli, "cli")
	t.Setenv("CCR_UPDATE_ID", "bad")
	t.Setenv("CCR_UPDATE_NONCE", "bad")
	m := NewManager(data, filepath.Join(install, filepath.FromSlash(gui)), filepath.Join(install, filepath.FromSlash(cli)), "0.1.0", nil)
	defer m.Close()
	if err := m.Start(context.Background()); err == nil {
		t.Fatal("accepted forged health probe")
	}
	if m.ready {
		t.Fatal("forged probe permitted application lease bypass")
	}
}
