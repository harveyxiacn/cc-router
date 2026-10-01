//go:build linux || darwin

package claude

import (
	"context"
	"io"
	"os"
	"sync"
	"syscall"
	"testing"
)

type startedWriter struct {
	once    sync.Once
	started chan struct{}
}

func (w *startedWriter) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	return len(data), nil
}

func TestForegroundSIGTERMIsForwardedAndExitCodePreserved(t *testing.T) {
	c := helperClient(t)
	t.Setenv("CCR_HELPER_RUN", "sleep")
	profile, cwd := t.TempDir(), t.TempDir()
	output := &startedWriter{started: make(chan struct{})}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-output.started:
			_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		case <-done:
		}
	}()
	code, err := c.Run(context.Background(), profile, cwd, nil, nil, output, io.Discard)
	if err != nil || code != 143 {
		t.Fatalf("forwarded signal exit: %d %v", code, err)
	}
}
