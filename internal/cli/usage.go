package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

type usageWindow struct {
	Used     *float64 `json:"used_percentage"`
	ResetsAt int64    `json:"resets_at"`
}
type usageInput struct {
	RateLimits *struct {
		FiveHour *usageWindow `json:"five_hour"`
		SevenDay *usageWindow `json:"seven_day"`
	} `json:"rate_limits"`
}

func (a *App) usage(args []string) (int, error) {
	if len(args) == 1 && args[0] == "config" {
		fmt.Fprintln(a.Out, `{"statusLine":{"type":"command","command":"cc-router usage statusline"}}`)
		return 0, nil
	}
	if len(args) == 0 || args[0] != "statusline" {
		return 0, errors.New("usage: ccr usage statusline [--prepare-at 90] [--switch-at 95], or ccr usage config")
	}
	prepare, switchAt := 90.0, 95.0
	for i := 1; i < len(args); i += 2 {
		if i+1 >= len(args) {
			return 0, errors.New("usage threshold needs a percentage")
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
	var input usageInput
	if err := json.Unmarshal(b, &input); err != nil {
		return 0, errors.New("invalid official statusline JSON; usage unknown")
	}
	now := time.Now().Unix()
	maxUsed := -1.0
	render := func(label string, w *usageWindow) string {
		if w == nil || w.Used == nil || *w.Used < 0 || *w.Used > 100 || w.ResetsAt <= now {
			return label + " unknown"
		}
		if *w.Used > maxUsed {
			maxUsed = *w.Used
		}
		return fmt.Sprintf("%s %.1f%% (reset %s)", label, *w.Used, time.Unix(w.ResetsAt, 0).Local().Format("01-02 15:04"))
	}
	var five, seven *usageWindow
	if input.RateLimits != nil {
		five = input.RateLimits.FiveHour
		seven = input.RateLimits.SevenDay
	}
	parts := []string{"CC Router", render("5h", five), render("7d", seven)}
	if maxUsed >= switchAt {
		parts = append(parts, "SWITCH RECOMMENDED: finish safely, update handoff, confirm switch")
	} else if maxUsed >= prepare {
		parts = append(parts, "PREPARE HANDOFF")
	}
	parts = append(parts, "last reported; /usage to verify")
	fmt.Fprintln(a.Out, strings.Join(parts, " | "))
	return 0, nil
}
