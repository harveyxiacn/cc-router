package state

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWithAccountUsesStableIDWithoutRewritingRegistryOrProfiles(t *testing.T) {
	s := testStore(t)
	a := addAccount(t, s, "one")
	registryPath := filepath.Join(s.Root, "accounts.json")
	original, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Unix(1000000, 0)
	if err := os.Chtimes(registryPath, old, old); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.ProfileDir(a)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.ProfileDir(a), []byte("official sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := s.WithAccount(a.ID, func(got Account) error {
		called = true
		if got != a {
			t.Fatalf("wrong identity: %+v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("callback not called")
	}
	after, err := os.Stat(registryPath)
	if err != nil || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("registry rewritten")
	}
	data, err := os.ReadFile(registryPath)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal("registry changed")
	}
	data, err = os.ReadFile(s.ProfileDir(a))
	if err != nil || string(data) != "official sentinel" {
		t.Fatal("profile path touched")
	}
}

func TestWithAccountRejectsInvalidOrMissingIDAndPropagatesCallbackError(t *testing.T) {
	s := testStore(t)
	a := addAccount(t, s, "one")
	for _, id := range []string{"../escape", strings.Repeat("f", 32)} {
		called := false
		if err := s.WithAccount(id, func(Account) error { called = true; return nil }); err == nil || called {
			t.Fatalf("accepted ID %q or invoked callback", id)
		}
	}
	sentinel := errors.New("callback failure")
	if err := s.WithAccount(a.ID, func(Account) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("callback error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "accounts.json"), []byte(`{"schemaVersion":99,"accounts":[],"bindings":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := s.WithAccount(a.ID, func(Account) error { called = true; return nil }); err == nil || called {
		t.Fatal("unsupported registry allowed callback")
	}
}
