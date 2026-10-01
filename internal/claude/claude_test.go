package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func helperClient(t *testing.T) Client {
	t.Helper()
	t.Setenv("CCR_CLAUDE_HELPER", "1")
	return Client{Executable: os.Args[0], PrefixArgs: []string{"-test.run=^TestClaudeHelper$", "--"}}
}

func TestClaudeHelper(t *testing.T) {
	if os.Getenv("CCR_CLAUDE_HELPER") != "1" {
		return
	}
	var args []string
	for i, arg := range os.Args {
		if arg == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	if len(args) == 1 && args[0] == "--version" {
		version := os.Getenv("CCR_HELPER_VERSION")
		if version == "" {
			version = "2.1.285"
		}
		fmt.Print(version + " (Claude Code)\n")
		os.Exit(0)
	}
	if len(args) >= 2 && args[0] == "auth" && args[1] == "status" {
		switch os.Getenv("CCR_HELPER_STATUS") {
		case "sleep":
			time.Sleep(time.Minute)
		case "huge":
			fmt.Print(strings.Repeat("S", 1024*1024+1))
		case "malformed":
			fmt.Print("SECRET_RAW_STATUS")
			fmt.Fprint(os.Stderr, "SECRET_RAW_STDERR")
		case "unknown":
			fmt.Print(`{"unexpected":"SECRET_RAW_STATUS"}`)
		case "newlayout":
			fmt.Printf(`{"loggedIn":true,"authMethod":"claude.ai","configDirectory":%q,"newIdentityLayout":"SECRET_RAW_STATUS"}`, os.Getenv("CLAUDE_CONFIG_DIR"))
		case "oauth":
			fmt.Printf(`{"loggedIn":true,"authMethod":"oauth","configDirectory":%q}`, os.Getenv("CLAUDE_CONFIG_DIR"))
		case "null":
			fmt.Printf(`{"loggedIn":null,"configDirectory":%q}`, os.Getenv("CLAUDE_CONFIG_DIR"))
			os.Exit(1)
		default:
			fmt.Printf(`{"loggedIn":true,"authMethod":"claude.ai","email":"user@example.com","configDirectory":%q}`, os.Getenv("CLAUDE_CONFIG_DIR"))
		}
		os.Exit(0)
	}
	if os.Getenv("CCR_HELPER_RUN") == "sleep" {
		fmt.Print("started\n")
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	cwd, _ := os.Getwd()
	stdin, _ := io.ReadAll(os.Stdin)
	json.NewEncoder(os.Stdout).Encode(map[string]any{"args": args, "cwd": cwd, "profile": os.Getenv("CLAUDE_CONFIG_DIR"), "anthropic": os.Getenv("ANTHROPIC_CONFIG_DIR"), "input": string(stdin)})
	os.Exit(7)
}

func TestProfileEnvReplacesOnlyConfigDirectories(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "个人 账户")
	env := ProfileEnv(profile, []string{"KEEP=a=b", "CLAUDE_CONFIG_DIR=old", "ANTHROPIC_CONFIG_DIR=old2", "CLAUDE_CONFIG_DIR=duplicate"})
	got := map[string]string{}
	for _, line := range env {
		k, v, _ := strings.Cut(line, "=")
		if _, exists := got[k]; exists {
			t.Fatal("duplicate environment key")
		}
		got[k] = v
	}
	if got["KEEP"] != "a=b" || got["CLAUDE_CONFIG_DIR"] != profile || got["ANTHROPIC_CONFIG_DIR"] != filepath.Join(profile, ".anthropic") {
		t.Fatalf("unexpected isolated environment: %v", got)
	}
}

func TestCheckRedactsConflictsAndBlocksOverrides(t *testing.T) {
	profile, cwd := t.TempDir(), t.TempDir()
	secret := "NEVER_ECHO_ME"
	findings, err := Check(profile, cwd, []string{"--settings=" + secret}, []string{"ANTHROPIC_API_KEY=" + secret, "HTTPS_PROXY=" + secret})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(findings)
	if bytes.Contains(data, []byte(secret)) {
		t.Fatal("conflict leaked value")
	}
	blocking, warnings := 0, 0
	for _, f := range findings {
		if f.Blocking {
			blocking++
		} else {
			warnings++
		}
	}
	if blocking < 2 || warnings < 1 {
		t.Fatalf("missing findings: %+v", findings)
	}
	findings, err = Check("", cwd, nil, []string{"ANTHROPIC_PROFILE=" + secret})
	if err != nil || len(findings) == 0 {
		t.Fatalf("doctor preflight: %v %v", findings, err)
	}
}

func TestSettingsAncestorsAndHelpersAreCheckedWithoutExecution(t *testing.T) {
	profile, project := t.TempDir(), t.TempDir()
	cwd := filepath.Join(project, "nested")
	if err := os.Mkdir(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(project, ".claude"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".claude", "settings.json"), []byte(`{"apiKeyHelper":"SECRET_HELPER","env":{"ANTHROPIC_AUTH_TOKEN":"SECRET_TOKEN"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	findings, err := Check(profile, cwd, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(findings)
	if strings.Contains(string(data), "SECRET_") {
		t.Fatal("settings leaked values")
	}
	keys := map[string]bool{}
	for _, f := range findings {
		if f.Blocking {
			keys[f.Key] = true
		}
	}
	if !keys["apiKeyHelper"] || !keys["ANTHROPIC_AUTH_TOKEN"] {
		t.Fatalf("missing parent overrides: %+v", findings)
	}
	if err := os.WriteFile(filepath.Join(profile, "settings.local.json"), []byte(`{"broken"`), 0600); err != nil {
		t.Fatal(err)
	}
	findings, err = Check(profile, cwd, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	blocked := false
	for _, f := range findings {
		if f.Key == "settingsJSON" && f.Blocking {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("malformed settings did not block")
	}
}

func TestRunPreservesArgumentEnvironmentDirectoryAndExitCode(t *testing.T) {
	c := helperClient(t)
	profile, cwd := t.TempDir(), t.TempDir()
	var out, errout bytes.Buffer
	args := []string{"--print", "中文 prompt with spaces", "quote\"and\\slash", "--continue"}
	code, err := c.Run(context.Background(), profile, cwd, args, strings.NewReader("input data"), &out, &errout)
	if err != nil || code != 7 {
		t.Fatalf("run=%d %v stderr=%s", code, err, errout.String())
	}
	var result struct {
		Args                           []string
		Cwd, Profile, Anthropic, Input string
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	// macOS reports /private/var for temporary directories supplied as /var.
	// Compare directory identity without weakening argument/environment checks.
	wantDir, err := os.Stat(cwd)
	if err != nil {
		t.Fatal(err)
	}
	gotDir, err := os.Stat(result.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(result.Args) != fmt.Sprint(args) || !os.SameFile(gotDir, wantDir) || result.Profile != profile || result.Anthropic != filepath.Join(profile, ".anthropic") || result.Input != "input data" {
		t.Fatalf("bad child transport: %+v", result)
	}
}

func TestRunBlocksIdentitySubcommandsAndOldVersions(t *testing.T) {
	c := helperClient(t)
	profile, cwd := t.TempDir(), t.TempDir()
	for _, args := range [][]string{{"auth", "login", "--console"}, {"auth", "logout"}, {"setup-token"}, {"--bare"}, {"--setting-sources=user"}, {"--client-data-url=secret"}, {"--background"}, {"--bg=yes"}, {"agents"}, {"attach", "session-id"}, {"--debug", "auth", "logout"}, {"auth", "login", "--claudeai", "--console"}} {
		var out, errout bytes.Buffer
		if _, err := c.Run(context.Background(), profile, cwd, args, nil, &out, &errout); err == nil {
			t.Fatalf("accepted identity override %v", args)
		}
		if out.Len() != 0 {
			t.Fatal("blocked invocation ran child")
		}
	}
	t.Setenv("CCR_HELPER_VERSION", "2.1.267")
	if _, err := c.Version(context.Background()); err == nil {
		t.Fatal("accepted unsupported version")
	}
}

func TestRunCancellationStopsForegroundChild(t *testing.T) {
	c := helperClient(t)
	t.Setenv("CCR_HELPER_RUN", "sleep")
	profile, cwd := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	code, err := c.Run(ctx, profile, cwd, nil, nil, io.Discard, io.Discard)
	if err == nil || code != -1 || time.Since(start) > 3*time.Second {
		t.Fatalf("foreground cancellation: code=%d error=%v elapsed=%s", code, err, time.Since(start))
	}
}

func TestExactClaudeAILoginIsAllowed(t *testing.T) {
	c := helperClient(t)
	profile, cwd := t.TempDir(), t.TempDir()
	var out bytes.Buffer
	code, err := c.Run(context.Background(), profile, cwd, []string{"auth", "login", "--claudeai"}, nil, &out, io.Discard)
	if err != nil || code != 7 {
		t.Fatalf("official login delegation: %d %v", code, err)
	}
}

func TestStatusIsBoundedConservativeAndRedactsRawOutput(t *testing.T) {
	c := helperClient(t)
	profile, cwd := t.TempDir(), t.TempDir()
	status, err := c.Status(context.Background(), profile, cwd)
	if err != nil || !status.Known || !status.LoggedIn || status.Email != "user@example.com" || status.Version != "2.1.285" {
		t.Fatalf("status=%+v %v", status, err)
	}
	for _, mode := range []string{"malformed", "unknown", "huge", "newlayout", "oauth", "null"} {
		t.Setenv("CCR_HELPER_STATUS", mode)
		status, err = c.Status(context.Background(), profile, cwd)
		if status.Known {
			t.Fatalf("trusted unknown status mode %s", mode)
		}
		if err != nil && strings.Contains(err.Error(), "SECRET_") {
			t.Fatal("status error leaked raw data")
		}
	}
	t.Setenv("CCR_HELPER_STATUS", "sleep")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	status, err = c.Status(ctx, profile, cwd)
	if err == nil || status.Known || time.Since(start) > 3*time.Second {
		t.Fatalf("status cancellation failed: %+v %v", status, err)
	}
}

func TestProfileManagedSettingsAndUnsafeTLSBlockLaunch(t *testing.T) {
	profile, cwd := t.TempDir(), t.TempDir()
	findings, err := Check(profile, cwd, nil, []string{"NODE_TLS_REJECT_UNAUTHORIZED=0"})
	if err != nil {
		t.Fatal(err)
	}
	blocked := false
	for _, f := range findings {
		if f.Key == "NODE_TLS_REJECT_UNAUTHORIZED" && f.Blocking {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("disabled TLS verification did not block")
	}
	if err := os.WriteFile(filepath.Join(profile, "managed-settings.json"), []byte(`{"token":"NEVER_READ_MANAGED_VALUE"}`), 0600); err != nil {
		t.Fatal(err)
	}
	findings, err = Check(profile, cwd, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	blocked = false
	for _, f := range findings {
		if f.Source == "profile/managed-settings.json" && f.Blocking {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("profile managed settings did not block")
	}
	if err := os.Remove(filepath.Join(profile, "managed-settings.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(profile, "managed-settings.d"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "managed-settings.d", "policy.json"), []byte(`not parsed`), 0600); err != nil {
		t.Fatal(err)
	}
	findings, err = Check(profile, cwd, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	blocked = false
	for _, f := range findings {
		if f.Source == "profile/managed-settings.d" && f.Blocking {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("profile managed fragments did not block")
	}
}

func TestRuntimeInjectionEnvironmentIsBlocked(t *testing.T) {
	profile, cwd := t.TempDir(), t.TempDir()
	findings, err := Check(profile, cwd, nil, []string{"NODE_OPTIONS=--require SECRET_MODULE", "BUN_OPTIONS=SECRET_MODULE"})
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, f := range findings {
		if f.Blocking {
			keys[f.Key] = true
		}
	}
	if !keys["NODE_OPTIONS"] || !keys["BUN_OPTIONS"] {
		t.Fatal("runtime injection was not blocked")
	}
	data, _ := json.Marshal(findings)
	if strings.Contains(string(data), "SECRET_MODULE") {
		t.Fatal("runtime diagnostic leaked module value")
	}
}

func TestResolveRequiresAbsoluteOverride(t *testing.T) {
	t.Setenv("CCR_CLAUDE_BIN", "relative.exe")
	if _, err := Resolve(); err == nil {
		t.Fatal("accepted relative executable")
	}
	t.Setenv("CCR_CLAUDE_BIN", os.Args[0])
	c, err := Resolve()
	if err != nil || c.Executable != os.Args[0] {
		t.Fatalf("resolve=%+v %v", c, err)
	}
}

func TestResolveNPMNativeShimUsesOnlyFixedExecutable(t *testing.T) {
	dir := t.TempDir()
	native := filepath.Join(dir, "node_modules", "@anthropic-ai", "claude-code", "bin", "claude.exe")
	if err := os.MkdirAll(filepath.Dir(native), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(native, []byte("not executed"), 0700); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "claude.cmd")
	if err := os.WriteFile(shim, []byte(`@"%dp0%\node_modules\@anthropic-ai\claude-code\bin\claude.exe" %*`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := resolveNPMShim(shim)
	if err != nil || c.Executable != native || len(c.PrefixArgs) != 0 {
		t.Fatalf("native npm resolve: %+v %v", c, err)
	}
	if err := os.WriteFile(shim, []byte("unsupported wrapper"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveNPMShim(shim); err == nil {
		t.Fatal("accepted unidentified npm wrapper")
	}
}
