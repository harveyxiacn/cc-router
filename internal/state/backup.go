package state

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const backupSchemaVersion = 1
const maxBackupJSONSize = 2 * MaxJSONSize
const maxForensicRegistrySize = 16 * 1024 * 1024

// BackupInfo describes a local metadata snapshot. Forensic snapshots preserve
// invalid current registry bytes and cannot be restored by this API.
type BackupInfo struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"createdAt"`
	Reason       string    `json:"reason"`
	AccountCount int       `json:"accountCount"`
	BindingCount int       `json:"bindingCount"`
	Restorable   bool      `json:"restorable"`
	Error        string    `json:"error,omitempty"`
}

type BackupPreview struct {
	Info             BackupInfo        `json:"info"`
	Accounts         []Account         `json:"accounts"`
	DefaultAccountID string            `json:"defaultAccountId"`
	ProjectBindings  map[string]string `json:"projectBindings"`
	Digest           string            `json:"digest"`
}

type backupEnvelope struct {
	SchemaVersion   int    `json:"schemaVersion"`
	RootFingerprint string `json:"rootFingerprint"`
	BackupInfo
	Registry  *Registry `json:"registry,omitempty"`
	RawSHA256 string    `json:"rawSha256,omitempty"`
	RawSize   int64     `json:"rawSize,omitempty"`
}

func (s *Store) backupRoot(root *os.Root, create bool) (*os.Root, error) {
	info, err := root.Lstat("backups")
	if errors.Is(err, os.ErrNotExist) && create {
		if err = root.Mkdir("backups", 0700); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("backup directory must be a real directory")
	}
	if err := privatePath(filepath.Join(s.Root, "backups"), true); err != nil {
		return nil, err
	}
	return root.OpenRoot("backups")
}

func (s *Store) rootFingerprint() (string, error) {
	canonical, err := filepath.EvalSymlinks(s.Root)
	if err != nil {
		return "", err
	}
	canonical = filepath.Clean(canonical)
	if runtime.GOOS == "windows" {
		canonical = strings.ToLower(canonical)
	}
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:]), nil
}

func (s *Store) CreateBackup() (BackupInfo, error) {
	return s.createRegistryBackup("manual")
}

// CreateUpdateBackup preserves validated metadata before an update is applied.
// Updating an unsupported or damaged current registry is refused.
func (s *Store) CreateUpdateBackup() (BackupInfo, error) {
	return s.createRegistryBackup("pre-update")
}

func (s *Store) createRegistryBackup(reason string) (BackupInfo, error) {
	var info BackupInfo
	err := s.locked(func(root *os.Root) error {
		registry, err := readRegistry(root)
		if err != nil {
			return err
		}
		if err := validateBackupProfiles(root, registry.Accounts); err != nil {
			return err
		}
		info, err = s.saveBackup(root, &registry, reason)
		return err
	})
	return info, err
}

// ListBackups keeps valid snapshots available for recovery and describes damaged
// snapshots using trusted disabled diagnostics, never their unvalidated metadata.
func (s *Store) ListBackups() ([]BackupInfo, error) {
	list := []BackupInfo{}
	err := s.locked(func(root *os.Root) error {
		backups, err := s.backupRoot(root, false)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer backups.Close()
		directory, err := backups.Open(".")
		if err != nil {
			return err
		}
		defer directory.Close()
		entries, err := directory.ReadDir(-1)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			id := strings.TrimSuffix(entry.Name(), ".json")
			if !idPattern.MatchString(id) {
				continue
			}
			envelope, _, err := s.readBackup(backups, id)
			if err != nil {
				list = append(list, BackupInfo{ID: id, Reason: "invalid", Error: "Snapshot is damaged, unsupported, or belongs to another data directory"})
				continue
			}
			list = append(list, envelope.BackupInfo)
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].CreatedAt.Equal(list[j].CreatedAt) {
				return list[i].ID < list[j].ID
			}
			return list[i].CreatedAt.After(list[j].CreatedAt)
		})
		return nil
	})
	return list, err
}

func (s *Store) PreviewBackup(id string) (BackupPreview, error) {
	var preview BackupPreview
	err := s.locked(func(root *os.Root) error {
		backups, err := s.backupRoot(root, false)
		if err != nil {
			return err
		}
		defer backups.Close()
		envelope, digest, err := s.readBackup(backups, id)
		if err != nil {
			return err
		}
		preview = BackupPreview{Info: envelope.BackupInfo, Accounts: []Account{}, ProjectBindings: map[string]string{}, Digest: digest}
		if envelope.Registry != nil {
			preview.Accounts = envelope.Registry.Accounts
			preview.DefaultAccountID = envelope.Registry.DefaultID
			preview.ProjectBindings = envelope.Registry.Bindings
		}
		return nil
	})
	return preview, err
}

// RestoreBackup requires the SHA256 returned by PreviewBackup so the metadata
// being restored is the exact snapshot that was reviewed. All session locks are
// attempted nonblockingly while the registry lock is held to avoid lock inversion.
func (s *Store) RestoreBackup(id, expectedDigest string) (BackupInfo, error) {
	var previous BackupInfo
	err := s.locked(func(root *os.Root) error {
		backups, err := s.backupRoot(root, false)
		if err != nil {
			return err
		}
		defer backups.Close()
		envelope, digest, err := s.readBackup(backups, id)
		if err != nil {
			return err
		}
		if expectedDigest == "" || expectedDigest != digest {
			return errors.New("backup changed or was not reviewed; preview it again")
		}
		if !envelope.Restorable {
			return errors.New("forensic registry backup cannot be restored")
		}
		registry := envelope.Registry
		if err := validateBackupProfiles(root, registry.Accounts); err != nil {
			return err
		}
		ids := map[string]bool{}
		for _, a := range registry.Accounts {
			ids[a.ID] = true
		}
		current, currentErr := readRegistry(root)
		if currentErr == nil {
			for _, a := range current.Accounts {
				ids[a.ID] = true
			}
		}
		// Tool-owned session lock names identify active accounts even if the current
		// registry cannot be decoded. No official profile contents are inspected.
		if err := existingAccountLocks(root, ids); err != nil {
			return err
		}
		ordered := make([]string, 0, len(ids))
		for id := range ids {
			ordered = append(ordered, id)
		}
		sort.Strings(ordered)
		releases := []func(){}
		defer func() {
			for i := len(releases) - 1; i >= 0; i-- {
				releases[i]()
			}
		}()
		for _, id := range ordered {
			release, err := s.LockAccount(id)
			if err != nil {
				return err
			}
			releases = append(releases, release)
		}
		if currentErr == nil {
			previous, err = s.saveBackup(root, &current, "pre-restore")
		} else {
			previous, err = s.saveBackup(root, nil, "pre-restore")
		}
		if err != nil {
			return err
		}
		if err := s.ensureProfiles(root, registry.Accounts); err != nil {
			return err
		}
		// Compact encoding preserves the supported size bound even when a valid
		// compact source registry would exceed it after adding indentation.
		data, err := json.Marshal(registry)
		if err != nil {
			return err
		}
		if len(data) > MaxJSONSize {
			return errors.New("restored registry exceeds size limit")
		}
		return s.write(root, data)
	})
	return previous, err
}

func existingAccountLocks(root *os.Root, ids map[string]bool) error {
	info, err := root.Lstat("locks")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("lock directory must be a real directory")
	}
	directory, err := root.Open("locks")
	if err != nil {
		return err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "account-") && strings.HasSuffix(name, ".lock") {
			id := strings.TrimSuffix(strings.TrimPrefix(name, "account-"), ".lock")
			if !idPattern.MatchString(id) {
				return errors.New("invalid account session lock name")
			}
			if err := regular(root, filepath.Join("locks", name)); err != nil {
				return err
			}
			ids[id] = true
		}
	}
	return nil
}

func validateBackupProfiles(root *os.Root, accounts []Account) error {
	paths := []string{"profiles"}
	for _, a := range accounts {
		paths = append(paths, filepath.Join("profiles", a.ID))
	}
	for _, path := range paths {
		info, err := root.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("profile directory must be a real directory")
		}
	}
	return nil
}

func (s *Store) saveBackup(root *os.Root, registry *Registry, reason string) (BackupInfo, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return BackupInfo{}, err
	}
	id := hex.EncodeToString(random[:])
	fingerprint, err := s.rootFingerprint()
	if err != nil {
		return BackupInfo{}, err
	}
	envelope := backupEnvelope{SchemaVersion: backupSchemaVersion, RootFingerprint: fingerprint, BackupInfo: BackupInfo{ID: id, CreatedAt: time.Now().UTC(), Reason: reason, Restorable: registry != nil}, Registry: registry}
	if registry != nil {
		envelope.AccountCount = len(registry.Accounts)
		envelope.BindingCount = len(registry.Bindings)
	}
	backups, err := s.backupRoot(root, true)
	if err != nil {
		return BackupInfo{}, err
	}
	defer backups.Close()
	committed := false
	rawCreated := false
	defer func() {
		if rawCreated && !committed {
			backups.Remove(id + ".raw")
		}
	}()
	if registry == nil {
		if err := regular(root, "accounts.json"); err != nil {
			return BackupInfo{}, err
		}
		current, err := root.Open("accounts.json")
		if err != nil {
			return BackupInfo{}, err
		}
		defer current.Close()
		info, err := current.Stat()
		if err != nil {
			return BackupInfo{}, err
		}
		if !info.Mode().IsRegular() || info.Size() > maxForensicRegistrySize {
			return BackupInfo{}, errors.New("current registry cannot be preserved: forensic metadata must be a regular file of at most 16 MiB")
		}
		saved, err := backups.OpenFile(id+".raw", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return BackupInfo{}, err
		}
		rawCreated = true
		if err := privatePath(filepath.Join(s.Root, "backups", id+".raw"), false); err != nil {
			saved.Close()
			return BackupInfo{}, err
		}
		digest := sha256.New()
		size, copyErr := io.Copy(io.MultiWriter(saved, digest), io.LimitReader(current, maxForensicRegistrySize+1))
		if copyErr == nil && size > maxForensicRegistrySize {
			copyErr = errors.New("current registry exceeds forensic preservation limit of 16 MiB")
		}
		if copyErr == nil {
			copyErr = saved.Sync()
		}
		closeErr := saved.Close()
		if copyErr != nil {
			return BackupInfo{}, copyErr
		}
		if closeErr != nil {
			return BackupInfo{}, closeErr
		}
		envelope.RawSHA256 = hex.EncodeToString(digest.Sum(nil))
		envelope.RawSize = size
	}
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return BackupInfo{}, err
	}
	data = append(data, '\n')
	if len(data) > maxBackupJSONSize {
		return BackupInfo{}, errors.New("backup exceeds size limit")
	}
	name := "." + id + ".tmp"
	output, err := backups.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return BackupInfo{}, err
	}
	defer backups.Remove(name)
	if err = privatePath(filepath.Join(s.Root, "backups", name), false); err == nil {
		_, err = output.Write(data)
	}
	if err == nil {
		err = output.Sync()
	}
	closeErr := output.Close()
	if err != nil {
		return BackupInfo{}, err
	}
	if closeErr != nil {
		return BackupInfo{}, closeErr
	}
	if _, err := backups.Lstat(id + ".json"); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			err = errors.New("backup ID collision")
		}
		return BackupInfo{}, err
	}
	if err := backups.Rename(name, id+".json"); err != nil {
		return BackupInfo{}, err
	}
	committed = true
	if err := syncRoot(backups); err != nil {
		return BackupInfo{}, err
	}
	return envelope.BackupInfo, nil
}

func (s *Store) readBackup(backups *os.Root, id string) (backupEnvelope, string, error) {
	var envelope backupEnvelope
	if !idPattern.MatchString(id) {
		return envelope, "", errors.New("invalid backup ID")
	}
	if err := regular(backups, id+".json"); err != nil {
		return envelope, "", err
	}
	input, err := backups.Open(id + ".json")
	if err != nil {
		return envelope, "", err
	}
	defer input.Close()
	data, err := io.ReadAll(io.LimitReader(input, maxBackupJSONSize+1))
	if err != nil {
		return envelope, "", err
	}
	if len(data) > maxBackupJSONSize {
		return envelope, "", errors.New("backup exceeds size limit")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return envelope, "", errors.New("backup JSON must be an object")
	}
	tokens := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueValue(tokens); err != nil {
		return envelope, "", errors.New("malformed backup or duplicate keys")
	}
	if _, err := tokens.Token(); err != io.EOF {
		return envelope, "", errors.New("trailing backup JSON data")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return envelope, "", errors.New("malformed backup or unknown fields")
	}
	fingerprint, err := s.rootFingerprint()
	if err != nil {
		return envelope, "", err
	}
	if envelope.SchemaVersion != backupSchemaVersion || envelope.ID != id || envelope.RootFingerprint != fingerprint || envelope.CreatedAt.IsZero() || envelope.Error != "" || (envelope.Reason != "manual" && envelope.Reason != "pre-restore" && envelope.Reason != "pre-update") {
		return envelope, "", errors.New("invalid backup version, identity, root or metadata")
	}
	if envelope.Restorable {
		if envelope.Registry == nil || envelope.RawSHA256 != "" || envelope.RawSize != 0 {
			return envelope, "", errors.New("invalid restorable backup")
		}
		if err := envelope.Registry.validate(); err != nil {
			return envelope, "", err
		}
		if envelope.AccountCount != len(envelope.Registry.Accounts) || envelope.BindingCount != len(envelope.Registry.Bindings) {
			return envelope, "", errors.New("backup counts do not match registry")
		}
		if envelope.Registry.Bindings == nil {
			envelope.Registry.Bindings = map[string]string{}
		}
		registryData, err := json.Marshal(envelope.Registry)
		if err != nil || len(registryData) > MaxJSONSize {
			return envelope, "", errors.New("backup registry exceeds size limit")
		}
	} else {
		hash, err := hex.DecodeString(envelope.RawSHA256)
		if envelope.Registry != nil || envelope.Reason != "pre-restore" || err != nil || len(hash) != sha256.Size || envelope.RawSize < 0 || envelope.RawSize > maxForensicRegistrySize || envelope.AccountCount != 0 || envelope.BindingCount != 0 {
			return envelope, "", errors.New("invalid forensic backup")
		}
		info, err := backups.Lstat(id + ".raw")
		if err != nil {
			return envelope, "", err
		}
		if !info.Mode().IsRegular() || info.Size() != envelope.RawSize {
			return envelope, "", errors.New("invalid forensic registry file")
		}
		raw, err := backups.Open(id + ".raw")
		if err != nil {
			return envelope, "", err
		}
		digest := sha256.New()
		size, copyErr := io.Copy(digest, io.LimitReader(raw, maxForensicRegistrySize+1))
		closeErr := raw.Close()
		if copyErr != nil {
			return envelope, "", copyErr
		}
		if closeErr != nil {
			return envelope, "", closeErr
		}
		if size != envelope.RawSize || hex.EncodeToString(digest.Sum(nil)) != envelope.RawSHA256 {
			return envelope, "", errors.New("forensic registry checksum mismatch")
		}
	}
	digest := sha256.Sum256(data)
	return envelope, hex.EncodeToString(digest[:]), nil
}
