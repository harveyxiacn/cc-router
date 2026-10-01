// Package state stores account metadata without inspecting official profile data.
package state

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const SchemaVersion = 1
const MaxJSONSize = 1024 * 1024

var ErrNoSelection = errors.New("no account selected")
var ErrBusy = errors.New("account or project is already in use")
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
var idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Account struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Label string `json:"label"`
}
type Registry struct {
	SchemaVersion int               `json:"schemaVersion"`
	Accounts      []Account         `json:"accounts"`
	DefaultID     string            `json:"defaultId,omitempty"`
	Bindings      map[string]string `json:"bindings"`
}
type Store struct{ Root string }

// LockAccount holds a nonblocking cross-process account session lock until released.
// Callers must reload the registry after locking to check that the ID still exists.
func (s *Store) LockAccount(id string) (func(), error) {
	if !idPattern.MatchString(id) {
		return nil, errors.New("invalid account lock ID")
	}
	return s.sessionLock("account-" + id + ".lock")
}

// LockProject locks the canonical project directory across managed sessions.
func (s *Store) LockProject(project string) (func(), error) {
	p, err := CanonicalProject(project)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(p))
	return s.sessionLock("project-" + hex.EncodeToString(digest[:]) + ".lock")
}

func (s *Store) sessionLock(name string) (func(), error) {
	root, err := s.openRoot()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat("locks")
	if err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("lock directory must be a real directory")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := root.Mkdir("locks", 0700); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return nil, err
			}
		} else if err := privatePath(filepath.Join(s.Root, "locks"), true); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	path := filepath.Join("locks", name)
	if err := regular(root, path); err != nil {
		return nil, err
	}
	f, err := root.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := tryLockFile(f); err != nil {
		f.Close()
		return nil, err
	}
	if err := privatePath(filepath.Join(s.Root, path), false); err != nil {
		unlockFile(f)
		f.Close()
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { unlockFile(f); f.Close() }) }, nil
}

func Open(root string) (*Store, error) {
	if root == "" {
		root = os.Getenv("CCR_HOME")
	}
	if root == "" {
		var base string
		switch runtime.GOOS {
		case "windows":
			base = os.Getenv("LOCALAPPDATA")
		case "darwin":
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			base = filepath.Join(home, "Library", "Application Support")
		default:
			base = os.Getenv("XDG_DATA_HOME")
			if base == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return nil, err
				}
				base = filepath.Join(home, ".local", "share")
			}
		}
		if base == "" || !filepath.IsAbs(base) {
			return nil, errors.New("user data directory must be absolute")
		}
		root = filepath.Join(base, "cc-router")
	}
	if !filepath.IsAbs(root) {
		return nil, errors.New("state directory must be absolute")
	}
	s := &Store{Root: filepath.Clean(root)}
	r, err := s.openRoot()
	if err != nil {
		return nil, err
	}
	r.Close()
	return s, nil
}

func (s *Store) openRoot() (*os.Root, error) {
	if !filepath.IsAbs(s.Root) {
		return nil, errors.New("state directory must be absolute")
	}
	created := false
	if info, err := os.Lstat(s.Root); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, errors.New("state directory must be a real directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else {
		if err := os.MkdirAll(filepath.Dir(s.Root), 0700); err != nil {
			return nil, err
		}
		if err := os.Mkdir(s.Root, 0700); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return nil, err
			}
		} else {
			created = true
		}
	}
	if info, err := os.Lstat(s.Root); err != nil {
		return nil, err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("state directory must be a real directory")
	}
	if created {
		if err := privatePath(s.Root, true); err != nil {
			return nil, err
		}
	}
	return os.OpenRoot(s.Root)
}

func regular(root *os.Root, name string) error {
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", name)
	}
	return nil
}

func (s *Store) locked(fn func(*os.Root) error) error {
	root, err := s.openRoot()
	if err != nil {
		return err
	}
	defer root.Close()
	if err := regular(root, "state.lock"); err != nil {
		return err
	}
	f, err := root.OpenFile("state.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return err
	}
	defer unlockFile(f)
	if err := privatePath(filepath.Join(s.Root, "state.lock"), false); err != nil {
		return err
	}
	return fn(root)
}

func fresh() Registry {
	return Registry{SchemaVersion: SchemaVersion, Accounts: []Account{}, Bindings: map[string]string{}}
}

func readRegistry(root *os.Root) (Registry, error) {
	if err := regular(root, "accounts.json"); err != nil {
		return Registry{}, err
	}
	f, err := root.Open("accounts.json")
	if errors.Is(err, os.ErrNotExist) {
		return fresh(), nil
	}
	if err != nil {
		return Registry{}, err
	}
	defer f.Close()
	var r Registry
	if err := decodeStrict(f, &r); err != nil {
		return Registry{}, fmt.Errorf("invalid account registry: %w", err)
	}
	if err := r.validate(); err != nil {
		return Registry{}, err
	}
	if r.Bindings == nil {
		r.Bindings = map[string]string{}
	}
	return r, nil
}

