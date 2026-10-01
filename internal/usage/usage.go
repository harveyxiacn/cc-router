// Package usage stores only documented quota observations, never raw statusline data.
package usage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"time"

	"github.com/harveyxiacn/cc-router/internal/localdata"
	"github.com/harveyxiacn/cc-router/internal/state"
)

type Window struct {
	UsedPercentage float64 `json:"usedPercentage"`
	ResetsAt       int64   `json:"resetsAt"`
}
type Snapshot struct {
	ObservedAt time.Time `json:"observedAt"`
	FiveHour   *Window   `json:"fiveHour"`
	SevenDay   *Window   `json:"sevenDay"`
	Source     string    `json:"source"`
}

func Decode(data []byte, now time.Time) (Snapshot, error) {
	result := Snapshot{ObservedAt: now.UTC(), Source: "official-statusline"}
	if len(data) > 1024*1024 {
		return result, errors.New("usage input exceeds size limit")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return result, errors.New("usage input must be a JSON object")
	}
	// Detect duplicate keys without retaining any unrelated values in the persisted structure.
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := unique(dec); err != nil {
		return result, errors.New("invalid statusline JSON")
	}
	if _, err := dec.Token(); err != io.EOF {
		return result, errors.New("invalid statusline JSON")
	}
	type inputWindow struct {
		Used     *float64 `json:"used_percentage"`
		ResetsAt int64    `json:"resets_at"`
	}
	var input struct {
		RateLimits *struct {
			Five  *inputWindow `json:"five_hour"`
			Seven *inputWindow `json:"seven_day"`
		} `json:"rate_limits"`
	}
	if err := json.Unmarshal(data, &input); err != nil {
		return result, errors.New("unsupported statusline usage data")
	}
	normalize := func(w *inputWindow) *Window {
		if w == nil || w.Used == nil || *w.Used < 0 || *w.Used > 100 || w.ResetsAt <= now.Unix() {
			return nil
		}
		return &Window{*w.Used, w.ResetsAt}
	}
	if input.RateLimits != nil {
		result.FiveHour = normalize(input.RateLimits.Five)
		result.SevenDay = normalize(input.RateLimits.Seven)
	}
	return result, nil
}

var accountID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func Save(s *state.Store, id string, snapshot Snapshot) error {
	if !accountID.MatchString(id) {
		return errors.New("invalid usage account ID")
	}
	snapshot.Source = "official-statusline"
	if err := validate(snapshot); err != nil {
		return err
	}
	// Reuse the registry transaction lock for short metadata writes. The callback
	// confirms this is a registered account; official profile files are not read.
	return s.Update(func(r *state.Registry) error {
		found := false
		for _, a := range r.Accounts {
			if a.ID == id {
				found = true
			}
		}
		if !found {
			return errors.New("usage account is no longer registered")
		}
		root, err := os.OpenRoot(s.Root)
		if err != nil {
			return err
		}
		defer root.Close()
		if err := root.Mkdir("usage", 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		st, err := root.Lstat("usage")
		if err != nil {
			return err
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return errors.New("usage directory must be local")
		}
		data, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		return localdata.Write(root, "usage/"+id+".json", data)
	})
}

func Load(s *state.Store, id string) (*Snapshot, error) {
	if !accountID.MatchString(id) {
		return nil, errors.New("invalid usage account ID")
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	st, err := root.Lstat("usage")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("usage directory must be local")
	}
	b, err := localdata.Read(root, "usage/"+id+".json", 16*1024)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result Snapshot
	check := json.NewDecoder(bytes.NewReader(b))
	if err := unique(check); err != nil {
		return nil, errors.New("invalid local usage observation")
	}
	if _, err := check.Token(); err != io.EOF {
		return nil, errors.New("invalid local usage observation")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&result); err != nil {
		return nil, errors.New("invalid local usage observation")
	}
	if err := validate(result); err != nil {
		return nil, err
	}
	return &result, nil
}
func validate(s Snapshot) error {
	if s.Source != "official-statusline" || s.ObservedAt.IsZero() {
		return errors.New("invalid usage observation")
	}
	for _, w := range []*Window{s.FiveHour, s.SevenDay} {
		if w != nil && (w.UsedPercentage < 0 || w.UsedPercentage > 100 || w.ResetsAt <= 0) {
			return errors.New("invalid usage window")
		}
	}
	return nil
}
func unique(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	v, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	for d.More() {
		if v == '{' {
			k, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := k.(string)
			if !ok || seen[s] {
				return errors.New("duplicate key")
			}
			seen[s] = true
		}
		if err := unique(d); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}
