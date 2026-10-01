package desktop

import (
	"errors"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

func openTerminal(executable, project string, args []string, _ string) error {
	// ShellExecute launches the native CLI file with a new console, not cmd.exe.
	// EscapeArg follows the Windows argv convention; no command shell is involved.
	var quoted []string
	for _, a := range args {
		quoted = append(quoted, syscall.EscapeArg(a))
	}
	verb, _ := syscall.UTF16PtrFromString("open")
	file, err := syscall.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	params, err := syscall.UTF16PtrFromString(strings.Join(quoted, " "))
	if err != nil {
		return err
	}
	cwd, err := syscall.UTF16PtrFromString(project)
	if err != nil {
		return err
	}
	proc := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")
	result, _, _ := proc.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), uintptr(unsafe.Pointer(params)), uintptr(unsafe.Pointer(cwd)), 1)
	runtime.KeepAlive(verb)
	runtime.KeepAlive(file)
	runtime.KeepAlive(params)
	runtime.KeepAlive(cwd)
	if result <= 32 {
		return errors.New("Windows could not open the CC Router terminal")
	}
	return nil
}
