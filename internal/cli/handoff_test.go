package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandoffPreservesUserNotesAndExcludesSecrets(t *testing.T) {
	dir := t.TempDir()
	git := exec.Command("git", "init", dir)
	if out, err := git.CombinedOutput(); err != nil {
		t.Skipf("git unavailable: %v %s", err, out)
	}
	for name, content := range map[string]string{"main.go": "package main", ".env": "DO_NOT_COPY", "private.pem": "DO_NOT_COPY", "notes.md": "PRIVATE_CONTENT"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path, created, err := WriteHandoff(dir)
	if err != nil || !created {
		t.Fatalf("handoff: %v %v", created, err)
	}
	content, _ := os.ReadFile(path)
	text := string(content)
	if strings.Contains(text, "DO_NOT_COPY") || strings.Contains(text, "PRIVATE_CONTENT") || strings.Contains(text, ".env") || strings.Contains(text, "private.pem") {
		t.Fatalf("sensitive content included: %s", text)
	}
	if !strings.Contains(text, "main.go") || !strings.Contains(text, "Next steps") {
		t.Fatalf("missing summary: %s", text)
	}
	if err := os.WriteFile(path, []byte("user notes"), 0600); err != nil {
		t.Fatal(err)
	}
	_, created, err = WriteHandoff(dir)
	if err != nil || created {
		t.Fatalf("existing handoff: %v %v", created, err)
	}
	content, _ = os.ReadFile(path)
	if string(content) != "user notes" {
		t.Fatal("overwrote notes")
	}
	cmd := exec.Command("git", "check-ignore", ".cc-router/handoff.md")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatal("handoff is not ignored")
	}
}

func TestHandoffWorksWithoutGitRepository(t *testing.T) {
	path, created, err := WriteHandoff(t.TempDir())
	if err != nil || !created {
		t.Fatalf("%v %v", created, err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "unavailable") {
		t.Fatal("missing unavailable marker")
	}
}

func TestSensitivePathsAndTerminalEscapes(t *testing.T) {
	for _, s := range []string{".env.production", "a/.credentials.json", "certs/key.pem", ".claude/history.jsonl", "id_ed25519", "secrets.txt", "token.txt"} {
		if !sensitivePath(s) {
			t.Errorf("not filtered: %s", s)
		}
	}
	summary := summarizeStatus([]byte("?? safe.go\x00?? .env\x00R  new.go\x00old.go\x00?? evil\x1b[2J.md\x00"))
	if strings.Contains(summary, "\x1b") || strings.Contains(summary, ".env") {
		t.Fatal("unsanitized summary")
	}
	if !strings.Contains(summary, "safe.go") || !strings.Contains(summary, "new.go") {
		t.Fatal("missing file summary")
	}
}

func TestHandoffRejectsLinkedDirectory(t *testing.T) {
	dir := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(dir, ".cc-router")); err != nil {
		t.Skip("symlink unavailable")
	}
	if _, _, err := WriteHandoff(dir); err == nil {
		t.Fatal("followed symlink")
	}
	if _, err := os.Stat(filepath.Join(target, "handoff.md")); !os.IsNotExist(err) {
		t.Fatal("wrote outside project")
	}
}

func TestHandoffPreservesAndExtendsExistingIgnoreFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".cc-router"), 0700); err != nil {
		t.Fatal(err)
	}
	ignore := filepath.Join(dir, ".cc-router", ".gitignore")
	if err := os.WriteFile(ignore, []byte("# user note\n*.log\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteHandoff(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(ignore)
	if !strings.Contains(string(b), "# user note") || !strings.Contains(string(b), "\n*\n") {
		t.Fatal("local ignore does not safely cover handoff")
	}
}

func TestHandoffDoesNotFollowGitExcludeLink(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, ".env")
	if err := os.WriteFile(secret, []byte("SENSITIVE_UNCHANGED"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, ".git", "info", "exclude")); err != nil {
		t.Skip("symlink creation unavailable")
	}
	if _, _, err := WriteHandoff(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(secret)
	if string(b) != "SENSITIVE_UNCHANGED" {
		t.Fatal("modified symlink target")
	}
}
