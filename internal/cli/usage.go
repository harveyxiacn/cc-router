package cli

import (
	"errors"
	"fmt"
	"github.com/harveyxiacn/cc-router/internal/state"
	"github.com/harveyxiacn/cc-router/internal/usage"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func (a *App) usage(args []string) (int, error) {
	if len(args) == 1 && args[0] == "config" {
		fmt.Fprintln(a.Out, `{"statusLine":{"type":"command","command":"cc-router usage statusline"}}`)
		return 0, nil
	}
	if len(args) == 0 || args[0] != "statusline" {
		return 0, errors.New("usage: ccr usage statusline [--prepare-at 90] [--switch-at 95], or ccr usage config")
	}
	prepare, switchAt := 90.0, 95.0
	accountID := ""
	for i := 1; i < len(args); i += 2 {
		if i+1 >= len(args) {
			return 0, errors.New("usage threshold needs a percentage")
		}
		if args[i] == "--account-id" {
			accountID = args[i+1]
			continue
		}
		n, err := strconv.ParseFloat(args[i+1], 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, errors.New("invalid usage threshold")
		}
		switch args[i] {
		case "--prepare-at":
			prepare = n
		case "--switch-at":
			switchAt = n
		default:
			return 0, errors.New("unknown usage option")
		}
	}
	if prepare < 1 || switchAt > 100 || prepare > switchAt {
		return 0, errors.New("thresholds must satisfy 1 <= prepare <= switch <= 100")
	}
	b, err := io.ReadAll(io.LimitReader(a.In, 1024*1024+1))
	if err != nil || len(b) > 1024*1024 {
		return 0, errors.New("usage input exceeds limit or cannot be read")
	}
	observation, err := usage.Decode(b, time.Now())
	if err != nil {
		return 0, errors.New("invalid official statusline JSON; usage unknown")
	}
	if accountID != "" {
		s, err := state.Open("")
		if err != nil {
			return 0, errors.New("usage cache unavailable")
		}
		r, err := s.Load()
		if err != nil {
			return 0, errors.New("usage registry unavailable")
		}
		found := false
		for _, account := range r.Accounts {
			if account.ID == accountID {
				expected := filepath.Clean(s.ProfileDir(account))
				actual := filepath.Clean(os.Getenv("CLAUDE_CONFIG_DIR"))
				matches := expected == actual
				if runtime.GOOS == "windows" {
					matches = strings.EqualFold(expected, actual)
				}
				if !matches {
					return 0, errors.New("usage callback does not match the selected profile")
				}
				found = true
				break
			}
		}
		if !found {
			return 0, errors.New("usage account is no longer registered")
		}
		if err := usage.Save(s, accountID, observation); err != nil {
			return 0, errors.New("usage observation could not be saved")
		}
	}
	now := time.Now().Unix()
	maxUsed := -1.0
	render := func(label string, w *usage.Window) string {
		if w == nil || w.ResetsAt <= now {
			return label + " unknown"
		}
		if w.UsedPercentage > maxUsed {
			maxUsed = w.UsedPercentage
		}
		return fmt.Sprintf("%s %.1f%% (reset %s)", label, w.UsedPercentage, time.Unix(w.ResetsAt, 0).Local().Format("01-02 15:04"))
	}
	parts := []string{"CC Router", render("5h", observation.FiveHour), render("7d", observation.SevenDay)}
	if maxUsed >= switchAt {
		parts = append(parts, "SWITCH RECOMMENDED: finish safely, update handoff, confirm switch")
	} else if maxUsed >= prepare {
		parts = append(parts, "PREPARE HANDOFF")
	}
	parts = append(parts, "last reported; /usage to verify")
	fmt.Fprintln(a.Out, strings.Join(parts, " | "))
	return 0, nil
}
