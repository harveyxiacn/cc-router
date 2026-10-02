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
	"errors"
	"fmt"
	"io"
	"net"
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
			if err := runTestOTAHelper(os.Args[2:]); err != nil {
				os.Exit(2)
			}
			os.Exit(0)
		}
		data := os.Getenv("CCR_HOME")
		if os.Getenv("CCR_UPDATE_ID") == "" {
			if err := os.WriteFile(filepath.Join(data, "restarted.txt"), []byte("old desktop restarted"), 0600); err != nil {
				os.Exit(11)
			}
			if address := os.Getenv("CCR_TEST_OTA_RESTART_NOTIFY_ADDR"); address != "" {
				connection, err := net.DialTimeout("tcp", address, 5*time.Second)
				if err != nil {
					os.Exit(8)
				}
				_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
				if err := json.NewEncoder(connection).Encode(os.Getpid()); err != nil {
					connection.Close()
					os.Exit(9)
				}
				// Keep this process alive until its direct parent has obtained a
				// process handle, including on Windows, before allowing exit.
				if _, err := io.Copy(io.Discard, connection); err != nil {
					connection.Close()
					os.Exit(10)
				}
				connection.Close()
			}
			// A test-owned exit barrier exposes helpers that return while their
			// restarted GUI can still be running during TempDir cleanup.
			if address := os.Getenv("CCR_TEST_OTA_RESTART_EXIT_ADDR"); address != "" {
				connection, err := net.DialTimeout("tcp", address, 5*time.Second)
				if err != nil {
					os.Exit(5)
				}
				_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
				if err := json.NewEncoder(connection).Encode(os.Getpid()); err != nil {
					connection.Close()
					os.Exit(6)
				}
				if _, err := io.Copy(io.Discard, connection); err != nil {
					connection.Close()
					os.Exit(7)
				}
				connection.Close()
			}
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
			if broken {
				cmd.Env = append(cmd.Env, "CCR_TEST_OTA_AWAIT_RESTART=1")
			}
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
				if restarted, err := os.ReadFile(filepath.Join(data, "restarted.txt")); err != nil || string(restarted) != "old desktop restarted" {
					t.Fatalf("old GUI did not finish restarting: %q, %v", restarted, err)
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
				// Hold the restarted GUI at an explicit exit barrier. The helper
				// must retain ownership and wait for this child before returning.
				restartExit, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer restartExit.Close()
				_ = restartExit.(*net.TCPListener).SetDeadline(time.Now().Add(10 * time.Second))
				rollback.Env = append(rollback.Env, "CCR_TEST_OTA_AWAIT_RESTART=1", "CCR_TEST_OTA_RESTART_EXIT_ADDR="+restartExit.Addr().String())
				if err := rollback.Start(); err != nil {
					t.Fatal(err)
				}
				rollbackDone := make(chan error, 1)
				go func() { rollbackDone <- rollback.Wait() }()
				connection, err := restartExit.Accept()
				if err != nil {
					t.Fatal("restarted GUI did not enter exit barrier:", err)
				}
				defer connection.Close()
				_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
				var restartedPID int
				if err := json.NewDecoder(connection).Decode(&restartedPID); err != nil {
					t.Fatal(err)
				}
				select {
				case err := <-rollbackDone:
					t.Fatalf("rollback helper returned before restarted GUI %d exited: %v", restartedPID, err)
				case <-time.After(500 * time.Millisecond):
				}
				if err := connection.Close(); err != nil {
					t.Fatal(err)
				}
				select {
				case err = <-rollbackDone:
				case <-time.After(10 * time.Second):
					t.Fatal("rollback helper did not wait for restarted GUI exit")
				}
				if err != nil {
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
				if restarted, err := os.ReadFile(filepath.Join(data, "restarted.txt")); err != nil || string(restarted) != "old desktop restarted" {
					t.Fatalf("rollback GUI did not finish restarting: %q, %v", restarted, err)
				}
			}
		})
	}
}

// The production helper intentionally restarts the desktop asynchronously. The
// test helper remains the restarted fixture's direct parent and reaps it before
// exiting, so the enclosing test cannot clean TempDir while that child runs.
func runTestOTAHelper(args []string) error {
	if os.Getenv("CCR_TEST_OTA_AWAIT_RESTART") != "1" {
		return RunHelper(args)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Setenv("CCR_TEST_OTA_RESTART_NOTIFY_ADDR", listener.Addr().String()); err != nil {
		return err
	}
	result := RunHelper(args)
	_ = listener.(*net.TCPListener).SetDeadline(time.Now().Add(10 * time.Second))
	connection, err := listener.Accept()
	if err != nil {
		return errors.Join(result, fmt.Errorf("restarted GUI did not report its PID: %w", err))
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
	var pid int
	if err := json.NewDecoder(connection).Decode(&pid); err != nil {
		return errors.Join(result, err)
	}
	if pid <= 0 || pid == os.Getpid() {
		return errors.Join(result, errors.New("invalid restarted fixture PID"))
	}
	child, err := os.FindProcess(pid)
	if err != nil {
		return errors.Join(result, err)
	}
	if err := connection.Close(); err != nil {
		child.Release()
		return errors.Join(result, err)
	}
	status, err := child.Wait()
	if err != nil {
		return errors.Join(result, fmt.Errorf("cannot reap restarted GUI: %w", err))
	}
	if !status.Success() {
		return errors.Join(result, errors.New("restarted GUI fixture failed"))
	}
	return result
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
