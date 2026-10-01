package desktop

import (
	"context"
	"errors"
	"os"

	"github.com/harveyxiacn/cc-router/internal/cli"
	"github.com/harveyxiacn/cc-router/internal/update"
)

type UpdateInfo = update.Status

var ErrRecoveryStarted = update.ErrRecoveryStarted
var ErrUpdateBusy = update.ErrBusy

func (s *Service) StartUpdates(ctx context.Context) error {
	s.updatesMu.Lock()
	defer s.updatesMu.Unlock()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	s.updater = update.NewManager(s.Store.Root, exe, s.CLIPath, cli.Version, func() error {
		registry, err := s.Store.Load()
		if err != nil {
			return err
		}
		for _, a := range registry.Accounts {
			unlock, err := s.Store.LockAccount(a.ID)
			if err != nil {
				return errors.New("close managed Claude sessions before updating")
			}
			unlock()
		}
		return nil
	})
	return s.updater.Start(ctx)
}
func (s *Service) CloseUpdates() {
	s.updatesMu.RLock()
	defer s.updatesMu.RUnlock()
	if s.updater != nil {
		s.updater.Close()
	}
}
func (s *Service) CheckForUpdates() (UpdateInfo, error) {
	s.updatesMu.RLock()
	defer s.updatesMu.RUnlock()
	if s.updater == nil {
		return UpdateInfo{}, errors.New("updater has not started")
	}
	return s.updater.Check()
}
func (s *Service) GetUpdateStatus() (UpdateInfo, error) {
	s.updatesMu.RLock()
	defer s.updatesMu.RUnlock()
	if s.updater == nil {
		return UpdateInfo{CurrentVersion: cli.Version, ReleaseURL: update.ReleasePage, Phase: "idle"}, nil
	}
	return s.updater.Status(), nil
}
func (s *Service) SetAutoUpdate(enabled bool) error {
	s.updatesMu.RLock()
	defer s.updatesMu.RUnlock()
	if s.updater == nil {
		return errors.New("updater has not started")
	}
	return s.updater.SetAuto(enabled)
}
func (s *Service) SetUpdateIdle(idle bool) error {
	s.updatesMu.RLock()
	defer s.updatesMu.RUnlock()
	if s.updater != nil {
		s.updater.SetIdle(idle)
	}
	return nil
}
func (s *Service) PrepareUpdate() error {
	s.updatesMu.RLock()
	defer s.updatesMu.RUnlock()
	if s.updater == nil {
		return errors.New("updater has not started")
	}
	return s.updater.Prepare()
}
func (s *Service) ApplyUpdate() error {
	s.updatesMu.RLock()
	defer s.updatesMu.RUnlock()
	if s.updater == nil {
		return errors.New("updater has not started")
	}
	return s.updater.Apply()
}
func (s *Service) RollbackUpdate() error {
	s.updatesMu.RLock()
	defer s.updatesMu.RUnlock()
	if s.updater == nil {
		return errors.New("updater has not started")
	}
	return s.updater.Rollback()
}
func (s *Service) ReportReady() error {
	s.updatesMu.RLock()
	defer s.updatesMu.RUnlock()
	if s.updater == nil {
		return nil
	}
	return s.updater.ReportReady()
}
