// Package desktop exposes the same local account operations to a native UI.
package desktop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/harveyxiacn/cc-router/internal/claude"
	"github.com/harveyxiacn/cc-router/internal/cli"
	"github.com/harveyxiacn/cc-router/internal/localdata"
	"github.com/harveyxiacn/cc-router/internal/state"
	"github.com/harveyxiacn/cc-router/internal/usage"
)

type Service struct {
	Store        *state.Store
	CLIPath      string
	OpenTerminal func(executable, project string, args []string) error
}
type AccountView struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Label      string          `json:"label"`
	IsDefault  bool            `json:"isDefault"`
	Active     bool            `json:"active"`
	Usage      *usage.Snapshot `json:"usage"`
	UsageError string          `json:"usageError,omitempty"`
}
type Snapshot struct {
	Accounts     []AccountView `json:"accounts"`
	Project      string        `json:"project"`
	BoundAccount string        `json:"boundAccount"`
	DataDir      string        `json:"dataDir"`
}
type Identity struct {
	Known      bool   `json:"known"`
	LoggedIn   bool   `json:"loggedIn"`
	Email      string `json:"email"`
	AuthMethod string `json:"authMethod"`
	Version    string `json:"version"`
}
type Finding struct {
	Source   string `json:"source"`
	Key      string `json:"key"`
	Message  string `json:"message"`
	Blocking bool   `json:"blocking"`
}
type Diagnosis struct {
	Version  string    `json:"version"`
	Findings []Finding `json:"findings"`
	Error    string    `json:"error"`
}
type Handoff struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Digest  string `json:"digest"`
}

func New(cliPath string) (*Service, error) {
	s, err := state.Open("")
	if err != nil {
		return nil, err
	}
	return &Service{Store: s, CLIPath: cliPath}, nil
}
func (s *Service) GetSnapshot(project string) (Snapshot, error) {
	result := Snapshot{Accounts: []AccountView{}, DataDir: s.Store.Root}
	if project != "" {
		p, err := state.CanonicalProject(project)
		if err != nil {
			return result, err
		}
		result.Project = p
	}
	r, err := s.Store.Load()
	if err != nil {
		return result, err
	}
	for _, a := range r.Accounts {
		v := AccountView{ID: a.ID, Name: a.Name, Label: a.Label, IsDefault: r.DefaultID == a.ID}
		unlock, err := s.Store.LockAccount(a.ID)
		if errors.Is(err, state.ErrBusy) {
			v.Active = true
		} else if err != nil {
			return result, err
		} else {
			unlock()
		}
		v.Usage, err = usage.Load(s.Store, a.ID)
		if err != nil {
			v.Usage = nil
			v.UsageError = "Local usage observation is unavailable"
		}
		if r.Bindings[result.Project] == a.ID {
			result.BoundAccount = a.Name
		}
		result.Accounts = append(result.Accounts, v)
	}
	return result, nil
}
func (s *Service) mutate(project string, args ...string) error {
	if project == "" {
		var err error
		project, err = os.UserHomeDir()
		if err != nil {
			return err
		}
	}
	app := cli.App{Store: s.Store, Dir: project, Out: io.Discard, Err: io.Discard}
	_, err := app.Execute(args)
	return err
}
func (s *Service) CreateAccount(name, label string) error {
	return s.mutate("", "account", "add", name, "--label", label)
}
func (s *Service) RenameAccount(name, newName, label string) error {
	return s.mutate("", "account", "rename", name, newName, "--label", label)
}
func (s *Service) RemoveAccount(name string) error { return s.mutate("", "account", "remove", name) }
func (s *Service) SetDefault(name string) error    { return s.mutate("", "use", name) }
func (s *Service) ExportMetadata() (string, error) {
	var b strings.Builder
	if err := s.Store.Export(&b); err != nil {
		return "", err
	}
	return b.String(), nil
}
func (s *Service) ImportMetadata(data string) error {
	if len(data) > 1024*1024 {
		return errors.New("metadata exceeds 1 MiB")
	}
	return s.Store.Import(strings.NewReader(data))
}
func (s *Service) Bind(project, name string) error {
	if project == "" {
		return errors.New("select a project first")
	}
	return s.mutate(project, "bind", name)
}
func (s *Service) Unbind(project string) error {
	if project == "" {
		return errors.New("select a project first")
	}
	return s.mutate(project, "unbind")
}
func (s *Service) account(name string) (state.Account, error) {
	r, err := s.Store.Load()
	if err != nil {
		return state.Account{}, err
	}
	return r.Find(name)
}

