package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/harveyxiacn/cc-router/internal/state"
)

var Version = "0.2.0-beta.1"

const Help = `CC Router — local Claude Code profile launcher

Usage:
  cc-router init
  cc-router account add NAME [--label LABEL]
  cc-router account list
  cc-router account rename NAME NEW_NAME [--label LABEL]
  cc-router account remove NAME
  cc-router login NAME
  cc-router use NAME
  cc-router bind NAME
  cc-router unbind
  cc-router run [NAME] [-- CLAUDE_ARGS...]
  cc-router status [NAME]
  cc-router doctor [NAME]
  cc-router handoff
  cc-router switch [NAME] [--handoff-reviewed]
  cc-router export
  cc-router import FILE|-
  cc-router usage statusline [--prepare-at 90] [--switch-at 95]
  cc-router usage config
  cc-router usage show|clear NAME
  cc-router usage record NAME [--five-hour PERCENT --five-reset RFC3339] [--seven-day PERCENT --seven-reset RFC3339] [--limited-until RFC3339]
  cc-router backup create|list
  cc-router backup show ID
  cc-router backup restore ID --reviewed-digest SHA256
  cc-router version

Selection: explicit name > current directory binding > global default > prompt.
Use cc-router as the primary command. The optional legacy alias may conflict with other tools.
Removed accounts retain their official configuration. No credentials are exported.
`

// App coordinates commands; the official CLI adapter is injected at the executable boundary.
type App struct {
	Store             *state.Store
	Dir               string
	In                io.Reader
	Out, Err          io.Writer
	Interactive       bool
	ExpectedAccountID string
	Launch            func(profile, dir string, args []string) (int, error)
	Inspect           func(profile, dir string) (string, error)
	Diagnose          func(profile, dir string) (string, error)
}

func (a *App) Execute(args []string) (int, error) {
	if a.Out == nil {
		a.Out = io.Discard
	}
	if a.Err == nil {
		a.Err = io.Discard
	}
	if a.In == nil {
		a.In = strings.NewReader("")
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(a.Out, Help)
		return 0, nil
	}
	cmd, rest := args[0], args[1:]
	if cmd == "version" || cmd == "--version" {
		if len(rest) > 0 {
			return 0, errors.New("version takes no arguments")
		}
		fmt.Fprintln(a.Out, Version)
		return 0, nil
	}
	if cmd == "usage" {
		return a.usage(rest)
	}
	if a.Store == nil {
		return 0, errors.New("account store is unavailable")
	}
	switch cmd {
	case "backup":
		return a.backup(rest)
	case "init":
		if len(rest) != 0 {
			return 0, errors.New("usage: cc-router init")
		}
		if err := a.Store.Update(func(*state.Registry) error { return nil }); err != nil {
			return 0, err
		}
		fmt.Fprintln(a.Out, "Account registry initialized. Add an account with: cc-router account add NAME")
	case "account":
		return a.account(rest)
	case "use", "bind":
		if len(rest) != 1 {
			return 0, fmt.Errorf("usage: cc-router %s NAME", cmd)
		}
		project, err := state.CanonicalProject(a.Dir)
		if err != nil {
			return 0, err
		}
		err = a.Store.Update(func(r *state.Registry) error {
			if cmd == "use" {
				return r.SetDefault(rest[0])
			}
			return r.Bind(project, rest[0])
		})
		if err != nil {
			return 0, err
		}
		fmt.Fprintf(a.Out, "%s: %s (future launches only)\n", cmd, rest[0])
	case "unbind":
		if len(rest) != 0 {
			return 0, errors.New("usage: cc-router unbind")
		}
		p, err := state.CanonicalProject(a.Dir)
		if err != nil {
			return 0, err
		}
		return 0, a.Store.Update(func(r *state.Registry) error { return r.Unbind(p) })
	case "run", "login", "switch":
		return a.launch(cmd, rest)
	case "status", "doctor":
		if len(rest) > 1 {
			return 0, fmt.Errorf("usage: cc-router %s [NAME]", cmd)
		}
		name := ""
		if len(rest) == 1 {
			name = rest[0]
		}
		selected, err := a.selectAccount(name, false)
		if err != nil {
			if cmd == "doctor" && errors.Is(err, state.ErrNoSelection) {
				fmt.Fprintln(a.Out, "No account selected; profile settings were not inspected. Add an account, then run doctor NAME.")
				if a.Diagnose == nil {
					return 0, errors.New("official CLI adapter unavailable")
				}
				text, err := a.Diagnose("", a.Dir)
				if text != "" {
					fmt.Fprintln(a.Out, text)
				}
				return 0, err
			}
			return 0, err
		}
		fn := a.Inspect
		if cmd == "doctor" {
			fn = a.Diagnose
		}
		if fn == nil {
			return 0, errors.New("official CLI adapter unavailable")
		}
		text, err := fn(a.Store.ProfileDir(selected), a.Dir)
		fmt.Fprintf(a.Out, "Account label: %s (%s); label is not verified identity.\n", selected.Label, selected.Name)
		if text != "" {
			fmt.Fprintln(a.Out, text)
		}
		return 0, err
	case "handoff":
		if len(rest) != 0 {
			return 0, errors.New("usage: cc-router handoff")
		}
		unlock, err := a.Store.LockProject(a.Dir)
		if err != nil {
			return 0, err
		}
		defer unlock()
		path, created, err := WriteHandoff(a.Dir)
		if err != nil {
			return 0, err
		}
		if created {
			fmt.Fprintf(a.Out, "Created %s. Fill in the goal, changes, checks and next steps.\n", path)
		} else {
			fmt.Fprintf(a.Out, "Kept existing %s. Review and update it before switching.\n", path)
		}
	case "export":
		if len(rest) != 0 {
			return 0, errors.New("usage: cc-router export (JSON on stdout)")
		}
		return 0, a.Store.Export(a.Out)
	case "import":
		if len(rest) != 1 {
			return 0, errors.New("usage: cc-router import FILE|-")
		}
		input := a.In
		if rest[0] != "-" {
			file, err := os.Open(rest[0])
			if err != nil {
				return 0, err
			}
			defer file.Close()
			input = file
		}
		if err := a.Store.Import(input); err != nil {
			return 0, err
		}
		fmt.Fprintln(a.Out, "Imported account metadata. Authenticate each new account on this device.")
	default:
		return 0, fmt.Errorf("unknown command %q; run cc-router help", cmd)
	}
	return 0, nil
}

