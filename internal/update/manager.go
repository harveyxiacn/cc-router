package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/harveyxiacn/cc-router/internal/state"
)

var ErrRecoveryStarted = errors.New("recovering an interrupted update; the desktop will restart")

type preferences struct {
	AutoUpdate    bool   `json:"autoUpdate"`
	FailedVersion string `json:"failedVersion"`
}

type Manager struct {
	mu                                  sync.Mutex
	dataRoot, gui, cli, version         string
	layout                              Layout
	status                              Status
	prefs                               preferences
	source                              *Source
	ctx                                 context.Context
	cancel                              context.CancelFunc
	unlock                              func()
	ready, idle, busy, closed, reported bool
	recoveryBlocked                     bool
	candidate                           *Candidate
	prepared, healthPlan                *plan
	canApply                            func() error
}

func NewManager(dataRoot, gui, cli, version string, canApply func() error) *Manager {
	return &Manager{dataRoot: dataRoot, gui: gui, cli: cli, version: version, canApply: canApply, source: NewSource(), prefs: preferences{AutoUpdate: true}, status: Status{CurrentVersion: version, ReleaseURL: ReleasePage, AutoUpdate: true, Phase: "idle"}}
}
func (m *Manager) Status() Status { m.mu.Lock(); defer m.mu.Unlock(); return m.status }
func (m *Manager) fail(err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.busy = false
	m.status.Phase = "error"
	m.status.Error = err.Error()
	return err
}
func (m *Manager) savePreferencesLocked() error {
	root, err := os.OpenRoot(cacheDirectory(m.dataRoot))
	if err != nil {
		return err
	}
	defer root.Close()
	return jsonWrite(root, "preferences.json", m.prefs)
}
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.startLocked(ctx)
	if err != nil && !errors.Is(err, ErrRecoveryStarted) {
		m.busy = false
		m.status.Phase = "error"
		m.status.Error = err.Error()
	}
	return err
}
func (m *Manager) startLocked(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)
	layout, err := Installation(m.gui, m.cli)
	if err != nil {
		return err
	}
	m.layout = layout
	if _, err = state.Open(cacheDirectory(m.dataRoot)); err != nil {
		return err
	}
	root, err := os.OpenRoot(cacheDirectory(m.dataRoot))
	if err != nil {
		return err
	}
	err = jsonRead(root, "preferences.json", &m.prefs)
	root.Close()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	m.status.AutoUpdate = m.prefs.AutoUpdate
	// A health probe is accepted only for the signed transaction that installed this executable.
	id, nonce := os.Getenv("CCR_UPDATE_ID"), os.Getenv("CCR_UPDATE_NONCE")
	if id != "" || nonce != "" {
		p, err := readPlan(m.dataRoot, id, nonce)
		if err != nil {
			return err
		}
		if _, err = p.verify(false); err != nil {
			return err
		}
		t, err := openTransaction(layout.Root, id)
		if err != nil {
			return err
		}
		if p.Layout != layout || p.Candidate.Manifest.Version != m.version || t.Phase != "probation" {
			return errors.New("startup probe does not match the installed update")
		}
		m.healthPlan = p
		m.status.Phase = "probation"
		m.status.Message = "Confirming updated desktop startup"
		return nil
	}
	unlock, err := AcquireLease(layout.Root, false)
	if err != nil {
		return err
	}
	m.unlock = unlock
	m.ready = true
	p, t, err := loadReferencedPlan(m.dataRoot, layout)
	if err == nil {
		switch t.Phase {
		case "complete":
			m.status.CanRollback = true
			m.status.RollbackVersion = p.OldVersion
		case "rolled-back":
			m.prefs.FailedVersion = p.Candidate.Manifest.Version
			m.status.Phase = "rolled-back"
			m.status.Message = "The previous version was restored"
			_ = m.savePreferencesLocked()
		default:
			m.recoveryBlocked = true
			m.ready = false
			if _, err = p.verify(false); err != nil {
				return err
			}
			if err = p.spawn("recover"); err != nil {
				return err
			}
			m.status.Phase = "recovering"
			m.status.Message = ErrRecoveryStarted.Error()
			return ErrRecoveryStarted
		}
	} else if errors.Is(err, errRecoveryUnsafe) || !errors.Is(err, os.ErrNotExist) {
		m.recoveryBlocked = true
		m.ready = false
		return err
	}
	go m.worker()
	return nil
}
func (m *Manager) worker() {
	if m.Status().AutoUpdate {
		_, _ = m.Check()
	}
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if m.Status().AutoUpdate {
				_, _ = m.Check()
			}
		}
	}
}
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	if m.cancel != nil {
		m.cancel()
	}
	if m.unlock != nil {
		m.unlock()
		m.unlock = nil
	}
}
func (m *Manager) SetAuto(enabled bool) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("updater is closed")
	}
	previous := m.prefs.AutoUpdate
	m.prefs.AutoUpdate = enabled
	if err := m.savePreferencesLocked(); err != nil {
		m.prefs.AutoUpdate = previous
		m.mu.Unlock()
		return err
	}
	m.status.AutoUpdate = enabled
	m.mu.Unlock()
	if enabled {
		_, _ = m.Check()
	}
	return nil
}
func (m *Manager) SetIdle(idle bool) { m.mu.Lock(); m.idle = idle; m.mu.Unlock() }
func (m *Manager) Check() (Status, error) {
	m.mu.Lock()
	if m.closed || !m.ready {
		status := m.status
		m.mu.Unlock()
		return status, errors.New("updater is not ready")
	}
	if m.busy || m.status.Phase == "installing" || m.status.Phase == "recovering" {
		status := m.status
		m.mu.Unlock()
		return status, nil
	}
	if m.prepared != nil {
		status := m.status
		m.mu.Unlock()
		return status, nil
	}
	m.busy = true
	m.status.Phase = "checking"
	m.status.Error = ""
	status := m.status
	m.mu.Unlock()
	go m.check()
	return status, nil
}
func (m *Manager) check() {
	candidate, err := m.source.Fetch(m.ctx, m.version)
	if err != nil {
		_ = m.fail(err)
		return
	}
	m.mu.Lock()
	m.status.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	m.candidate = candidate
	if candidate == nil {
		m.busy = false
		m.status.Phase = "upToDate"
		m.status.UpdateAvailable = false
		m.status.Message = "No newer signed release is available"
		m.mu.Unlock()
		return
	}
	m.status.LatestVersion = candidate.Manifest.Version
	m.status.Notes = candidate.Manifest.Notes
	m.status.Prerelease = strings.Contains(candidate.Manifest.Version, "-")
	m.status.UpdateAvailable = true
	m.status.Phase = "available"
	auto := m.prefs.AutoUpdate && m.prefs.FailedVersion != candidate.Manifest.Version
	if m.prefs.FailedVersion == candidate.Manifest.Version {
		m.status.Message = "A previous automatic attempt did not finish; use the manual download button to retry this version"
	}
	if !auto {
		m.busy = false
	}
	m.mu.Unlock()
	if auto {
		m.prepare(candidate)
	}
}
func (m *Manager) Prepare() error {
	m.mu.Lock()
	if m.closed || !m.ready || m.busy || m.status.Phase == "installing" {
		m.mu.Unlock()
		return errors.New("updater is busy or unavailable")
	}
	if m.prepared != nil {
		m.mu.Unlock()
		return nil
	}
	candidate := m.candidate
	if candidate == nil {
		m.mu.Unlock()
		return errors.New("check for an update first")
	}
	m.busy = true
	m.mu.Unlock()
	go m.prepare(candidate)
	return nil
}
func (m *Manager) prepare(candidate *Candidate) {
	m.mu.Lock()
	m.status.Phase = "downloading"
	m.status.Progress = 0
	m.status.Error = ""
	m.mu.Unlock()
	p, err := newPlan(m.dataRoot, m.layout, m.version, candidate)
	if err != nil {
		_ = m.fail(err)
		return
	}
	err = m.source.Download(m.ctx, candidate, filepath.Join(planDirectory(m.dataRoot, p.ID), candidate.Asset.Name), func(got, total int64) { m.mu.Lock(); m.status.Progress = float64(got) / float64(total); m.mu.Unlock() })
	if err != nil {
		if cache, openErr := os.OpenRoot(cacheDirectory(m.dataRoot)); openErr == nil {
			_ = cache.RemoveAll("transactions/" + p.ID)
			cache.Close()
		}
		_ = m.fail(err)
		return
	}
	m.mu.Lock()
	m.prepared = p
	m.busy = false
	m.status.Phase = "ready"
	m.status.Progress = 1
	m.status.Message = "Verified update ready; waiting for an idle desktop and closed managed sessions"
	m.mu.Unlock()
}
func (m *Manager) applyGuardLocked() error {
	if m.closed || !m.ready || m.busy || !m.idle || m.unlock == nil {
		return errors.New("save drafts and wait for the desktop to become idle before restarting")
	}
	if m.canApply != nil {
		if err := m.canApply(); err != nil {
			return err
		}
	}
	return nil
}
func (m *Manager) Apply() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.applyGuardLocked(); err != nil {
		return err
	}
	if m.prepared == nil {
		return errors.New("no verified update has been downloaded")
	}
	if err := m.prepared.spawn("install"); err != nil {
		return err
	}
	m.status.Phase = "installing"
	m.status.Message = "Restarting to install the verified update"
	m.busy = true
	return nil
}
func (m *Manager) Rollback() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.applyGuardLocked(); err != nil {
		return err
	}
	p, t, err := loadReferencedPlan(m.dataRoot, m.layout)
	if err != nil {
		return err
	}
	if t.Phase != "complete" {
		return errors.New("no completed update is available to roll back")
	}
	if _, err = p.verify(false); err != nil {
		return err
	}
	if err = t.matchesInstalled(); err != nil {
		return err
	}
	if err = p.spawn("rollback"); err != nil {
		return err
	}
	m.status.Phase = "installing"
	m.status.Message = "Restarting to restore the previous version"
	m.busy = true
	return nil
}

