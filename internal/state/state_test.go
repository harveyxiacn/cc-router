package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func addAccount(t *testing.T, s *Store, name string) Account {
	t.Helper()
	var a Account
	err := s.Update(func(r *Registry) (err error) { a, err = r.Add(name, "Label "+name); return })
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestStableProfileAfterRenameAndRemove(t *testing.T) {
	s := testStore(t)
	a := addAccount(t, s, "personal")
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(a.ID) {
		t.Fatalf("ID is not a random 128-bit identifier: %q", a.ID)
	}
	profile := s.ProfileDir(a)
	if err := os.WriteFile(filepath.Join(profile, "opaque-official-data"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(r *Registry) error { return r.Rename("personal", "work", "Work account") }); err != nil {
		t.Fatal(err)
	}
	r, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := r.Find("work")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.ID != a.ID || renamed.Label != "Work account" || s.ProfileDir(renamed) != profile {
		t.Fatalf("rename changed profile identity: %+v", renamed)
	}
	if err := s.Update(func(r *Registry) error { return r.Remove("work") }); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(profile, "opaque-official-data"))
	if err != nil || string(data) != "keep" {
		t.Fatalf("remove touched profile data: %q, %v", data, err)
	}
	b := addAccount(t, s, "work")
	if b.ID == a.ID {
		t.Fatal("re-added account reused deleted identity")
	}
}

func TestSelectionPrecedenceAndReferenceCleanup(t *testing.T) {
	s := testStore(t)
	first := addAccount(t, s, "first")
	bound := addAccount(t, s, "bound")
	explicit := addAccount(t, s, "explicit")
	project, err := CanonicalProject(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = s.Update(func(r *Registry) error {
		if err := r.SetDefault("first"); err != nil {
			return err
		}
		return r.Bind(project, "bound")
	})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := s.Load()
	for _, tc := range []struct{ name, project, id string }{{"explicit", project, explicit.ID}, {"", project, bound.ID}, {"", "", first.ID}} {
		a, err := r.Select(tc.name, tc.project)
		if err != nil || a.ID != tc.id {
			t.Fatalf("Select(%q,%q)=%+v,%v", tc.name, tc.project, a, err)
		}
	}
	if _, err := r.Select("missing", project); err == nil {
		t.Fatal("explicit unknown account fell back")
	}
	if err := s.Update(func(r *Registry) error {
		if err := r.Remove("bound"); err != nil {
			return err
		}
		return r.Remove("first")
	}); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Load()
	if r.DefaultID != "" || len(r.Bindings) != 0 {
		t.Fatalf("dangling references: %+v", r)
	}
	if _, err := r.Select("", ""); !errors.Is(err, ErrNoSelection) {
		t.Fatalf("missing selection = %v", err)
	}
}

func TestInvalidMetadataDoesNotPersist(t *testing.T) {
	s := testStore(t)
	addAccount(t, s, "valid")
	for _, name := range []string{"", "../escape", "UPPER", "space name", strings.Repeat("x", 65), "valid"} {
		if err := s.Update(func(r *Registry) error { _, err := r.Add(name, ""); return err }); err == nil {
			t.Errorf("accepted name %q", name)
		}
	}
	before, _ := os.ReadFile(filepath.Join(s.Root, "accounts.json"))
	if err := s.Update(func(r *Registry) error { r.Accounts[0].ID = "../escape"; return nil }); err == nil {
		t.Fatal("accepted escaping ID")
	}
	if err := s.Update(func(r *Registry) error { r.Accounts[0].Label = "secret\ncontrol"; return nil }); err == nil {
		t.Fatal("accepted terminal control")
	}
	after, _ := os.ReadFile(filepath.Join(s.Root, "accounts.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("failed transaction modified registry")
	}
}

func TestCorruptionAndFutureSchemaAreNeverOverwritten(t *testing.T) {
	for _, input := range []string{`{broken`, `{"schemaVersion":99,"accounts":[],"bindings":{}}`, `{"schemaVersion":1,"accounts":[],"bindings":{},"token":"no"}`, `{"schemaVersion":1,"accounts":[],"bindings":{}} {}`} {
		t.Run(input, func(t *testing.T) {
			s := testStore(t)
			path := filepath.Join(s.Root, "accounts.json")
			if err := os.WriteFile(path, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Load(); err == nil {
				t.Fatal("loaded corrupt or unsupported registry")
			}
			if err := s.Update(func(r *Registry) error { _, err := r.Add("new", ""); return err }); err == nil {
				t.Fatal("overwrote corrupt registry")
			}
			data, _ := os.ReadFile(path)
			if string(data) != input {
				t.Fatal("corrupt registry changed")
			}
		})
	}
}

func TestExportImportContainsMetadataOnlyAndPreservesIdentity(t *testing.T) {
	s := testStore(t)
	a := addAccount(t, s, "one")
	project, _ := CanonicalProject(t.TempDir())
	if err := s.Update(func(r *Registry) error {
		if err := r.SetDefault("one"); err != nil {
			return err
		}
		return r.Bind(project, "one")
	}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := s.Export(&buf); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{a.ID, s.Root, project, "bindings", "configDir"} {
		if strings.Contains(buf.String(), secret) {
			t.Fatalf("export exposed %q", secret)
		}
	}
	other := testStore(t)
	existing := addAccount(t, other, "one")
	if err := other.Import(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatal(err)
	}
	r, _ := other.Load()
	imported, err := r.Select("", "")
	if err != nil || imported.ID != existing.ID {
		t.Fatalf("import replaced existing identity: %+v %v", imported, err)
	}
	if len(r.Bindings) != 0 {
		t.Fatal("import introduced project paths")
	}
	if err := other.Import(strings.NewReader(`{"schemaVersion":1,"accounts":[{"name":"two","label":"Two"}]}`)); err != nil {
		t.Fatal(err)
	}
	r, _ = other.Load()
	added, err := r.Find("two")
	if err != nil || added.ID == "" || added.ID == a.ID {
		t.Fatalf("new import ID: %+v %v", added, err)
	}
}

func TestImportStrictAndTransactional(t *testing.T) {
	s := testStore(t)
	addAccount(t, s, "one")
	before, _ := os.ReadFile(filepath.Join(s.Root, "accounts.json"))
	for _, input := range []string{
		`{"schemaVersion":2,"accounts":[]}`,
		`{"schemaVersion":1,"accounts":[{"name":"two","label":"ok","configDir":"/tmp"}]}`,
		`{"schemaVersion":1,"accounts":[{"name":"two","label":"ok"},{"name":"../bad","label":"bad"}]}`,
		`{"schemaVersion":1,"accounts":[{"name":"two"},{"name":"two"}]}`,
		`{"schemaVersion":1,"accounts":[],"defaultAccount":"missing"}`,
		`{"schemaVersion":1,"accounts":[],"token":"secret"}`,
		`{"schemaVersion":1,"accounts":[]} {}`,
		strings.Repeat(" ", 1024*1024+1),
	} {
		if err := s.Import(strings.NewReader(input)); err == nil {
			t.Errorf("accepted invalid import %.150q", input)
		}
		after, _ := os.ReadFile(filepath.Join(s.Root, "accounts.json"))
		if !bytes.Equal(before, after) {
			t.Fatal("failed import partially changed state")
		}
	}
}

func TestConcurrentTransactionsDoNotLoseUpdates(t *testing.T) {
	s := testStore(t)
	var wg sync.WaitGroup
	failures := make(chan error, 24)
	for i := range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			failures <- s.Update(func(r *Registry) error { _, err := r.Add(fmt.Sprintf("account-%d", i), ""); return err })
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err := s.Load()
	if err != nil || len(r.Accounts) != 24 {
		t.Fatalf("lost concurrent transaction: %d %v", len(r.Accounts), err)
	}
}

func TestCrossProcessTransactions(t *testing.T) {
	if root := os.Getenv("CCR_STATE_TEST_ROOT"); root != "" {
		s, err := Open(root)
		if err != nil {
			t.Fatal(err)
		}
		for i := range 8 {
			err = s.Update(func(r *Registry) error { _, err := r.Add(fmt.Sprintf("p%d-%d", os.Getpid(), i), ""); return err })
			if err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	s := testStore(t)
	var wg sync.WaitGroup
	failures := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestCrossProcessTransactions$")
			cmd.Env = append(os.Environ(), "CCR_STATE_TEST_ROOT="+s.Root)
			out, err := cmd.CombinedOutput()
			if err != nil {
				failures <- fmt.Errorf("child: %w: %s", err, out)
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	r, err := s.Load()
	if err != nil || len(r.Accounts) != 32 {
		t.Fatalf("lost cross-process transaction: %d %v", len(r.Accounts), err)
	}
}

func TestRootOverrideAndCanonicalProject(t *testing.T) {
	t.Setenv("CCR_HOME", "relative")
	if _, err := Open(""); err == nil {
		t.Fatal("accepted relative CCR_HOME")
	}
	root := filepath.Join(t.TempDir(), "override")
	t.Setenv("CCR_HOME", root)
	s, err := Open("")
	if err != nil || s.Root != root {
		t.Fatalf("override: %+v %v", s, err)
	}
	if _, err := CanonicalProject(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("accepted nonexistent project")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := CanonicalProject(file); err == nil {
		t.Fatal("accepted file as project")
	}
	var raw map[string]any
	var buf bytes.Buffer
	if err := s.Export(&buf); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
}