func labelArgument(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	if len(args) == 2 && args[0] == "--label" {
		return args[1], nil
	}
	return "", errors.New("expected --label LABEL")
}

func (a *App) account(args []string) (int, error) {
	if len(args) == 0 {
		return 0, errors.New("usage: cc-router account add|list|rename|remove")
	}
	switch args[0] {
	case "add":
		if len(args) < 2 {
			return 0, errors.New("usage: cc-router account add NAME [--label LABEL]")
		}
		label, err := labelArgument(args[2:])
		if err != nil {
			return 0, err
		}
		if label == "" {
			label = args[1]
		}
		err = a.Store.Update(func(r *state.Registry) error { _, err := r.Add(args[1], label); return err })
		if err != nil {
			return 0, err
		}
		fmt.Fprintf(a.Out, "Added %s. Authenticate with: cc-router login %s\n", args[1], args[1])
	case "list":
		if len(args) != 1 {
			return 0, errors.New("usage: cc-router account list")
		}
		r, err := a.Store.Load()
		if err != nil {
			return 0, err
		}
		if len(r.Accounts) == 0 {
			fmt.Fprintln(a.Out, "No accounts. Run cc-router account add NAME.")
		}
		for _, account := range r.Accounts {
			mark := " "
			if r.DefaultID == account.ID {
				mark = "*"
			}
			fmt.Fprintf(a.Out, "%s %s\t%s\n", mark, account.Name, account.Label)
		}
	case "rename", "remove":
		if len(args) < 2 {
			return 0, errors.New("account name is required")
		}
		label := ""
		if args[0] == "remove" && len(args) != 2 {
			return 0, errors.New("usage: cc-router account remove NAME")
		}
		if args[0] == "rename" {
			if len(args) < 3 {
				return 0, errors.New("usage: cc-router account rename NAME NEW_NAME [--label LABEL]")
			}
			var err error
			label, err = labelArgument(args[3:])
			if err != nil {
				return 0, err
			}
		}
		r, err := a.Store.Load()
		if err != nil {
			return 0, err
		}
		account, err := r.Find(args[1])
		if err != nil {
			return 0, err
		}
		if args[0] == "rename" && len(args) == 3 {
			label = account.Label
		}
		unlock, err := a.Store.LockAccount(account.ID)
		if err != nil {
			return 0, err
		}
		defer unlock()
		err = a.Store.Update(func(r *state.Registry) error {
			current, err := r.Find(args[1])
			if err != nil {
				return err
			}
			if current.ID != account.ID {
				return errors.New("account changed concurrently; retry")
			}
			if args[0] == "remove" {
				return r.Remove(args[1])
			}
			return r.Rename(args[1], args[2], label)
		})
		if err != nil {
			return 0, err
		}
		fmt.Fprintln(a.Out, "Account registry updated; official profile files preserved.")
	default:
		return 0, errors.New("usage: cc-router account add|list|rename|remove")
	}
	return 0, nil
}

