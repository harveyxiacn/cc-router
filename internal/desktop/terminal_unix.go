//go:build linux || darwin

package desktop

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func openTerminal(executable, project string, args []string, dataRoot string) error {
	if runtime.GOOS == "darwin" {
		// Terminal.app opens command files. This narrow bridge only execs our CLI;
		// all data is single-quoted and the official CLI is still launched with argv.
		root, err := os.OpenRoot(dataRoot)
		if err != nil {
			return err
		}
		defer root.Close()
		if err := root.Mkdir("launch", 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		st, err := root.Lstat("launch")
		if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return errors.New("launch directory must be local")
		}
		file, err := os.CreateTemp(filepath.Join(dataRoot, "launch"), "terminal-*.command")
		if err != nil {
			return err
		}
		var q []string
		q = append(q, posixQuote(executable))
		for _, a := range args {
			q = append(q, posixQuote(a))
		}
		script := "#!/bin/sh\ncd -- " + posixQuote(project) + " || exit 1\nexec " + strings.Join(q, " ") + "\n"
		if _, err = file.WriteString(script); err == nil {
			err = file.Chmod(0700)
		}
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		cmd := exec.Command("/usr/bin/open", "-a", "Terminal", file.Name())
		return startTerminal(cmd)
	}
	candidates := []struct {
		name string
		args []string
	}{
		{"x-terminal-emulator", append([]string{"-e", executable}, args...)},
		{"konsole", append([]string{"--workdir", project, "-e", executable}, args...)},
		{"gnome-terminal", append([]string{"--working-directory", project, "--", executable}, args...)},
		{"kitty", append([]string{"--directory", project, executable}, args...)},
		{"alacritty", append([]string{"--working-directory", project, "-e", executable}, args...)},
		{"xterm", append([]string{"-e", executable}, args...)},
	}
	for _, c := range candidates {
		path, err := exec.LookPath(c.name)
		if err != nil {
			continue
		}
		cmd := exec.Command(path, c.args...)
		cmd.Dir = project
		return startTerminal(cmd)
	}
	return errors.New("no supported terminal found; install Konsole, GNOME Terminal, Kitty, Alacritty or xterm")
}
func startTerminal(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
