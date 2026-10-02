package desktop

import (
	"github.com/harveyxiacn/cc-router/internal/usage"
	"time"
)

type ManualInput = usage.ManualInput

func (s *Service) RecordManualUsage(name string, input ManualInput) error {
	a, err := s.account(name)
	if err != nil {
		return err
	}
	return usage.SaveManual(s.Store, a.ID, input, time.Now())
}
func (s *Service) ClearManualUsage(name string) error {
	a, err := s.account(name)
	if err != nil {
		return err
	}
	return usage.ClearManual(s.Store, a.ID)
}