func (a *App) selectAccount(name string, prompt bool) (state.Account, error) {
	r, err := a.Store.Load()
	if err != nil {
		return state.Account{}, err
	}
	project, err := state.CanonicalProject(a.Dir)
	if err != nil {
		return state.Account{}, err
	}
	account, err := r.Select(name, project)
	if !errors.Is(err, state.ErrNoSelection) || !prompt || !a.Interactive {
		return account, err
	}
	if len(r.Accounts) == 0 {
		return state.Account{}, errors.New("no accounts; run cc-router account add NAME")
	}
	for i, v := range r.Accounts {
		fmt.Fprintf(a.Err, "%d) %s — %s\n", i+1, v.Name, v.Label)
	}
	fmt.Fprint(a.Err, "Choose account number: ")
	// Read one byte at a time so buffered menu input never swallows Claude's stdin.
	var line strings.Builder
	buf := make([]byte, 1)
	for line.Len() < 32 {
		n, e := a.In.Read(buf)
		if n == 1 {
			if buf[0] == '\n' {
				break
			}
			line.WriteByte(buf[0])
		}
		if e != nil {
			if line.Len() == 0 {
				return state.Account{}, errors.New("selection cancelled")
			}
			break
		}
	}
	i, err := strconv.Atoi(strings.TrimSpace(line.String()))
	if err != nil || i < 1 || i > len(r.Accounts) {
		return state.Account{}, errors.New("invalid account selection")
	}
	return r.Accounts[i-1], nil
}

func (a *App) launch(cmd string, args []string) (int, error) {
	name := ""
	var forward []string
	reviewed := false
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name = args[0]
		args = args[1:]
	}
	if cmd == "login" {
		if name == "" || len(args) > 0 {
			return 0, errors.New("usage: cc-router login NAME")
		}
		forward = []string{"auth", "login", "--claudeai"}
	}
	if cmd == "run" && len(args) > 0 {
		if args[0] != "--" {
			return 0, errors.New("pass Claude arguments after --")
		}
		forward = append([]string{}, args[1:]...)
	}
	if cmd == "switch" {
		if len(args) == 1 && args[0] == "--handoff-reviewed" {
			reviewed = true
		} else if len(args) != 0 {
			return 0, errors.New("usage: cc-router switch [NAME] [--handoff-reviewed]; switch starts a new session")
		}
	}
	account, err := a.selectAccount(name, true)
	if err != nil {
		return 0, err
	}
	if a.ExpectedAccountID != "" && account.ID != a.ExpectedAccountID {
		return 0, errors.New("account changed after desktop selection; refresh and try again")
	}
	accountUnlock, err := a.Store.LockAccount(account.ID)
	if err != nil {
		return 0, err
	}
	defer accountUnlock()
	r, err := a.Store.Load()
	if err != nil {
		return 0, err
	}
	current, err := r.Find(account.Name)
	if err != nil || current.ID != account.ID {
		return 0, errors.New("account changed concurrently; retry")
	}
	if cmd != "login" {
		unlock, err := a.Store.LockProject(a.Dir)
		if err != nil {
			return 0, err
		}
		defer unlock()
	}
	if cmd == "switch" {
		path, created, err := WriteHandoff(a.Dir)
		if err != nil {
			return 0, err
		}
		fmt.Fprintf(a.Err, "Handoff: %s. Ask the new session to read it and verify the workspace.\n", path)
		if !reviewed || created {
			return 0, errors.New("review and update the handoff, then run cc-router switch NAME --handoff-reviewed")
		}
	}
	if a.Launch == nil {
		return 0, errors.New("official CLI adapter unavailable")
	}
	fmt.Fprintf(a.Err, "Launching account %s (%s). Confirm identity and billing with /status.\n", account.Name, account.Label)
	if _, err := os.Stat(filepath.Join(a.Dir, ".cc-router", "handoff.md")); err == nil && cmd == "run" {
		fmt.Fprintln(a.Err, "Local handoff exists; ask Claude to read .cc-router/handoff.md and verify it.")
	}
	return a.Launch(a.Store.ProfileDir(account), a.Dir, forward)
}
