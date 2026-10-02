package usage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"time"

	"github.com/harveyxiacn/cc-router/internal/localdata"
	"github.com/harveyxiacn/cc-router/internal/state"
)

// ManualInput is explicitly supplied by the owner, independently of official reports.
type ManualInput struct {
	FiveHour     *Window `json:"fiveHour"`
	SevenDay     *Window `json:"sevenDay"`
	LimitedUntil int64   `json:"limitedUntil"`
}
type ManualRecord struct {
	ObservedAt   time.Time `json:"observedAt"`
	Source       string    `json:"source"`
	FiveHour     *Window   `json:"fiveHour"`
	SevenDay     *Window   `json:"sevenDay"`
	LimitedUntil int64     `json:"limitedUntil"`
}

func validateManual(record ManualRecord) error {
	if record.Source != "manual" || record.ObservedAt.IsZero() || record.ObservedAt.After(time.Now().Add(5*time.Minute)) {
		return errors.New("invalid manual observation source or time")
	}
	if record.FiveHour == nil && record.SevenDay == nil && record.LimitedUntil == 0 {
		return errors.New("provide a quota window or a local limit expiry")
	}
	deadline := record.ObservedAt.Add(366 * 24 * time.Hour).Unix()
	validExpiry := func(v int64) bool { return v > record.ObservedAt.Unix() && v <= deadline }
	for _, w := range []*Window{record.FiveHour, record.SevenDay} {
		if w != nil && (math.IsNaN(w.UsedPercentage) || math.IsInf(w.UsedPercentage, 0) || w.UsedPercentage < 0 || w.UsedPercentage > 100 || !validExpiry(w.ResetsAt)) {
			return errors.New("manual percentage must be 0..100 and reset time must be within the next year")
		}
	}
	if record.LimitedUntil != 0 && !validExpiry(record.LimitedUntil) {
		return errors.New("local limit expiry must be within the next year")
	}
	return nil
}

func manualRoot(s *state.Store, create bool) (*os.Root, error) {
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return nil, err
	}
	if create {
		if err = root.Mkdir("usage", 0700); err != nil && !errors.Is(err, os.ErrExist) {
			root.Close()
			return nil, err
		}
	}
	st, err := root.Lstat("usage")
	if err != nil {
		root.Close()
		return nil, err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		root.Close()
		return nil, errors.New("usage directory must be local")
	}
	return root, nil
}

func withManualAccount(s *state.Store, id string, fn func() error) error {
	if !accountID.MatchString(id) {
		return errors.New("invalid usage account ID")
	}
	return s.WithAccount(id, func(state.Account) error { return fn() })
}

func SaveManual(s *state.Store, id string, input ManualInput, now time.Time) error {
	record := ManualRecord{ObservedAt: now.UTC(), Source: "manual", FiveHour: input.FiveHour, SevenDay: input.SevenDay, LimitedUntil: input.LimitedUntil}
	if err := validateManual(record); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return errors.New("invalid manual observation")
	}
	return withManualAccount(s, id, func() error {
		root, err := manualRoot(s, true)
		if err != nil {
			return err
		}
		defer root.Close()
		return localdata.Write(root, "usage/manual-"+id+".json", data)
	})
}

func LoadManual(s *state.Store, id string) (*ManualRecord, error) {
	if !accountID.MatchString(id) {
		return nil, errors.New("invalid usage account ID")
	}
	root, err := manualRoot(s, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, err := localdata.Read(root, "usage/manual-"+id+".json", 16*1024)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	check := json.NewDecoder(bytes.NewReader(data))
	if err = unique(check); err != nil {
		return nil, errors.New("invalid manual observation JSON")
	}
	if _, err = check.Token(); err != io.EOF {
		return nil, errors.New("invalid trailing manual observation JSON")
	}
	var record ManualRecord
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&record); err != nil {
		return nil, errors.New("invalid manual observation fields")
	}
	if err = validateManual(record); err != nil {
		return nil, err
	}
	return &record, nil
}

func ClearManual(s *state.Store, id string) error {
	return withManualAccount(s, id, func() error {
		root, err := manualRoot(s, false)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer root.Close()
		name := "usage/manual-" + id + ".json"
		st, err := root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !st.Mode().IsRegular() {
			return errors.New("manual observation must be a regular file")
		}
		return root.Remove(name)
	})
}
