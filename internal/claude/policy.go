package claude

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func identityEnv(key string) bool {
	key = strings.ToUpper(key)
	switch key {
	case "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_PROFILE", "ANTHROPIC_FEDERATION_RULE_ID", "ANTHROPIC_ORGANIZATION_ID", "ANTHROPIC_IDENTITY_TOKEN", "ANTHROPIC_IDENTITY_TOKEN_FILE", "ANTHROPIC_CUSTOM_HEADERS", "CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CODE_SIMPLE", "CLAUDE_CODE_CLIENT_DATA_URL", "CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST":
		return true
	}
	return strings.HasPrefix(key, "CLAUDE_CODE_USE_") || strings.HasPrefix(key, "ANTHROPIC_") && strings.HasSuffix(key, "BASE_URL")
}
func transportEnv(key string) bool {
	key = strings.ToUpper(key)
	switch key {
	case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "NODE_EXTRA_CA_CERTS", "NODE_TLS_REJECT_UNAUTHORIZED", "SSL_CERT_FILE", "SSL_CERT_DIR", "CLAUDE_CODE_CLIENT_CERT", "CLAUDE_CODE_CLIENT_KEY", "CLAUDE_CODE_CLIENT_KEY_PASSPHRASE":
		return true
	}
	return false
}
func envFindings(source, profile string, env []string) []Finding {
	var findings []Finding
	for _, line := range env {
		key, value, ok := strings.Cut(line, "=")
		if !ok || value == "" {
			continue
		}
		key = strings.ToUpper(key)
		if key == "NODE_OPTIONS" || key == "BUN_OPTIONS" {
			findings = append(findings, Finding{Source: source, Key: key, Message: "runtime options can bypass verified CLI initialization", Blocking: true})
		} else if key == "NODE_TLS_REJECT_UNAUTHORIZED" && value == "0" {
			findings = append(findings, Finding{Source: source, Key: key, Message: "TLS certificate verification is disabled", Blocking: true})
		} else if identityEnv(key) {
			findings = append(findings, Finding{Source: source, Key: key, Message: "external authentication or provider configuration is unsupported", Blocking: true})
		} else if key == "CLAUDE_CONFIG_DIR" || key == "ANTHROPIC_CONFIG_DIR" {
			expected := profile
			if key == "ANTHROPIC_CONFIG_DIR" {
				expected = filepath.Join(profile, ".anthropic")
			}
			if profile == "" || !samePath(value, expected) {
				findings = append(findings, Finding{Source: source, Key: key, Message: "configuration directory conflicts with the selected account", Blocking: true})
			}
		} else if transportEnv(key) {
			findings = append(findings, Finding{Source: source, Key: key, Message: "transport configuration may affect connectivity or trust", Blocking: false})
		}
	}
	return findings
}

