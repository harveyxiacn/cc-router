package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/harveyxiacn/cc-router/internal/state"
)

// Only the Go test executable accepts fixture keys. Release binaries never read these variables.
func TestMain(m *testing.M) {
	if os.Getenv("CCR_TEST_OTA_PARENT") == "1" {
		os.Exit(0)
	}
	if os.Getenv("CCR_TEST_OTA_PROCESS") == "1" {
		releasePublicKey = os.Getenv("CCR_TEST_OTA_PUBLIC")
		if len(os.Args) > 1 && os.Args[1] == "internal-update" {
			if err := RunHelper(os.Args[2:]); err != nil {
				os.Exit(2)
			}
			os.Exit(0)
		}
		data := os.Getenv("CCR_HOME")
		if os.Getenv("CCR_UPDATE_ID") == "" {
			_ = os.WriteFile(filepath.Join(data, "restarted.txt"), []byte("old desktop restarted"), 0600)
			os.Exit(0)
		}
		exe, _ := os.Executable()
		cli := filepath.Join(filepath.Dir(exe), "cc-router")
		if runtime.GOOS == "windows" {
			cli += ".exe"
		}
		manager := NewManager(data, exe, cli, "0.2.0", nil)
		if err := manager.Start(context.Background()); err != nil {
			os.Exit(3)
		}
		if err := manager.ReportReady(); err != nil {
			os.Exit(4)
		}
		time.Sleep(30 * time.Second)
		manager.Close()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestHelperLifecycleInstallsAndRestoresAfterFailedStartup(t *testing.T) {
	for _, broken := range []bool{false, true} {
		t.Run(map[bool]string{false: "healthy", true: "failed-startup"}[broken], func(t *testing.T) {
			install, data := t.TempDir(), t.TempDir()
			gui, cli := "cc-router-desktop", "cc-router"
			doc := "README.md"
			if runtime.GOOS == "windows" {
				gui += ".exe"
				cli += ".exe"
			}
			if runtime.GOOS == "darwin" {
				gui = "CC Router.app/Contents/MacOS/" + gui
				cli = "CC Router.app/Contents/MacOS/" + cli
				doc = "CC Router.app/Contents/Resources/Documentation/README.md"
			}
			binary, err := os.ReadFile(os.Args[0])
			if err != nil {
				t.Fatal(err)
			}
			fixtureFile(t, install, gui, string(binary))
			fixtureFile(t, install, cli, string(binary))
			fixtureFile(t, install, doc, "old documentation")
			var archive bytes.Buffer
			zw := zip.NewWriter(&archive)
			for name, contents := range map[string][]byte{gui: binary, cli: binary, doc: []byte("updated documentation")} {
				if broken && name == gui {
					contents = []byte("invalid executable")
				}
				h := &zip.FileHeader{Name: name, Method: zip.Deflate}
				h.SetMode(0755)
				w, err := zw.CreateHeader(h)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = w.Write(contents); err != nil {
					t.Fatal(err)
				}
			}
			if err = zw.Close(); err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(archive.Bytes())
			a := Asset{OS: runtime.GOOS, Arch: runtime.GOARCH, Name: "fixture.zip", Size: int64(archive.Len()), SHA256: hex.EncodeToString(sum[:]), GUI: gui, CLI: cli}
			manifest := Manifest{Schema: 1, Version: "0.2.0", PublishedAt: time.Now().UTC().Add(-time.Minute), ExpiresAt: time.Now().UTC().Add(time.Hour), Assets: []Asset{a}}
			raw, _ := json.Marshal(manifest)
			public, private, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			oldKey := releasePublicKey
			releasePublicKey = base64.StdEncoding.EncodeToString(public)
			defer func() { releasePublicKey = oldKey }()
			candidate := &Candidate{Manifest: manifest, Asset: a, ManifestBytes: raw, Signature: ed25519.Sign(private, raw)}
			layout := Layout{Root: install, GUI: gui, CLI: cli}
			p, err := newPlan(data, layout, "0.1.0", candidate)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(planDirectory(data, p.ID), a.Name), archive.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = state.Open(cacheDirectory(data)); err != nil {
				t.Fatal(err)
			}
			cache, err := os.OpenRoot(cacheDirectory(data))
			if err != nil {
				t.Fatal(err)
			}
			err = jsonWrite(cache, "preferences.json", preferences{AutoUpdate: false})
			cache.Close()
			if err != nil {
				t.Fatal(err)
			}
			parent := exec.Command(os.Args[0])
			parent.Env = append(os.Environ(), "CCR_TEST_OTA_PARENT=1")
			if err = parent.Run(); err != nil {
				t.Fatal(err)
			}
			// The helper runs from a copy outside the installation it replaces.
			helper := filepath.Join(planDirectory(data, p.ID), helperName())
			if err = os.WriteFile(helper, binary, 0755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(helper, "internal-update", data, p.ID, p.Nonce, "install", itoaPID(parent.Process.Pid))
			cmd.Env = append(os.Environ(), "CCR_TEST_OTA_PROCESS=1", "CCR_TEST_OTA_PUBLIC="+releasePublicKey)
			err = cmd.Run()
			if !broken && err != nil {
				t.Fatalf("helper failed: %v", err)
			}
			if broken && err == nil {
				t.Fatal("invalid updated GUI accepted")
			}
			store, err := state.Open(data)
			if err != nil {
				t.Fatal(err)
			}
			backups, err := store.ListBackups()
			if err != nil || len(backups) != 1 || backups[0].Reason != "pre-update" || !backups[0].Restorable {
				t.Fatalf("metadata was not backed up before installation: %+v %v", backups, err)
			}
			txn, err := openTransaction(install, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if broken {
				if txn.Phase != "rolled-back" {
					t.Fatalf("phase=%s", txn.Phase)
				}
			} else {
				if txn.Phase != "complete" {
					t.Fatalf("phase=%s", txn.Phase)
				}
			}
			want := "updated documentation"
			if broken {
				want = "old documentation"
			}
			b, _ := os.ReadFile(filepath.Join(install, filepath.FromSlash(doc)))
			if string(b) != want {
				t.Fatalf("documentation=%q", b)
			}
			if broken {
				deadline := time.Now().Add(3 * time.Second)
				for {
					if _, err = os.Stat(filepath.Join(data, "restarted.txt")); err == nil {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("old GUI was not restarted")
					}
					time.Sleep(30 * time.Millisecond)
				}
			} else {
				var h health
				root, err := os.OpenRoot(planDirectory(data, p.ID))
				if err != nil {
					t.Fatal(err)
				}
				err = jsonRead(root, "healthy.json", &h)
				root.Close()
				if err != nil {
					t.Fatal(err)
				}
				child, err := os.FindProcess(h.PID)
				if err != nil {
					t.Fatal(err)
				}
				_ = child.Kill()
				_ = child.Release()
				deadline := time.Now().Add(3 * time.Second)
				for {
					alive, e := processAlive(h.PID)
					if e != nil || !alive {
						break
					}
					if time.Now().After(deadline) {
						break
					}
					time.Sleep(30 * time.Millisecond)
				}
				rollback := exec.Command(helper, "internal-update", data, p.ID, p.Nonce, "rollback", itoaPID(parent.Process.Pid))
				rollback.Env = cmd.Env
				if err = rollback.Run(); err != nil {
					t.Fatalf("manual rollback failed: %v", err)
				}
				restored, err := openTransaction(install, p.ID)
				if err != nil || restored.Phase != "rolled-back" {
					t.Fatalf("manual rollback state: %v %v", restored, err)
				}
				b, _ = os.ReadFile(filepath.Join(install, filepath.FromSlash(doc)))
				if string(b) != "old documentation" {
					t.Fatal("manual rollback did not restore previous files")
				}
			}
		})
	}
}

func itoaPID(n int) string { return strconv.Itoa(n) }

func TestHelperReadinessMustPrecedeDesktopExit(t *testing.T) {
	manifest, raw, sig, public := signedFixture(t, []byte("fixture archive"))
	oldKey := releasePublicKey
	releasePublicKey = base64.StdEncoding.EncodeToString(public)
	defer func() { releasePublicKey = oldKey }()
	t.Setenv("CCR_TEST_OTA_PROCESS", "1")
	t.Setenv("CCR_TEST_OTA_PUBLIC", releasePublicKey)
	install, data := t.TempDir(), t.TempDir()
	a := manifest.Assets[0]
	binary, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, install, a.GUI, string(binary))
	fixtureFile(t, install, a.CLI, string(binary))
	p, err := newPlan(data, Layout{Root: install, GUI: a.GUI, CLI: a.CLI}, "1.1.0", &Candidate{Manifest: manifest, ManifestBytes: raw, Signature: sig, Asset: a})
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(planDirectory(data, p.ID))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	p.Candidate.Signature = append([]byte(nil), sig...)
	p.Candidate.Signature[0] ^= 1
	if err = jsonWrite(root, "plan.json", p); err != nil {
		t.Fatal(err)
	}
	if err = p.spawn("install"); err == nil {
		t.Fatal("desktop would exit for a helper that rejected its inputs")
	}
	p.Candidate.Signature = sig
	if err = jsonWrite(root, "plan.json", p); err != nil {
		t.Fatal(err)
	}
	if err = p.spawn("install"); err != nil {
		t.Fatal(err)
	}
	var ready helperReady
	if err = jsonRead(root, "helper-ready.json", &ready); err != nil {
		t.Fatal(err)
	}
	child, err := os.FindProcess(ready.PID)
	if err != nil {
		t.Fatal(err)
	}
	_ = child.Kill()
	_ = child.Release()
	deadline := time.Now().Add(3 * time.Second)
	for {
		alive, e := processAlive(ready.PID)
		if e != nil || !alive {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if _, err = readReference(p.Layout); !os.IsNotExist(err) {
		t.Fatalf("helper modified the installation before parent exit: %v", err)
	}
}
