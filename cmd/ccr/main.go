package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/harveyxiacn/cc-router/internal/claude"
	"github.com/harveyxiacn/cc-router/internal/cli"
	"github.com/harveyxiacn/cc-router/internal/state"
)

func main() {
	info, err := os.Stdin.Stat()
	interactive := err == nil && info.Mode()&os.ModeCharDevice != 0
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, interactive))
}

func run(args []string, in io.Reader, out, errout io.Writer, interactive bool) int {
	if len(args) > 0 && args[0] == "--data-dir" {
		if len(args) < 3 || !filepath.IsAbs(args[1]) {
			fmt.Fprintln(errout, "cc-router: --data-dir requires an absolute directory and a command")
			return 2
		}
		previous, existed := os.LookupEnv("CCR_HOME")
		if err := os.Setenv("CCR_HOME", filepath.Clean(args[1])); err != nil {
			fmt.Fprintln(errout, "cc-router: could not set private data directory")
			return 2
		}
		defer func() {
			if existed {
				_ = os.Setenv("CCR_HOME", previous)
			} else {
				_ = os.Unsetenv("CCR_HOME")
			}
		}()
		args = args[2:]
	}
	app := &cli.App{In: in, Out: out, Err: errout, Interactive: interactive}
	needsStore := len(args) > 0
	if len(args) > 0 {
		switch args[0] {
		case "--help", "-h", "help", "--version", "version", "usage":
			needsStore = false
		}
	}
	if needsStore {
		s, err := state.Open("")
		if err != nil {
			fmt.Fprintln(errout, "cc-router:", err)
			return 2
		}
		app.Store = s
		dir, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(errout, "cc-router: cannot determine working directory")
			return 2
		}
		app.Dir = dir
	}
	app.Launch = func(profile, dir string, args []string) (int, error) {
		client, err := claude.Resolve()
		if err != nil {
			return 0, err
		}
		return client.Run(context.Background(), profile, dir, args, in, out, errout)
	}
	app.Inspect = func(profile, dir string) (string, error) {
		client, err := claude.Resolve()
		if err != nil {
			return "Identity: unknown", err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		status, err := client.Status(ctx, profile, dir)
		if err != nil {
			return "Identity: unknown. Verify with official /status.", err
		}
		if !status.Known {
			return "Identity: unknown. Verify with official /status.", nil
		}
		email := status.Email
		if email == "" {
			email = "unknown"
		}
		method := status.AuthMethod
		if method == "" {
			method = "unknown"
		}
		return fmt.Sprintf("Official status: logged in=%t; email=%s; auth=%s; Claude=%s\nQuota: use the official statusline or /usage; identity does not establish remaining quota.", status.LoggedIn, email, method, status.Version), nil
	}
	app.Diagnose = func(profile, dir string) (string, error) {
		var b strings.Builder
		var versionErr error
		client, resolveErr := claude.Resolve()
		if resolveErr != nil {
			fmt.Fprintln(&b, "Claude CLI unavailable; install the official Claude Code executable.")
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			version, err := client.Version(ctx)
			if err != nil {
				versionErr = err
				fmt.Fprintln(&b, "Claude version: unknown")
			} else {
				fmt.Fprintln(&b, "Claude version:", version)
			}
		}
		findings, checkErr := claude.Check(profile, dir, nil, os.Environ())
		blocked := false
		for _, f := range findings {
			level := "notice"
			if f.Blocking {
				level = "BLOCK"
				blocked = true
			}
			fmt.Fprintf(&b, "%s: %s / %s: %s\n", level, f.Source, f.Key, f.Message)
		}
		if len(findings) == 0 && checkErr == nil {
			fmt.Fprintln(&b, "No recognized authentication conflicts found in the inspected sources.")
		}
		fmt.Fprintln(&b, "This is a launch-time check. Confirm identity and billing in official /status; settings may change during a session.")
		if checkErr != nil {
			return b.String(), checkErr
		}
		if resolveErr != nil {
			return b.String(), resolveErr
		}
		if versionErr != nil {
			return b.String(), versionErr
		}
		if blocked {
			return b.String(), errors.New("resolve the reported authentication/configuration conflicts before launch")
		}
		return b.String(), nil
	}
	code, err := app.Execute(args)
	if err != nil {
		fmt.Fprintln(errout, "cc-router:", err)
		return 2
	}
	return code
}