func (s *Service) CheckAccount(name, project string) (Identity, error) {
	var result Identity
	a, err := s.account(name)
	if err != nil {
		return result, err
	}
	if project == "" {
		return result, errors.New("select a project first")
	}
	client, err := claude.Resolve()
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	status, err := client.Status(ctx, s.Store.ProfileDir(a), project)
	if err != nil {
		return result, err
	}
	return Identity{status.Known, status.LoggedIn, status.Email, status.AuthMethod, status.Version}, nil
}
func (s *Service) Diagnose(name, project string) (Diagnosis, error) {
	result := Diagnosis{Findings: []Finding{}}
	profile := ""
	if name != "" {
		a, err := s.account(name)
		if err != nil {
			return result, err
		}
		profile = s.Store.ProfileDir(a)
	}
	if project == "" {
		var err error
		project, err = os.UserHomeDir()
		if err != nil {
			return result, err
		}
	}
	client, err := claude.Resolve()
	if err != nil {
		result.Error = err.Error()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result.Version, err = client.Version(ctx)
		if err != nil {
			result.Error = err.Error()
		}
	}
	findings, err := claude.Check(profile, project, nil, os.Environ())
	if err != nil {
		return result, err
	}
	for _, f := range findings {
		result.Findings = append(result.Findings, Finding{f.Source, f.Key, f.Message, f.Blocking})
	}
	return result, nil
}

func (s *Service) CreateHandoff(project string) (Handoff, error) {
	if project == "" {
		return Handoff{}, errors.New("select a project first")
	}
	unlock, err := s.Store.LockProject(project)
	if err != nil {
		return Handoff{}, err
	}
	defer unlock()
	if _, _, err := cli.WriteHandoff(project); err != nil {
		return Handoff{}, err
	}
	return s.ReadHandoff(project)
}
func handoffRoot(project string) (*os.Root, error) {
	if project == "" {
		return nil, errors.New("select a project first")
	}
	root, err := os.OpenRoot(project)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	st, err := root.Lstat(".cc-router")
	if err != nil {
		return nil, errors.New("create a handoff template first")
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("handoff directory must be local")
	}
	return root.OpenRoot(".cc-router")
}
func handoffValue(project string, data []byte) Handoff {
	sum := sha256.Sum256(data)
	return Handoff{filepath.Join(project, ".cc-router", "handoff.md"), string(data), hex.EncodeToString(sum[:])}
}
func (s *Service) ReadHandoff(project string) (Handoff, error) {
	root, err := handoffRoot(project)
	if err != nil {
		return Handoff{}, err
	}
	defer root.Close()
	b, err := localdata.Read(root, "handoff.md", 256*1024)
	if err != nil {
		return Handoff{}, errors.New("handoff unavailable, linked, or exceeds 256 KiB")
	}
	if !utf8.Valid(b) {
		return Handoff{}, errors.New("handoff must be UTF-8")
	}
	return handoffValue(project, b), nil
}
func (s *Service) SaveHandoff(project, content, expectedDigest string) (Handoff, error) {
	if len(content) > 256*1024 || !utf8.ValidString(content) {
		return Handoff{}, errors.New("handoff must be UTF-8 and at most 256 KiB")
	}
	unlock, err := s.Store.LockProject(project)
	if err != nil {
		return Handoff{}, err
	}
	defer unlock()
	root, err := handoffRoot(project)
	if err != nil {
		return Handoff{}, err
	}
	defer root.Close()
	b, err := localdata.Read(root, "handoff.md", 256*1024)
	if err != nil {
		return Handoff{}, err
	}
	if expectedDigest == "" || handoffValue(project, b).Digest != expectedDigest {
		return Handoff{}, errors.New("handoff changed on disk; reload and merge your draft before saving")
	}
	if err := localdata.Write(root, "handoff.md", []byte(content)); err != nil {
		return Handoff{}, err
	}
	return handoffValue(project, []byte(content)), nil
}

