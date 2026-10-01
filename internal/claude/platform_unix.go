//go:build linux || darwin

package claude

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
)

func managedFindings() []Finding {
	base := "/etc/claude-code"
	if runtime.GOOS == "darwin" {
		base = "/Library/Application Support/ClaudeCode"
	}
	findings := managedFile(filepath.Join(base, "managed-settings.json"), "managed/system")
	findings = append(findings, managedDirectory(filepath.Join(base, "managed-settings.d"), "managed/system")...)
	if runtime.GOOS == "darwin" {
		for _, path := range []string{"/Library/Managed Preferences/com.anthropic.claudecode.plist", "/Library/Preferences/com.anthropic.claudecode.plist"} {
			findings = append(findings, managedFile(path, "managed/macOSPreferences")...)
		}
		findings = append(findings, Finding{Source: "managed/macOSMDM", Key: "verificationLimit", Message: "macOS dynamic MDM preferences cannot be fully verified by this alpha", Blocking: false})
	}
	return findings
}

func runForeground(cmd *exec.Cmd) error {
	notifications := make(chan os.Signal, 2)
	signal.Notify(notifications, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(notifications)
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case err := <-done:
			return err
		case sig := <-notifications:
			// A terminal Ctrl+C already reaches the foreground child. Forwarding
			// SIGINT here would deliver it twice. Parent-only SIGINT is not forwarded.
			if sig == syscall.SIGTERM {
				_ = cmd.Process.Signal(sig)
			}
		}
	}
}
func interruptedExitCode(exit *exec.ExitError) int {
	if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return 1
}
