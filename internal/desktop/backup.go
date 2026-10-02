package desktop

import (
	"errors"
	"github.com/harveyxiacn/cc-router/internal/state"
)

type BackupInfo = state.BackupInfo
type BackupPreview = state.BackupPreview

func (s *Service) ListBackups() ([]BackupInfo, error)             { return s.Store.ListBackups() }
func (s *Service) CreateBackup() (BackupInfo, error)              { return s.Store.CreateBackup() }
func (s *Service) PreviewBackup(id string) (BackupPreview, error) { return s.Store.PreviewBackup(id) }
func (s *Service) RestoreBackup(id, digest string, reviewed bool) (BackupInfo, error) {
	if !reviewed {
		return BackupInfo{}, errors.New("review the backup and confirm restoration first")
	}
	return s.Store.RestoreBackup(id, digest)
}
