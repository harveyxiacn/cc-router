package update

import (
	"os/exec"
	"syscall"
)

var processKernel = syscall.NewLazyDLL("kernel32.dll")
var openUpdateProcess = processKernel.NewProc("OpenProcess")
var waitUpdateProcess = processKernel.NewProc("WaitForSingleObject")

func hideHelper(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000, HideWindow: true}
}
func processAlive(pid int) (bool, error) {
	h, _, err := openUpdateProcess.Call(0x00100000, 0, uintptr(pid))
	if h == 0 {
		if err == syscall.Errno(87) {
			return false, nil
		}
		return false, err
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	result, _, err := waitUpdateProcess.Call(h, 0)
	if result == 0 {
		return false, nil
	}
	if result == 258 {
		return true, nil
	}
	return false, err
}