func Check(profile, cwd string, args, env []string) ([]Finding, error) {
	if profile != "" {
		if err := validateProfile(profile); err != nil {
			return nil, err
		}
	}
	if !filepath.IsAbs(cwd) {
		return nil, errors.New("working directory must be absolute")
	}
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		return nil, errors.New("working directory is unavailable")
	}
	findings := envFindings("environment", profile, env)
	blockedFlags := map[string]bool{"--bare": true, "--settings": true, "--setting-sources": true, "--client-data-url": true, "--sdk-url": true, "--remote-control": true, "--remote": true, "--teleport": true, "--bg": true, "--background": true, "--tmux": true, "--worktree": true, "-w": true}
	for _, arg := range args {
		if arg == "--" {
			break
		}
		key, _, _ := strings.Cut(arg, "=")
		if blockedFlags[key] {
			findings = append(findings, Finding{Source: "arguments", Key: key, Message: "argument bypasses account configuration checks", Blocking: true})
		}
	}
	allowedLogin := len(args) == 3 && args[0] == "auth" && args[1] == "login" && args[2] == "--claudeai"
	for _, command := range args {
		if command == "--" {
			break
		}
		reject := command == "auth" || command == "setup-token" || command == "login" || command == "logout" || command == "remote-control" || command == "web" || command == "desktop" || command == "agents" || command == "attach" || command == "daemon" || command == "background"
		if reject && !allowedLogin {
			findings = append(findings, Finding{Source: "arguments", Key: "identitySubcommand", Message: "only auth login --claudeai is supported for account authentication", Blocking: true})
			break
		}
	}
	if profile != "" {
		for _, name := range []string{"settings.json", "settings.local.json"} {
			findings = append(findings, inspectSettings(filepath.Join(profile, name), "profile/"+name, profile)...)
		}
		findings = append(findings, managedFile(filepath.Join(profile, "remote-settings.json"), "profile/remote-settings.json")...)
		findings = append(findings, managedFile(filepath.Join(profile, "managed-settings.json"), "profile/managed-settings.json")...)
		findings = append(findings, managedDirectory(filepath.Join(profile, "managed-settings.d"), "profile/managed-settings.d")...)
	}
	for dir := filepath.Clean(cwd); ; dir = filepath.Dir(dir) {
		for _, name := range []string{"settings.json", "settings.local.json"} {
			findings = append(findings, inspectSettings(filepath.Join(dir, ".claude", name), "project/.claude/"+name, profile)...)
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	findings = append(findings, managedFindings()...)
	return findings, nil
}

func inspectSettings(path, source, profile string) []Finding {
	failure := func(key, message string) []Finding {
		return []Finding{{Source: source, Key: key, Message: message, Blocking: true}}
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return failure("settingsJSON", "settings cannot be inspected")
	}
	if !info.Mode().IsRegular() {
		return failure("settingsJSON", "settings must be a regular file")
	}
	if info.Size() > maxOutput {
		return failure("settingsJSON", "settings exceed the size limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return failure("settingsJSON", "settings cannot be read")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxOutput+1))
	if err != nil || len(data) > maxOutput {
		return failure("settingsJSON", "settings cannot be read within the size limit")
	}
	doc, err := strictObject(data)
	if err != nil {
		return failure("settingsJSON", "settings contain invalid JSON")
	}
	var findings []Finding
	for _, key := range []string{"apiKeyHelper", "policyHelper", "forceLoginGatewayUrl"} {
		if value, ok := doc[key]; ok && string(value) != "null" && string(value) != `""` {
			findings = append(findings, Finding{Source: source, Key: key, Message: "authentication helper or gateway configuration is unsupported", Blocking: true})
		}
	}
	if value, ok := doc["forceLoginMethod"]; ok && string(value) != "null" {
		var method string
		if json.Unmarshal(value, &method) != nil || method != "claudeai" {
			findings = append(findings, Finding{Source: source, Key: "forceLoginMethod", Message: "login method does not allow isolated claude.ai authentication", Blocking: true})
		}
	}
	if value, ok := doc["env"]; ok && string(value) != "null" {
		var variables map[string]json.RawMessage
		if json.Unmarshal(value, &variables) != nil || variables == nil {
			return failure("env", "settings environment has an unsupported layout")
		}
		for key, value := range variables {
			var text string
			if json.Unmarshal(value, &text) != nil {
				findings = append(findings, Finding{Source: source, Key: "env", Message: "settings environment contains a non-string value", Blocking: true})
				continue
			}
			findings = append(findings, envFindings(source, profile, []string{key + "=" + text})...)
		}
	}
	return findings
}

func managedFile(path, source string) []Finding {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	message := "managed settings are present; this alpha cannot verify their authentication effects"
	if err != nil {
		message = "managed settings source cannot be inspected"
	}
	return []Finding{{Source: source, Key: "managedSettings", Message: message, Blocking: true}}
}
func managedDirectory(path, source string) []Finding {
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []Finding{{Source: source, Key: "managedSettings", Message: "managed settings directory cannot be inspected", Blocking: true}}
	}
	for _, entry := range entries {
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			return []Finding{{Source: source, Key: "managedSettings", Message: "managed settings fragments are present and unsupported", Blocking: true}}
		}
	}
	return nil
}
