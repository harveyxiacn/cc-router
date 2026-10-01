package update

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPreparedTransactionTamperingDoesNotReplaceInstalledFiles(t *testing.T) {
	install, stage := t.TempDir(), t.TempDir()
	fixtureFile(t, install, "cc-router", "old")
	f := fixtureFile(t, stage, "cc-router", "new")
	txn, err := beginTransaction(install, "44444444444444444444444444444444", stage, []File{f})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(install, workDirectory, txn.ID, "new", "cc-router"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = txn.install(); err == nil {
		t.Fatal("accepted changed transaction file")
	}
	b, _ := os.ReadFile(filepath.Join(install, "cc-router"))
	if string(b) != "old" {
		t.Fatal("original changed")
	}
}

func TestHealthProbeProcess(t *testing.T) {
	if os.Getenv("CCR_TEST_HEALTH_CHILD") != "1" {
		return
	}
	if os.Getenv("CCR_TEST_HEALTH_REPORT") == "1" {
		b, _ := json.Marshal(health{Nonce: "nonce", Version: "0.2.0", PID: os.Getpid()})
		if err := os.WriteFile(filepath.Join(os.Getenv("CCR_TEST_HEALTH_DIR"), "healthy.json"), b, 0600); err != nil {
			os.Exit(2)
		}
	}
	time.Sleep(20 * time.Second)
	os.Exit(0)
}
func TestHelperHealthRequiresMatchingLiveChild(t *testing.T) {
	for _, report := range []bool{true, false} {
		t.Run(map[bool]string{true: "ready", false: "timeout"}[report], func(t *testing.T) {
			data := t.TempDir()
			p := &plan{DataRoot: data, ID: "55555555555555555555555555555555", Nonce: "nonce", Candidate: Candidate{Manifest: Manifest{Version: "0.2.0"}}}
			dir := planDirectory(data, p.ID)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestHealthProbeProcess$")
			flag := "0"
			if report {
				flag = "1"
			}
			cmd.Env = append(os.Environ(), "CCR_TEST_HEALTH_CHILD=1", "CCR_TEST_HEALTH_REPORT="+flag, "CCR_TEST_HEALTH_DIR="+dir)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			timeout := 2 * time.Second
			if !report {
				timeout = 200 * time.Millisecond
			}
			err := waitHealthy(p, cmd, timeout)
			if report {
				_ = cmd.Process.Kill()
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil {
					t.Fatal("accepted unconfirmed child")
				}
				if cmd.ProcessState == nil || !cmd.ProcessState.Exited() { // Windows exit state for a killed process is not successful.
					if alive, e := processAlive(cmd.Process.Pid); e == nil && alive {
						t.Fatal("timed out helper child still alive")
					}
				}
			}
		})
	}
}

func TestPlanRejectsArbitraryPathAndNonce(t *testing.T) {
	if _, err := readPlan(t.TempDir(), "../../outside", "bad"); err == nil {
		t.Fatal("accepted arbitrary plan")
	}
}
