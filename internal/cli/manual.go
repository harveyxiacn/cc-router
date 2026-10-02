package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/harveyxiacn/cc-router/internal/usage"
)

func (a *App) manualUsage(args []string) (int, error) {
	if a.Store == nil {
		return 0, errors.New("account store is unavailable")
	}
	if len(args) < 2 {
		return 0, errors.New("usage: cc-router usage show|record|clear NAME")
	}
	r, err := a.Store.Load()
	if err != nil {
		return 0, err
	}
	account, err := r.Find(args[1])
	if err != nil {
		return 0, err
	}
	switch args[0] {
	case "show":
		if len(args) != 2 {
			return 0, errors.New("usage: cc-router usage show NAME")
		}
		official, err := usage.Load(a.Store, account.ID)
		if err != nil {
			return 0, err
		}
		manual, err := usage.LoadManual(a.Store, account.ID)
		if err != nil {
			return 0, err
		}
		return 0, json.NewEncoder(a.Out).Encode(struct {
			Official *usage.Snapshot     `json:"official"`
			Manual   *usage.ManualRecord `json:"manual"`
		}{official, manual})
	case "clear":
		if len(args) != 2 {
			return 0, errors.New("usage: cc-router usage clear NAME")
		}
		return 0, usage.ClearManual(a.Store, account.ID)
	case "record":
		input, err := parseManualInput(args[2:])
		if err != nil {
			return 0, err
		}
		if err = usage.SaveManual(a.Store, account.ID, input, time.Now()); err != nil {
			return 0, err
		}
		fmt.Fprintln(a.Out, "Manual observation saved separately from official reports; verify current limits with official /usage.")
		return 0, nil
	}
	return 0, errors.New("unknown manual usage action")
}

func parseManualInput(args []string) (usage.ManualInput, error) {
	var result usage.ManualInput
	flags := map[string]string{}
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) {
			return result, errors.New("manual usage option requires a value")
		}
		name := args[i]
		if _, exists := flags[name]; exists {
			return result, errors.New("duplicate manual usage option")
		}
		switch name {
		case "--five-hour", "--five-reset", "--seven-day", "--seven-reset", "--limited-until":
		default:
			return result, errors.New("unknown manual usage option")
		}
		flags[name] = args[i+1]
	}
	parseTime := func(value string) (int64, error) {
		t, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return 0, errors.New("manual reset/limit times must use RFC3339 with a timezone")
		}
		return t.Unix(), nil
	}
	for _, item := range []struct {
		value, reset string
		target       **usage.Window
	}{{"--five-hour", "--five-reset", &result.FiveHour}, {"--seven-day", "--seven-reset", &result.SevenDay}} {
		v, hasValue := flags[item.value]
		reset, hasReset := flags[item.reset]
		if hasValue != hasReset {
			return result, errors.New("provide both percentage and reset time for each quota window")
		}
		if !hasValue {
			continue
		}
		percentage, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return result, errors.New("invalid manual usage percentage")
		}
		stamp, err := parseTime(reset)
		if err != nil {
			return result, err
		}
		*item.target = &usage.Window{UsedPercentage: percentage, ResetsAt: stamp}
	}
	if value, ok := flags["--limited-until"]; ok {
		var err error
		result.LimitedUntil, err = parseTime(value)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}