func (s *Store) Load() (Registry, error) {
	var r Registry
	err := s.locked(func(root *os.Root) (err error) { r, err = readRegistry(root); return })
	return r, err
}

func (s *Store) Update(fn func(*Registry) error) error {
	return s.locked(func(root *os.Root) error {
		r, err := readRegistry(root)
		if err != nil {
			return err
		}
		if err := fn(&r); err != nil {
			return err
		}
		if err := r.validate(); err != nil {
			return err
		}
		data, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		if len(data) > MaxJSONSize {
			return errors.New("account registry exceeds size limit")
		}
		if err := s.ensureProfiles(root, r.Accounts); err != nil {
			return err
		}
		return s.write(root, data)
	})
}

func (s *Store) ensureProfiles(root *os.Root, accounts []Account) error {
	paths := []string{"profiles"}
	for _, a := range accounts {
		paths = append(paths, filepath.Join("profiles", a.ID))
	}
	for _, p := range paths {
		info, err := root.Lstat(p)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("profile directory must be a real directory")
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		} else {
			if err := root.Mkdir(p, 0700); err != nil {
				return err
			}
			if err := privatePath(filepath.Join(s.Root, p), true); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) write(root *os.Root, data []byte) error {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	name := ".accounts-" + hex.EncodeToString(id[:]) + ".tmp"
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(name)
	if err = privatePath(filepath.Join(s.Root, name), false); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := regular(root, "accounts.json"); err != nil {
		return err
	}
	if err := root.Rename(name, "accounts.json"); err != nil {
		return err
	}
	return syncRoot(root)
}

func (s *Store) ProfileDir(a Account) string {
	if !idPattern.MatchString(a.ID) {
		return ""
	}
	return filepath.Join(s.Root, "profiles", a.ID)
}

func validName(name string) error {
	if !namePattern.MatchString(name) {
		return errors.New("account name must contain 1-64 lowercase letters, digits, underscores or hyphens and start with a letter or digit")
	}
	return nil
}
func validLabel(label string) error {
	if len(label) > 256 || !utf8.ValidString(label) {
		return errors.New("invalid account label")
	}
	for _, r := range label {
		if unicode.IsControl(r) {
			return errors.New("account label contains a control character")
		}
	}
	return nil
}
func (r *Registry) validate() error {
	if r.SchemaVersion != SchemaVersion {
		return errors.New("unsupported registry schema version")
	}
	if r.Accounts == nil {
		return errors.New("accounts must be an array")
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for _, a := range r.Accounts {
		if !idPattern.MatchString(a.ID) {
			return errors.New("invalid account ID")
		}
		if err := validName(a.Name); err != nil {
			return err
		}
		if err := validLabel(a.Label); err != nil {
			return err
		}
		if ids[a.ID] || names[a.Name] {
			return errors.New("duplicate account ID or name")
		}
		ids[a.ID], names[a.Name] = true, true
	}
	if r.DefaultID != "" && !ids[r.DefaultID] {
		return errors.New("default account does not exist")
	}
	for path, id := range r.Bindings {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || !ids[id] {
			return errors.New("invalid project binding")
		}
	}
	return nil
}

func (r *Registry) Find(name string) (Account, error) {
	for _, a := range r.Accounts {
		if a.Name == name {
			return a, nil
		}
	}
	return Account{}, fmt.Errorf("account %q does not exist", name)
}
func (r *Registry) Add(name, label string) (Account, error) {
	if err := validName(name); err != nil {
		return Account{}, err
	}
	if err := validLabel(label); err != nil {
		return Account{}, err
	}
	if _, err := r.Find(name); err == nil {
		return Account{}, errors.New("account name already exists")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Account{}, err
	}
	a := Account{ID: hex.EncodeToString(id[:]), Name: name, Label: label}
	r.Accounts = append(r.Accounts, a)
	return a, nil
}
func (r *Registry) Rename(name, newName, label string) error {
	if err := validName(newName); err != nil {
		return err
	}
	if err := validLabel(label); err != nil {
		return err
	}
	a, err := r.Find(name)
	if err != nil {
		return err
	}
	if newName != name {
		if _, err := r.Find(newName); err == nil {
			return errors.New("account name already exists")
		}
	}
	for i := range r.Accounts {
		if r.Accounts[i].ID == a.ID {
			r.Accounts[i].Name, r.Accounts[i].Label = newName, label
		}
	}
	return nil
}
func (r *Registry) Remove(name string) error {
	a, err := r.Find(name)
	if err != nil {
		return err
	}
	for i := range r.Accounts {
		if r.Accounts[i].ID == a.ID {
			r.Accounts = append(r.Accounts[:i], r.Accounts[i+1:]...)
			break
		}
	}
	if r.DefaultID == a.ID {
		r.DefaultID = ""
	}
	for p, id := range r.Bindings {
		if id == a.ID {
			delete(r.Bindings, p)
		}
	}
	return nil
}
func (r *Registry) Select(name, project string) (Account, error) {
	if name != "" {
		return r.Find(name)
	}
	id := r.Bindings[project]
	if id == "" {
		id = r.DefaultID
	}
	if id == "" {
		return Account{}, ErrNoSelection
	}
	for _, a := range r.Accounts {
		if a.ID == id {
			return a, nil
		}
	}
	return Account{}, errors.New("selected account does not exist")
}
func (r *Registry) SetDefault(name string) error {
	a, err := r.Find(name)
	if err != nil {
		return err
	}
	r.DefaultID = a.ID
	return nil
}
func (r *Registry) Bind(project, name string) error {
	p, err := CanonicalProject(project)
	if err != nil {
		return err
	}
	a, err := r.Find(name)
	if err != nil {
		return err
	}
	if r.Bindings == nil {
		r.Bindings = map[string]string{}
	}
	r.Bindings[p] = a.ID
	return nil
}
func (r *Registry) Unbind(project string) error {
	p, err := CanonicalProject(project)
	if err != nil {
		return err
	}
	delete(r.Bindings, p)
	return nil
}
func CanonicalProject(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("project must be a directory")
	}
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path, nil
}

type exportAccount struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}
type exportFile struct {
	SchemaVersion  int             `json:"schemaVersion"`
	Accounts       []exportAccount `json:"accounts"`
	DefaultAccount string          `json:"defaultAccount,omitempty"`
}

