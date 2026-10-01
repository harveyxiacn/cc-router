package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpAndUsageDoNotCreateProfiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	t.Setenv("CCR_HOME", root)
	for _, args := range [][]string{{"--help"}, {"version"}, {"usage", "statusline"}} {
		out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
		if code := run(args, strings.NewReader(`{}`), out, errOut, false); code != 0 {
			t.Fatalf("%v: %d %s", args, code, errOut.String())
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("informational command created state")
	}
}

func TestMainMetadataCommandsDoNotRequireClaude(t *testing.T) {
	t.Setenv("CCR_HOME", filepath.Join(t.TempDir(), "state"))
	t.Setenv("CCR_CLAUDE_BIN", filepath.Join(t.TempDir(), "missing"))
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run([]string{"account", "add", "a"}, strings.NewReader(""), out, errOut, false); code != 0 {
		t.Fatalf("%d %s", code, errOut.String())
	}
	if code := run([]string{"run", "a"}, strings.NewReader(""), out, errOut, false); code == 0 {
		t.Fatal("missing Claude succeeded")
	}
}

func TestExplicitDataDirectorySurvivesTerminalEnvironment(t *testing.T) {
	t.Setenv("CCR_HOME", filepath.Join(t.TempDir(), "wrong"))
	root := filepath.Join(t.TempDir(), "chosen")
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	if code := run([]string{"--data-dir", root, "account", "add", "a"}, strings.NewReader(""), out, errOut, false); code != 0 {
		t.Fatalf("%d %s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(root, "accounts.json")); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"--data-dir", "relative", "init"}, strings.NewReader(""), out, errOut, false); code == 0 {
		t.Fatal("accepted relative private directory")
	}
}