// CanLaunch keeps official CLI launches out of the helper's startup probation.
func (m *Manager) CanLaunch() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recoveryBlocked || m.healthPlan != nil && !m.ready || m.status.Phase == "installing" || m.status.Phase == "recovering" {
		return errors.New("wait for the application update to finish")
	}
	return nil
}
func (m *Manager) ReportReady() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.healthPlan == nil || m.reported {
		return nil
	}
	p := m.healthPlan
	root, err := os.OpenRoot(planDirectory(m.dataRoot, p.ID))
	if err != nil {
		return err
	}
	defer root.Close()
	if err = jsonWrite(root, "healthy.json", health{Nonce: p.Nonce, Version: m.version, PID: os.Getpid()}); err != nil {
		return err
	}
	m.reported = true
	go func() {
		deadline := time.NewTimer(15 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-deadline.C:
				_ = m.fail(errors.New("update helper did not release the installation"))
				return
			case <-ticker.C:
				unlock, err := AcquireLease(m.layout.Root, false)
				if errors.Is(err, ErrBusy) {
					continue
				}
				if err != nil {
					_ = m.fail(err)
					return
				}
				m.mu.Lock()
				if m.closed {
					unlock()
					m.mu.Unlock()
					return
				}
				m.unlock = unlock
				t, inspectErr := openTransaction(m.layout.Root, p.ID)
				if inspectErr != nil || t.Phase != "complete" {
					m.recoveryBlocked = true
					if inspectErr == nil {
						inspectErr = p.spawn("recover")
					}
					if inspectErr != nil {
						m.status.Phase = "error"
						m.status.Error = inspectErr.Error()
					} else {
						m.status.Phase = "recovering"
						m.status.Message = ErrRecoveryStarted.Error()
					}
					m.mu.Unlock()
					return
				}
				m.ready = true
				m.status.Phase = "upToDate"
				m.status.Message = "Update installed and startup confirmed"
				m.status.CanRollback = true
				m.status.RollbackVersion = p.OldVersion
				m.mu.Unlock()
				go m.worker()
				return
			}
		}
	}()
	return nil
}