func (s *Service) Launch(name, project, mode string, reviewed bool) error {
	if mode != "run" && mode != "login" && mode != "switch" {
		return errors.New("unsupported launch mode")
	}
	a, err := s.account(name)
	if err != nil {
		return err
	}
	if project == "" {
		if mode != "login" {
			return errors.New("select a project first")
		}
		project, err = os.UserHomeDir()
		if err != nil {
			return err
		}
	}
	project, err = state.CanonicalProject(project)
	if err != nil {
		return err
	}
	if mode == "switch" {
		if !reviewed {
			return errors.New("review and save the handoff before switching")
		}
		if _, err := s.ReadHandoff(project); err != nil {
			return err
		}
	}
	if err := validCLI(s.CLIPath); err != nil {
		return err
	}
	var officialArgs []string
	if mode == "login" {
		officialArgs = []string{"auth", "login", "--claudeai"}
	}
	findings, err := claude.Check(s.Store.ProfileDir(a), project, officialArgs, os.Environ())
	if err != nil {
		return err
	}
	for _, f := range findings {
		if f.Blocking {
			return fmt.Errorf("launch blocked: %s / %s: %s", f.Source, f.Key, f.Message)
		}
	}
	// Probe locks only. The launched CLI acquires and holds them for its entire run;
	// keeping them here until after terminal startup would race the child itself.
	unlock, err := s.Store.LockAccount(a.ID)
	if err != nil {
		return err
	}
	unlock()
	if mode != "login" {
		unlock, err := s.Store.LockProject(project)
		if err != nil {
			return err
		}
		unlock()
	}
	// Terminal servers may have a different inherited environment from the GUI.
	// Pin the registry explicitly; the CLI also propagates it to statusline callbacks.
	args := []string{"--data-dir", s.Store.Root, mode, a.Name}
	if mode == "switch" {
		args = append(args, "--handoff-reviewed")
	}
	if s.OpenTerminal != nil {
		return s.OpenTerminal(s.CLIPath, project, args)
	}
	return openTerminal(s.CLIPath, project, args, s.Store.Root)
}
func validCLI(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("CC Router CLI path must be absolute")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("CC Router CLI missing; place cc-router next to the desktop app or set CC_ROUTER_CLI")
	}
	if runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(path), ".exe") {
		return errors.New("CC Router CLI must be a native executable")
	}
	return nil
}

func (s *Service) InstallUsage(name string, prepare, switchAt float64) error {
	if math.IsNaN(prepare) || math.IsNaN(switchAt) || math.IsInf(prepare, 0) || math.IsInf(switchAt, 0) || prepare < 1 || switchAt > 100 || prepare > switchAt {
		return errors.New("thresholds must satisfy 1 <= prepare <= switch <= 100")
	}
	a, err := s.account(name)
	if err != nil {
		return err
	}
	if err := validCLI(s.CLIPath); err != nil {
		return err
	}
	unlock, err := s.Store.LockAccount(a.ID)
	if err != nil {
		return err
	}
	defer unlock()
	current, err := s.account(name)
	if err != nil || current.ID != a.ID {
		return errors.New("account changed; refresh and try again")
	}
	root, err := os.OpenRoot(s.Store.ProfileDir(a))
	if err != nil {
		return err
	}
	defer root.Close()
	settings := map[string]json.RawMessage{}
	b, err := localdata.Read(root, "settings.json", 1024*1024)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		// Decode validates object roots, duplicates, and trailing data without retaining secrets.
		if _, err := usage.Decode(b, time.Now()); err != nil {
			return errors.New("settings JSON cannot be safely updated")
		}
		if err := json.Unmarshal(b, &settings); err != nil || settings == nil {
			return errors.New("settings JSON must be an object")
		}
	}
	if raw, exists := settings["statusLine"]; exists && string(raw) != "null" {
		return errors.New("a statusline is already configured; preserve or remove it manually before installing CC Router's statusline")
	}
	command, err := statuslineCommand(s.CLIPath, a.ID, prepare, switchAt)
	if err != nil {
		return err
	}
	settings["statusLine"], _ = json.Marshal(map[string]string{"type": "command", "command": command})
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > 1024*1024 {
		return errors.New("updated settings exceed size limit")
	}
	return localdata.Write(root, "settings.json", append(data, '\n'))
}
func statuslineCommand(executable, id string, prepare, switchAt float64) (string, error) {
	if strings.ContainsAny(executable, "\r\n\x00") {
		return "", errors.New("unsupported CLI path characters")
	}
	quoted := posixQuote(executable)
	if runtime.GOOS == "windows" {
		if strings.ContainsAny(executable, "\"%$`!&|<>") {
			return "", errors.New("CLI path contains characters unsupported by statusline shells")
		}
		quoted = "\"" + filepath.ToSlash(executable) + "\""
	}
	return fmt.Sprintf("%s usage statusline --account-id %s --prepare-at %s --switch-at %s", quoted, id, strconv.FormatFloat(prepare, 'f', -1, 64), strconv.FormatFloat(switchAt, 'f', -1, 64)), nil
}
func posixQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