func (s *Store) Export(w io.Writer) error {
	r, err := s.Load()
	if err != nil {
		return err
	}
	doc := exportFile{SchemaVersion: SchemaVersion, Accounts: []exportAccount{}}
	for _, a := range r.Accounts {
		doc.Accounts = append(doc.Accounts, exportAccount{Name: a.Name, Label: a.Label})
		if a.ID == r.DefaultID {
			doc.DefaultAccount = a.Name
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
func (s *Store) Import(reader io.Reader) error {
	var doc exportFile
	if err := decodeStrict(reader, &doc); err != nil {
		return fmt.Errorf("invalid metadata import: %w", err)
	}
	if doc.SchemaVersion != SchemaVersion || doc.Accounts == nil {
		return errors.New("unsupported import schema or missing accounts")
	}
	seen := map[string]bool{}
	for _, a := range doc.Accounts {
		if err := validName(a.Name); err != nil {
			return err
		}
		if err := validLabel(a.Label); err != nil {
			return err
		}
		if seen[a.Name] {
			return errors.New("duplicate import account")
		}
		seen[a.Name] = true
	}
	if doc.DefaultAccount != "" && !seen[doc.DefaultAccount] {
		return errors.New("import default account is absent")
	}
	return s.Update(func(r *Registry) error {
		for _, a := range doc.Accounts {
			if existing, err := r.Find(a.Name); err == nil {
				for i := range r.Accounts {
					if r.Accounts[i].ID == existing.ID {
						r.Accounts[i].Label = a.Label
					}
				}
			} else {
				if _, err := r.Add(a.Name, a.Label); err != nil {
					return err
				}
			}
		}
		if doc.DefaultAccount != "" {
			return r.SetDefault(doc.DefaultAccount)
		}
		return nil
	})
}

// Detect duplicate object keys before decoding; encoding/json otherwise accepts them.
func decodeStrict(reader io.Reader, out any) error {
	data, err := io.ReadAll(io.LimitReader(reader, MaxJSONSize+1))
	if err != nil {
		return errors.New("cannot read metadata JSON")
	}
	if len(data) > MaxJSONSize {
		return errors.New("metadata JSON exceeds size limit")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("metadata JSON must be an object")
	}
	tokens := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueValue(tokens); err != nil {
		return errors.New("malformed JSON or duplicate object keys")
	}
	if _, err := tokens.Token(); err != io.EOF {
		return errors.New("trailing JSON data")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return errors.New("malformed JSON or unknown metadata fields")
	}
	return nil
}
func uniqueValue(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
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
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("duplicate key")
			}
			seen[name] = true
			if err := uniqueValue(dec); err != nil {
				return err
			}
		}
	case '[':
		for dec.More() {
			if err := uniqueValue(dec); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	_, err = dec.Token()
	return err
}
