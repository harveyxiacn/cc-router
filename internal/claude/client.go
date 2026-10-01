// Package claude delegates credentials and interactive sessions to the official CLI.
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxOutput = 1024 * 1024
const probeTimeout = 10 * time.Second

type Client struct {
	Executable string
	PrefixArgs []string
}
type Status struct {
	Known                      bool
	LoggedIn                   bool
	Email, AuthMethod, Version string
}
type Finding struct {
	Source, Key, Message string
	Blocking             bool
}

func Resolve() (Client, error) {
	if override := os.Getenv("CCR_CLAUDE_BIN"); override != "" {
		if !filepath.IsAbs(override) {
			return Client{}, errors.New("CCR_CLAUDE_BIN must name an absolute executable")
		}
		if err := nativeExecutable(override); err != nil {
			return Client{}, err
		}
		return Client{Executable: override}, nil
	}
	if runtime.GOOS == "windows" {
		if path, err := exec.LookPath("claude.exe"); err == nil {
			if err := nativeExecutable(path); err == nil {
				return Client{Executable: path}, nil
			}
		}
		for _, name := range []string{"claude.cmd", "claude.ps1"} {
			if path, err := exec.LookPath(name); err == nil {
				return resolveNPMShim(path)
			}
		}
		return Client{}, errors.New("official Claude executable not found; set CCR_CLAUDE_BIN to its absolute path")
	}
	path, err := exec.LookPath("claude")
	if err != nil {
		return Client{}, errors.New("official Claude executable not found")
	}
	if err := nativeExecutable(path); err != nil {
		return Client{}, err
	}
	return Client{Executable: path}, nil
}
func nativeExecutable(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("Claude executable must be absolute")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("Claude executable is unavailable")
	}
	if runtime.GOOS == "windows" {
		if !strings.EqualFold(filepath.Ext(path), ".exe") {
			return errors.New("shell wrappers are unsupported; use the native Claude executable")
		}
	} else if info.Mode().Perm()&0111 == 0 {
		return errors.New("Claude binary is not executable")
	}
	return nil
}
func resolveNPMShim(path string) (Client, error) {
	f, err := os.Open(path)
	if err != nil {
		return Client{}, errors.New("cannot inspect npm Claude wrapper")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64*1024+1))
	if err != nil || len(data) > 64*1024 {
		return Client{}, errors.New("npm Claude wrapper is unsupported")
	}
	text := strings.ReplaceAll(string(data), "\\", "/")
	dir := filepath.Dir(path)
	if strings.Contains(text, "node_modules/@anthropic-ai/claude-code/bin/claude.exe") {
		native := filepath.Join(dir, "node_modules", "@anthropic-ai", "claude-code", "bin", "claude.exe")
		if err := nativeExecutable(native); err != nil {
			return Client{}, errors.New("official npm Claude native executable is unavailable")
		}
		return Client{Executable: native}, nil
	}
	if !strings.Contains(text, "node_modules/@anthropic-ai/claude-code/cli.js") {
		return Client{}, errors.New("npm wrapper does not identify the official Claude entry point")
	}
	entry := filepath.Join(dir, "node_modules", "@anthropic-ai", "claude-code", "cli.js")
	if info, err := os.Stat(entry); err != nil || !info.Mode().IsRegular() {
		return Client{}, errors.New("official npm Claude entry point is unavailable")
	}
	node := filepath.Join(dir, "node.exe")
	if err := nativeExecutable(node); err != nil {
		var lookupErr error
		node, lookupErr = exec.LookPath("node.exe")
		if lookupErr != nil {
			return Client{}, errors.New("node executable for npm Claude is unavailable")
		}
		if err := nativeExecutable(node); err != nil {
			return Client{}, err
		}
	}
	return Client{Executable: node, PrefixArgs: []string{entry}}, nil
}

