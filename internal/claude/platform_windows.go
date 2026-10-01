package claude

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
)

func managedFindings() []Finding {
	var findings []Finding
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramW6432")} {
		if base != "" {
			dir := filepath.Join(base, "ClaudeCode")
			findings = append(findings, managedFile(filepath.Join(dir, "managed-settings.json"), "managed/Windows")...)
			findings = append(findings, managedDirectory(filepath.Join(dir, "managed-settings.d"), "managed/Windows")...)
		}
	}
	subkey, _ := syscall.UTF16PtrFromString(`SOFTWARE\Policies\ClaudeCode`)
	valueName, _ := syscall.UTF16PtrFromString("Settings")
	for _, hive := range []syscall.Handle{syscall.HKEY_LOCAL_MACHINE, syscall.HKEY_CURRENT_USER} {
		for _, view := range []uint32{0x0100, 0x0200} {
			var key syscall.Handle
			err := syscall.RegOpenKeyEx(hive, subkey, 0, syscall.KEY_READ|view, &key)
			if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
				continue
			}
			if err != nil {
				findings = append(findings, Finding{Source: "managed/WindowsRegistry", Key: "Settings", Message: "managed registry policy cannot be inspected", Blocking: true})
				continue
			}
			var kind, size uint32
			err = syscall.RegQueryValueEx(key, valueName, nil, &kind, nil, &size)
			syscall.RegCloseKey(key)
			if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
				continue
			}
			message := "managed registry policy is present and unsupported"
			if err != nil {
				message = "managed registry policy cannot be inspected"
			}
			findings = append(findings, Finding{Source: "managed/WindowsRegistry", Key: "Settings", Message: message, Blocking: true})
		}
	}
	return findings
}

func runForeground(cmd *exec.Cmd) error {
	interrupts := make(chan os.Signal, 2)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	// The child shares the foreground console and receives Ctrl+C from Windows.
	// Keep the wrapper alive to collect and preserve the child's exit code.
	return cmd.Run()
}
func interruptedExitCode(exit *exec.ExitError) int { return exit.ExitCode() }
