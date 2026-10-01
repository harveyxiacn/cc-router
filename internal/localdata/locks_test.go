package localdata

import (
	"os"
	"sync"
	"testing"
)

func TestConcurrentFirstLockCreation(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	start := make(chan struct{})
	failures := make(chan error, 32)
	var wg sync.WaitGroup
	var parked sync.WaitGroup
	parked.Add(32)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			parked.Done()
			<-start
			f, err := OpenLockFile(root, "shared.lock")
			if err == nil {
				err = f.Close()
			}
			failures <- err
		}()
	}
	parked.Wait()
	close(start)
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	f, err := root.OpenFile("shared.lock", os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("preserve contents")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	f, err = OpenLockFile(root, "shared.lock")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	got, err := Read(root, "shared.lock", 128)
	if err != nil || string(got) != "preserve contents" {
		t.Fatalf("lock content changed: %q %v", got, err)
	}
}
func TestOpenLockRejectsDirectoryAndMissingParent(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = root.Mkdir("directory", 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"directory", "missing/lock"} {
		if f, err := OpenLockFile(root, name); err == nil {
			f.Close()
			t.Fatalf("accepted %q", name)
		}
	}
}