func ProfileEnv(profile string, env []string) []string {
	result := make([]string, 0, len(env)+2)
	for _, line := range env {
		key, _, ok := strings.Cut(line, "=")
		if ok && (strings.EqualFold(key, "CLAUDE_CONFIG_DIR") || strings.EqualFold(key, "ANTHROPIC_CONFIG_DIR")) {
			continue
		}
		result = append(result, line)
	}
	return append(result, "CLAUDE_CONFIG_DIR="+profile, "ANTHROPIC_CONFIG_DIR="+filepath.Join(profile, ".anthropic"))
}
func (c Client) command(ctx context.Context, args []string) (*exec.Cmd, error) {
	if err := nativeExecutable(c.Executable); err != nil {
		return nil, err
	}
	argv := append(append([]string{}, c.PrefixArgs...), args...)
	return exec.CommandContext(ctx, c.Executable, argv...), nil
}

type cappedWriter struct {
	buffer   bytes.Buffer
	exceeded bool
}

func (w *cappedWriter) Write(data []byte) (int, error) {
	if w.buffer.Len()+len(data) > maxOutput {
		w.exceeded = true
		return 0, errors.New("CLI probe output exceeds limit")
	}
	return w.buffer.Write(data)
}
func (c Client) probe(ctx context.Context, profile, cwd string, args []string) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd, err := c.command(ctx, args)
	if err != nil {
		return nil, -1, err
	}
	cmd.Dir = cwd
	if profile != "" {
		cmd.Env = ProfileEnv(profile, os.Environ())
	}
	var out, errout cappedWriter
	cmd.Stdout = &out
	cmd.Stderr = &errout
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	if ctx.Err() != nil {
		return nil, -1, errors.New("Claude CLI probe timed out or was cancelled")
	}
	if out.exceeded || errout.exceeded {
		return nil, -1, errors.New("Claude CLI probe output exceeds limit")
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return out.buffer.Bytes(), exit.ExitCode(), nil
		}
		return nil, -1, errors.New("Claude CLI probe failed")
	}
	return out.buffer.Bytes(), 0, nil
}

var versionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:\s+\(Claude Code\))?$`)

func (c Client) Version(ctx context.Context) (string, error) {
	output, code, err := c.probe(ctx, "", "", []string{"--version"})
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", errors.New("Claude version probe failed")
	}
	match := versionPattern.FindStringSubmatch(strings.TrimSpace(string(output)))
	if match == nil {
		return "", errors.New("unsupported Claude version output")
	}
	values := make([]int, 3)
	for i := range values {
		value, err := strconv.Atoi(match[i+1])
		if err != nil {
			return "", errors.New("invalid Claude version")
		}
		values[i] = value
	}
	if values[0] < 2 || values[0] == 2 && (values[1] < 1 || values[1] == 1 && values[2] < 268) {
		return "", errors.New("Claude Code 2.1.268 or later is required")
	}
	return strings.Join(match[1:4], "."), nil
}

func reportFindings(findings []Finding, w io.Writer) bool {
	blocked := false
	for _, f := range findings {
		level := "warning"
		if f.Blocking {
			level = "blocked"
			blocked = true
		}
		fmt.Fprintf(w, "%s: %s [%s: %s]\n", level, f.Message, f.Source, f.Key)
	}
	return blocked
}
func (c Client) Run(ctx context.Context, profile, cwd string, args []string, in io.Reader, out, errout io.Writer) (int, error) {
	if err := validateProfile(profile); err != nil {
		return -1, err
	}
	if errout == nil {
		errout = io.Discard
	}
	findings, err := Check(profile, cwd, args, os.Environ())
	if err != nil {
		return -1, err
	}
	if reportFindings(findings, errout) {
		return -1, errors.New("Claude launch blocked by identity or settings conflicts")
	}
	if _, err := c.Version(ctx); err != nil {
		return -1, err
	}
	cmd, err := c.command(ctx, args)
	if err != nil {
		return -1, err
	}
	cmd.Dir = cwd
	cmd.Env = ProfileEnv(profile, os.Environ())
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = errout
	err = runForeground(cmd)
	if ctx.Err() != nil {
		return -1, ctx.Err()
	}
	if err == nil {
		return 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code := exit.ExitCode()
		if code < 0 {
			code = interruptedExitCode(exit)
		}
		return code, nil
	}
	return -1, errors.New("could not launch official Claude CLI")
}

func (c Client) Status(ctx context.Context, profile, cwd string) (Status, error) {
	var result Status
	if err := validateProfile(profile); err != nil {
		return result, err
	}
	findings, err := Check(profile, cwd, nil, os.Environ())
	if err != nil {
		return result, err
	}
	for _, f := range findings {
		if f.Blocking {
			return result, errors.New("auth status blocked by identity or settings conflicts")
		}
	}
	result.Version, err = c.Version(ctx)
	if err != nil {
		return result, err
	}
	output, code, err := c.probe(ctx, profile, cwd, []string{"auth", "status", "--json"})
	if err != nil {
		return result, err
	}
	if code != 0 && code != 1 {
		return result, errors.New("auth status is unavailable")
	}
	doc, err := strictObject(output)
	if err != nil {
		return result, errors.New("auth status returned an unsupported JSON layout")
	}
	knownFields := map[string]bool{"loggedIn": true, "configDirectory": true, "authMethod": true, "email": true, "apiProvider": true, "apiKeySource": true, "tokenSource": true, "organizationId": true, "organizationName": true, "subscriptionType": true}
	for key := range doc {
		if !knownFields[key] {
			return result, errors.New("auth status returned an unknown field layout")
		}
	}
	var loggedIn bool
	if raw, ok := doc["loggedIn"]; !ok || (string(raw) != "true" && string(raw) != "false") || json.Unmarshal(raw, &loggedIn) != nil {
		return result, errors.New("auth status does not contain a supported loggedIn field")
	}
	var dir string
	if raw, ok := doc["configDirectory"]; !ok || json.Unmarshal(raw, &dir) != nil || !samePath(dir, profile) {
		return result, errors.New("auth status configuration directory could not be verified")
	}
	var method, email string
	if raw, ok := doc["authMethod"]; ok && string(raw) != "null" {
		if json.Unmarshal(raw, &method) != nil {
			return result, errors.New("unsupported auth method layout")
		}
	}
	if raw, ok := doc["email"]; ok && string(raw) != "null" {
		if json.Unmarshal(raw, &email) != nil {
			return result, errors.New("unsupported auth email layout")
		}
	}
	if !safeDisplay(email, 320) || !safeDisplay(method, 64) {
		return result, errors.New("auth status contains unsafe display text")
	}
	if loggedIn && method != "claude.ai" && method != "claudeai" {
		return result, errors.New("auth status does not confirm a claude.ai login")
	}
	if loggedIn && code != 0 || !loggedIn && code != 1 {
		return result, errors.New("auth status exit code does not match loggedIn")
	}
	result.Known, result.LoggedIn, result.Email, result.AuthMethod = true, loggedIn, email, method
	return result, nil
}
func safeDisplay(text string, limit int) bool {
	if len(text) > limit || !utf8.ValidString(text) {
		return false
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func samePath(a, b string) bool {
	if !filepath.IsAbs(a) || !filepath.IsAbs(b) {
		return false
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
func validateProfile(profile string) error {
	if !filepath.IsAbs(profile) {
		return errors.New("profile must be an absolute directory")
	}
	info, err := os.Lstat(profile)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("profile must be an existing real directory")
	}
	if info, err := os.Lstat(filepath.Join(profile, ".anthropic")); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Anthropic profile directory must be a real directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot inspect Anthropic profile directory")
	}
	return nil
}

// Reject duplicates, trailing JSON, non-object roots, and invalid UTF-8 without
// including raw content in diagnostics. Only settings metadata is read.
func strictObject(data []byte) (map[string]json.RawMessage, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("invalid JSON encoding")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("JSON must be an object")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueJSON(dec); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("trailing JSON")
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, errors.New("invalid JSON")
	}
	return result, nil
}
func uniqueJSON(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return errors.New("invalid JSON")
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return errors.New("invalid JSON")
			}
			text, ok := key.(string)
			if !ok || seen[text] {
				return errors.New("duplicate JSON key")
			}
			seen[text] = true
			if err := uniqueJSON(dec); err != nil {
				return err
			}
		}
	case '[':
		for dec.More() {
			if err := uniqueJSON(dec); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = dec.Token()
	if err != nil {
		return errors.New("invalid JSON")
	}
	return nil
}
