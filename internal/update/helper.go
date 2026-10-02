package update

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/harveyxiacn/cc-router/internal/localdata"
	"github.com/harveyxiacn/cc-router/internal/state"
)

// Plans and journals contain paths, hashes and signed release metadata only.
type plan struct {
	Schema     int       `json:"schema"`
	ID         string    `json:"id"`
	Nonce      string    `json:"nonce"`
	DataRoot   string    `json:"dataRoot"`
	Layout     Layout    `json:"layout"`
	OldVersion string    `json:"oldVersion"`
	Candidate  Candidate `json:"candidate"`
	OldGUI     File      `json:"oldGUI"`
	OldCLI     File      `json:"oldCLI"`
}
type reference struct {
	ID       string `json:"id"`
	DataRoot string `json:"dataRoot"`
}
type outcome struct {
	Phase   string `json:"phase"`
	Error   string `json:"error"`
	Version string `json:"version"`
}
type health struct {
	Nonce   string `json:"nonce"`
	Version string `json:"version"`
	PID     int    `json:"pid"`
}
type helperReady struct {
	Nonce  string `json:"nonce"`
	PID    int    `json:"pid"`
	Parent int    `json:"parent"`
	Mode   string `json:"mode"`
}

func randomID() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func cacheDirectory(dataRoot string) string { return filepath.Join(dataRoot, "updates") }
func planDirectory(dataRoot, id string) string {
	return filepath.Join(cacheDirectory(dataRoot), "transactions", id)
}
func jsonWrite(root *os.Root, name string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return localdata.Write(root, name, b)
}
func jsonRead(root *os.Root, name string, v any) error {
	b, err := localdata.Read(root, name, 2<<20)
	if err != nil {
		return err
	}
	if err = rejectDuplicateKeys(b); err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
func readPlan(dataRoot, id, nonce string) (*plan, error) {
	if !filepath.IsAbs(dataRoot) || !transactionID.MatchString(id) || !transactionID.MatchString(nonce) {
		return nil, errors.New("invalid update plan location or nonce")
	}
	root, err := os.OpenRoot(cacheDirectory(dataRoot))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	name := "transactions/" + id + "/plan.json"
	if err = plainParents(root, name, false); err != nil {
		return nil, err
	}
	var p plan
	if err = jsonRead(root, name, &p); err != nil {
		return nil, err
	}
	if p.Schema != 1 || p.ID != id || p.Nonce != nonce || filepath.Clean(p.DataRoot) != filepath.Clean(dataRoot) {
		return nil, errors.New("update plan identity mismatch")
	}
	layout, err := Installation(filepath.Join(p.Layout.Root, filepath.FromSlash(p.Layout.GUI)), filepath.Join(p.Layout.Root, filepath.FromSlash(p.Layout.CLI)))
	if err != nil || layout != p.Layout {
		return nil, errors.New("invalid installation layout in plan")
	}
	return &p, nil
}
func (p *plan) verify(expiry bool) (Manifest, error) {
	key, err := TrustedPublicKey()
	if err != nil {
		return Manifest{}, err
	}
	now := time.Now()
	if !expiry {
		now = p.Candidate.Manifest.PublishedAt.Add(time.Second)
	}
	m, err := VerifyManifest(p.Candidate.ManifestBytes, p.Candidate.Signature, key, now)
	if err != nil {
		return m, err
	}
	a := p.Candidate.Asset
	found := false
	for _, v := range m.Assets {
		if v == a {
			found = true
		}
	}
	if !found || a.OS != runtime.GOOS || a.Arch != runtime.GOARCH || a.GUI != p.Layout.GUI || a.CLI != p.Layout.CLI || m.Version != p.Candidate.Manifest.Version {
		return m, errors.New("signed update does not match this installation")
	}
	newer, err := IsNewer(m.Version, p.OldVersion)
	if err != nil || !newer {
		return m, errors.New("update must advance the installed version")
	}
	return m, nil
}
func (p *plan) result(v outcome) error {
	root, err := os.OpenRoot(planDirectory(p.DataRoot, p.ID))
	if err != nil {
		return err
	}
	defer root.Close()
	return jsonWrite(root, "result.json", v)
}
func (p *plan) matchesOld() error {
	root, err := os.OpenRoot(p.Layout.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, f := range []File{p.OldGUI, p.OldCLI} {
		a, err := measure(root, f.Path)
		if err != nil || !sameFileData(a, f) {
			return errors.New("installed programs changed after update preparation")
		}
	}
	return nil
}
func readReference(layout Layout) (reference, error) {
	root, err := os.OpenRoot(filepath.Join(layout.Root, workDirectory))
	if err != nil {
		return reference{}, err
	}
	defer root.Close()
	var ref reference
	err = jsonRead(root, "active.json", &ref)
	if err == nil && (!transactionID.MatchString(ref.ID) || !filepath.IsAbs(ref.DataRoot)) {
		err = errors.New("invalid update recovery reference")
	}
	return ref, err
}

var errRecoveryUnsafe = errors.New("incomplete or invalid update recovery data")

func loadReferencedPlan(dataRoot string, layout Layout) (p *plan, t *transaction, result error) {
	ref, err := readReference(layout)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if result != nil {
			result = errors.Join(errRecoveryUnsafe, result)
		}
	}()
	if filepath.Clean(ref.DataRoot) != filepath.Clean(dataRoot) {
		return nil, nil, errors.New("update belongs to another local data directory")
	}
	root, err := os.OpenRoot(planDirectory(dataRoot, ref.ID))
	if err != nil {
		return nil, nil, err
	}
	var raw plan
	err = jsonRead(root, "plan.json", &raw)
	root.Close()
	if err != nil {
		return nil, nil, err
	}
	p, err = readPlan(dataRoot, ref.ID, raw.Nonce)
	if err != nil {
		return nil, nil, err
	}
	if p.Layout != layout {
		return nil, nil, errors.New("update recovery layout mismatch")
	}
	t, err = openTransaction(layout.Root, p.ID)
	return p, t, err
}
func helperName() string {
	if runtime.GOOS == "windows" {
		return "update-helper.exe"
	}
	return "update-helper"
}
func cleanHealthEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, s := range env {
		key, _, _ := strings.Cut(s, "=")
		if strings.EqualFold(key, "CCR_UPDATE_ID") || strings.EqualFold(key, "CCR_UPDATE_NONCE") || strings.EqualFold(key, "CCR_HOME") {
			continue
		}
		out = append(out, s)
	}
	return out
}
func (p *plan) spawn(mode string) error {
	source, err := os.OpenRoot(p.Layout.Root)
	if err != nil {
		return err
	}
	defer source.Close()
	root, err := os.OpenRoot(planDirectory(p.DataRoot, p.ID))
	if err != nil {
		return err
	}
	defer root.Close()
	exe, err := measure(source, p.Layout.CLI)
	if err != nil {
		return err
	}
	// A previous helper is reusable only if it is exactly the currently trusted CLI.
	if existing, err := measure(root, helperName()); err == nil {
		if !sameFileData(existing, exe) {
			if err = root.Remove(helperName()); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err = root.Stat(helperName()); errors.Is(err, os.ErrNotExist) {
		if err = copyVerified(source, p.Layout.CLI, root, helperName(), exe); err != nil {
			return err
		}
	}
	command := exec.Command(filepath.Join(planDirectory(p.DataRoot, p.ID), helperName()), "internal-update", p.DataRoot, p.ID, p.Nonce, mode, strconv.Itoa(os.Getpid()))
	command.Env = cleanHealthEnv(os.Environ())
	command.Dir = planDirectory(p.DataRoot, p.ID)
	hideHelper(command)
	if err = root.Remove("helper-ready.json"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err = command.Start(); err != nil {
		return err
	}
	// Keep the GUI alive until the helper has authenticated its inputs and can wait for it.
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return errors.New("update helper exited before confirming readiness")
		case <-timer.C:
			_ = command.Process.Kill()
			<-done
			return errors.New("update helper did not become ready; the desktop was kept open")
		case <-ticker.C:
			var ready helperReady
			if jsonRead(root, "helper-ready.json", &ready) == nil && ready.Nonce == p.Nonce && ready.PID == command.Process.Pid && ready.Parent == os.Getpid() && ready.Mode == mode {
				select {
				case <-done:
					return errors.New("update helper exited during readiness confirmation")
				default:
				}
				alive, err := processAlive(command.Process.Pid)
				if err != nil || !alive {
					_ = command.Process.Kill()
					<-done
					return errors.New("update helper is no longer running")
				}
				return nil
			}
		}
	}
}
func (p *plan) startGUI(probe bool) (*exec.Cmd, error) {
	cmd := exec.Command(filepath.Join(p.Layout.Root, filepath.FromSlash(p.Layout.GUI)))
	cmd.Dir = p.Layout.Root
	cmd.Env = append(cleanHealthEnv(os.Environ()), "CCR_HOME="+p.DataRoot)
	if probe {
		cmd.Env = append(cmd.Env, "CCR_UPDATE_ID="+p.ID, "CCR_UPDATE_NONCE="+p.Nonce)
	}
	return cmd, cmd.Start()
}
func waitParent(ctx context.Context, pid int) error {
	if pid <= 0 || pid == os.Getpid() {
		return errors.New("invalid updater parent")
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		alive, err := processAlive(pid)
		if err != nil {
			return err
		}
		if !alive {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("desktop did not exit before update deadline")
		case <-ticker.C:
		}
	}
}
func waitHealthy(p *plan, child *exec.Cmd, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	root, err := os.OpenRoot(planDirectory(p.DataRoot, p.ID))
	if err != nil {
		_ = child.Process.Kill()
		<-done
		return err
	}
	defer root.Close()
	for {
		select {
		case <-done:
			return errors.New("updated desktop exited before startup confirmation")
		case <-timer.C:
			_ = child.Process.Kill()
			<-done
			return errors.New("updated desktop did not confirm a rendered, usable interface in time")
		case <-ticker.C:
			var h health
			if err = jsonRead(root, "healthy.json", &h); err == nil && h.Nonce == p.Nonce && h.Version == p.Candidate.Manifest.Version && h.PID == child.Process.Pid {
				select {
				case <-done:
					return errors.New("updated desktop exited during startup confirmation")
				default:
					return nil
				}
			}
		}
	}
}

// RunHelper is an internal CLI entrypoint. No credentials or account files are read.
func RunHelper(args []string) error {
	if len(args) != 5 {
		return errors.New("invalid internal updater arguments")
	}
	p, err := readPlan(args[0], args[1], args[2])
	if err != nil {
		return err
	}
	mode := args[3]
	if mode != "install" && mode != "rollback" && mode != "recover" {
		return errors.New("invalid updater operation")
	}
	if _, err = p.verify(mode == "install"); err != nil {
		return err
	}
	pid, err := strconv.Atoi(args[4])
	if err != nil {
		return err
	}
	if pid <= 0 || pid == os.Getpid() {
		return errors.New("invalid updater parent")
	}
	planRoot, err := os.OpenRoot(planDirectory(p.DataRoot, p.ID))
	if err != nil {
		return err
	}
	err = jsonWrite(planRoot, "helper-ready.json", helperReady{Nonce: p.Nonce, PID: os.Getpid(), Parent: pid, Mode: mode})
	planRoot.Close()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = waitParent(ctx, pid); err != nil {
		return err
	}
	unlock, err := AcquireLease(p.Layout.Root, true)
	if err != nil {
		_ = p.result(outcome{Phase: "deferred", Error: err.Error()})
		// Avoid a restart loop while another desktop owns a shared installation lease.
		p.suppressAutomaticRetry()
		child, startErr := p.startGUI(false)
		if startErr == nil {
			_ = child.Process.Release()
		}
		return err
	}
	locked := true
	release := func() {
		if locked {
			unlock()
			locked = false
		}
	}
	defer release()
	restart := func() {
		release()
		child, startErr := p.startGUI(false)
		if startErr == nil {
			_ = child.Process.Release()
		}
	}
	if mode != "install" {
		t, err := openTransaction(p.Layout.Root, p.ID)
		if err != nil {
			return err
		}
		if mode == "rollback" {
			if t.Phase != "complete" {
				return errors.New("only a successful update can be manually rolled back")
			}
			if err = t.matchesInstalled(); err != nil {
				return err
			}
		}
		if err = t.rollback(); err != nil {
			_ = p.result(outcome{Phase: "error", Error: err.Error()})
			return err
		}
		_ = p.result(outcome{Phase: "rolled-back", Version: p.Candidate.Manifest.Version})
		restart()
		return nil
	}
	if err = p.matchesOld(); err != nil {
		_ = p.result(outcome{Phase: "error", Error: err.Error()})
		restart()
		return err
	}
	stage := filepath.Join(planDirectory(p.DataRoot, p.ID), "extracted")
	files, err := Extract(filepath.Join(planDirectory(p.DataRoot, p.ID), p.Candidate.Asset.Name), stage, p.Candidate.Asset)
	if err != nil {
		_ = p.result(outcome{Phase: "error", Error: err.Error()})
		restart()
		return err
	}
	metadata, err := state.Open(p.DataRoot)
	if err == nil {
		_, err = metadata.CreateUpdateBackup()
	}
	if err != nil {
		_ = p.result(outcome{Phase: "error", Error: "cannot preserve tool configuration before updating"})
		p.suppressAutomaticRetry()
		restart()
		return errors.New("cannot preserve tool configuration before updating")
	}
	t, err := beginTransaction(p.Layout.Root, p.ID, stage, files)
	if err != nil {
		_ = p.result(outcome{Phase: "error", Error: err.Error()})
		restart()
		return err
	}
	work, err := workRoot(p.Layout.Root)
	if err != nil {
		return err
	}
	previous, _ := readReference(p.Layout)
	err = jsonWrite(work, "active.json", reference{ID: p.ID, DataRoot: p.DataRoot})
	work.Close()
	if err != nil {
		return err
	}
	if err = t.install(); err != nil {
		restoreErr := t.rollback()
		_ = p.result(outcome{Phase: "rolled-back", Error: err.Error(), Version: p.Candidate.Manifest.Version})
		if restoreErr == nil {
			restart()
		}
		return errors.Join(err, restoreErr)
	}
	err = t.finish(func() error {
		child, err := p.startGUI(true)
		if err != nil {
			return err
		}
		return waitHealthy(p, child, 45*time.Second)
	})
	if err != nil {
		_ = p.result(outcome{Phase: "rolled-back", Error: err.Error(), Version: p.Candidate.Manifest.Version})
		if t.Phase == "rolled-back" {
			restart()
		}
		return err
	}
	if err = p.result(outcome{Phase: "complete", Version: p.Candidate.Manifest.Version}); err != nil {
		return err
	}
	p.cleanCompleted(previous)
	return nil
}

func (p *plan) suppressAutomaticRetry() {
	root, err := os.OpenRoot(cacheDirectory(p.DataRoot))
	if err != nil {
		return
	}
	defer root.Close()
	prefs := preferences{AutoUpdate: true}
	err = jsonRead(root, "preferences.json", &prefs)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
	prefs.FailedVersion = p.Candidate.Manifest.Version
	_ = jsonWrite(root, "preferences.json", prefs)
}
func (p *plan) cleanCompleted(previous reference) {
	// Only transaction paths generated by this updater are eligible for cleanup.
	root, err := os.OpenRoot(planDirectory(p.DataRoot, p.ID))
	if err == nil {
		_ = root.RemoveAll("extracted")
		_ = root.Remove(p.Candidate.Asset.Name)
		root.Close()
	}
	if previous.ID == p.ID || !transactionID.MatchString(previous.ID) || previous.DataRoot != p.DataRoot {
		return
	}
	t, err := openTransaction(p.Layout.Root, previous.ID)
	if err != nil || (t.Phase != "complete" && t.Phase != "rolled-back") {
		return
	}
	work, err := workRoot(p.Layout.Root)
	if err == nil {
		_ = work.RemoveAll(previous.ID)
		work.Close()
	}
	cache, err := os.OpenRoot(cacheDirectory(p.DataRoot))
	if err == nil {
		_ = cache.RemoveAll("transactions/" + previous.ID)
		cache.Close()
	}
}
func (t *transaction) matchesInstalled() error {
	root, err := os.OpenRoot(t.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, entry := range t.Entries {
		actual, err := measure(root, entry.New.Path)
		if err != nil || !sameFileData(actual, entry.New) {
			return fmt.Errorf("installed file changed since update: %s", entry.New.Path)
		}
	}
	return nil
}

func newPlan(dataRoot string, layout Layout, version string, candidate *Candidate) (*plan, error) {
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	nonce, err := randomID()
	if err != nil {
		return nil, err
	}
	_, err = state.Open(planDirectory(dataRoot, id))
	if err != nil {
		return nil, err
	}
	p := &plan{Schema: 1, ID: id, Nonce: nonce, DataRoot: dataRoot, Layout: layout, OldVersion: version, Candidate: *candidate}
	if _, err = p.verify(true); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(layout.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if p.OldGUI, err = measure(root, layout.GUI); err != nil {
		return nil, err
	}
	if p.OldCLI, err = measure(root, layout.CLI); err != nil {
		return nil, err
	}
	cache, err := os.OpenRoot(planDirectory(dataRoot, id))
	if err != nil {
		return nil, err
	}
	defer cache.Close()
	if err = jsonWrite(cache, "plan.json", p); err != nil {
		return nil, err
	}
	return p, nil
}
