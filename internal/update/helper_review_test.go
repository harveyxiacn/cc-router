package update

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCorruptRecoveryReferenceBlocksManagedLaunch(t *testing.T) {
	install, data := t.TempDir(), t.TempDir()
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
	work, err := workRoot(install)
	if err != nil {
		t.Fatal(err)
	}
	if err = jsonWrite(work, "active.json", map[string]any{"id": "corrupt", "dataRoot": data}); err != nil {
		t.Fatal(err)
	}
	work.Close()
	m := NewManager(data, filepath.Join(install, filepath.FromSlash(gui)), filepath.Join(install, filepath.FromSlash(cli)), "0.1.0", nil)
	defer m.Close()
	if err = m.Start(context.Background()); err == nil {
		t.Fatal("accepted invalid recovery reference")
	}
	if err = m.CanLaunch(); err == nil {
		t.Fatal("allowed a managed launch from an installation whose recovery journal could not be verified")
	}
	// The installed files remain available for repair; the updater must not invent a recovery result.
	if _, err = os.Stat(filepath.Join(install, filepath.FromSlash(gui))); err != nil {
		t.Fatal(err)
	}
}
