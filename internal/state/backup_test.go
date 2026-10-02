package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestBackupRoundTripPreservesIdentityBindingsAndProfile(t *testing.T) {
	s := testStore(t)
	a := addAccount(t, s, "one")
	project, _ := CanonicalProject(t.TempDir())
	if err := s.Update(func(r *Registry) error { r.DefaultID = a.ID; r.Bindings[project] = a.ID; return nil }); err != nil {
		t.Fatal(err)
	}
	want, _ := s.Load()
	sentinel := filepath.Join(s.ProfileDir(a), "official-secret")
	if err := os.WriteFile(sentinel, []byte("never read or copy me"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := s.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewBackup(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Restorable || info.Reason != "manual" || info.AccountCount != 1 || info.BindingCount != 1 || preview.DefaultAccountID != a.ID || preview.ProjectBindings[project] != a.ID {
		t.Fatalf("unexpected backup: %+v / %+v", info, preview)
	}
	raw, err := os.ReadFile(filepath.Join(s.Root, "backups", info.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("never read or copy me")) {
		t.Fatal("profile secret copied")
	}
	if err := s.Update(func(r *Registry) error { return r.Remove("one") }); err != nil {
		t.Fatal(err)
	}
	b := addAccount(t, s, "two")
	prior, err := s.RestoreBackup(info.ID, preview.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if prior.ID == info.ID || prior.Reason != "pre-restore" || !prior.Restorable {
		t.Fatalf("wrong pre-restore: %+v", prior)
	}
	got, err := s.Load()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip: %+v, %v", got, err)
	}
	old, err := s.PreviewBackup(prior.ID)
	if err != nil || len(old.Accounts) != 1 || old.Accounts[0].ID != b.ID {
		t.Fatalf("pre-restore state: %+v, %v", old, err)
	}
	contents, err := os.ReadFile(sentinel)
	if err != nil || string(contents) != "never read or copy me" {
		t.Fatal("profile changed")
	}
	list, err := s.ListBackups()
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %+v, %v", list, err)
	}
}

func TestBackupForensicFileIntegrity(t *testing.T) {
	s := testStore(t)
	addAccount(t, s, "one")
	backup, err := s.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewBackup(backup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "accounts.json"), []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	prior, err := s.RestoreBackup(backup.ID, preview.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "backups", prior.ID+".raw"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PreviewBackup(prior.ID); err == nil {
		t.Fatal("accepted modified forensic backup")
	}
}

func TestBackupListRetainsValidSnapshotsWithDisabledInvalidEntries(t *testing.T) {
	s := testStore(t)
	addAccount(t, s, "one")
	valid, err := s.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	invalidID := strings.Repeat("f", 32)
	if err := os.WriteFile(filepath.Join(s.Root, "backups", invalidID+".json"), []byte(`{"createdAt":"2099-01-01T00:00:00Z","reason":"manual","accountCount":99}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "backups", "unexpected.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("lost available snapshots: %+v", list)
	}
	if list[0].ID != valid.ID || !list[0].Restorable || list[0].Error != "" {
		t.Fatalf("valid snapshot unavailable: %+v", list[0])
	}
	bad := list[1]
	if bad.ID != invalidID || bad.Restorable || bad.Reason != "invalid" || bad.Error == "" || !bad.CreatedAt.IsZero() || bad.AccountCount != 0 || bad.BindingCount != 0 {
		t.Fatalf("untrusted diagnostic: %+v", bad)
	}
	if _, err := s.PreviewBackup(invalidID); err == nil {
		t.Fatal("invalid snapshot preview accepted")
	}
	if _, err := s.RestoreBackup(invalidID, "ignored"); err == nil {
		t.Fatal("invalid snapshot restored")
	}
}

func TestBackupUpdateProvenanceAndCorruptStateRejection(t *testing.T) {
	s := testStore(t)
	a := addAccount(t, s, "one")
	backup, err := s.CreateUpdateBackup()
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewBackup(backup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if backup.Reason != "pre-update" || !backup.Restorable || len(preview.Accounts) != 1 || preview.Accounts[0].ID != a.ID {
		t.Fatalf("wrong update provenance: %+v %+v", backup, preview)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "accounts.json"), []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUpdateBackup(); err == nil {
		t.Fatal("updating corrupt state accepted")
	}
	list, err := s.ListBackups()
	if err != nil || len(list) != 1 {
		t.Fatalf("invalid update made snapshot: %d %v", len(list), err)
	}
}

func TestBackupRejectsOversizedForensicRegistryBeforeMutation(t *testing.T) {
	s := testStore(t)
	addAccount(t, s, "one")
	backup, err := s.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewBackup(backup.ID)
	if err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(s.Root, "accounts.json")
	raw := bytes.Repeat([]byte("x"), 16*1024*1024+1)
	if err := os.WriteFile(current, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestoreBackup(backup.ID, preview.Digest); err == nil {
		t.Fatal("restored over oversized current registry")
	}
	got, err := os.ReadFile(current)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal("oversized raw registry changed")
	}
	entries, err := os.ReadDir(filepath.Join(s.Root, "backups"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("oversize restore wrote partial backup: %d, %v", len(entries), err)
	}
}

func TestBackupAcceptsBoundedCompactRegistry(t *testing.T) {
	s := testStore(t)
	registry := fresh()
	for i := 0; i < 10000; i++ {
		registry.Accounts = append(registry.Accounts, Account{ID: fmt.Sprintf("%032x", i), Name: fmt.Sprintf("account-%d", i), Label: "Label"})
	}
	compact, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	pretty, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(compact) > MaxJSONSize || len(pretty) <= MaxJSONSize {
		t.Fatalf("test fixture wrong size: compact=%d pretty=%d", len(compact), len(pretty))
	}
	if err := os.WriteFile(filepath.Join(s.Root, "accounts.json"), compact, 0600); err != nil {
		t.Fatal(err)
	}
	backup, err := s.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewBackup(backup.ID)
	if err != nil || len(preview.Accounts) != 10000 {
		t.Fatalf("bounded valid registry rejected: %d, %v", len(preview.Accounts), err)
	}
}

func TestBackupRestoresExactSizeLimitRegistry(t *testing.T) {
	s := testStore(t)
	a := addAccount(t, s, "one")
	registry, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(s.Root, "project")
	registry.Bindings[key] = a.ID
	data, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	delete(registry.Bindings, key)
	registry.Bindings[key+strings.Repeat("p", MaxJSONSize-len(data))] = a.ID
	data, err = json.Marshal(registry)
	if err != nil || len(data) != MaxJSONSize {
		t.Fatalf("fixture size: %d, %v", len(data), err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "accounts.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	backup, err := s.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewBackup(backup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestoreBackup(backup.ID, preview.Digest); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Load(); err != nil || !reflect.DeepEqual(got, registry) {
		t.Fatalf("size-limit restore failed: %v", err)
	}
}

func TestBackupContainmentAndNonregularFiles(t *testing.T) {
	for _, attack := range []string{"backup-directory", "backup-file", "profile-directory"} {
		t.Run(attack, func(t *testing.T) {
			s := testStore(t)
			a := addAccount(t, s, "one")
			backup, err := s.CreateBackup()
			if err != nil {
				t.Fatal(err)
			}
			preview, err := s.PreviewBackup(backup.ID)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(s.Root, "accounts.json"))
			if err != nil {
				t.Fatal(err)
			}
			switch attack {
			case "backup-directory":
				if err := os.Rename(filepath.Join(s.Root, "backups"), filepath.Join(s.Root, "saved-backups")); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(s.Root, "backups"), []byte("file"), 0600); err != nil {
					t.Fatal(err)
				}
			case "backup-file":
				path := filepath.Join(s.Root, "backups", backup.ID+".json")
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "profile-directory":
				if err := os.Remove(s.ProfileDir(a)); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(s.ProfileDir(a), []byte("file"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.RestoreBackup(backup.ID, preview.Digest); err == nil {
				t.Fatal("accepted nonregular path")
			}
			after, err := os.ReadFile(filepath.Join(s.Root, "accounts.json"))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed containment check changed registry")
			}
		})
	}
	for _, attack := range []string{"backups", "snapshot", "profiles"} {
		t.Run("symlink-"+attack, func(t *testing.T) {
			s := testStore(t)
			a := addAccount(t, s, "one")
			backup, err := s.CreateBackup()
			if err != nil {
				t.Fatal(err)
			}
			preview, _ := s.PreviewBackup(backup.ID)
			outside := t.TempDir()
			target := filepath.Join(s.Root, "backups")
			if attack == "snapshot" {
				target = filepath.Join(target, backup.ID+".json")
				outside = filepath.Join(outside, "snapshot")
				if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
			} else if attack == "profiles" {
				target = s.ProfileDir(a)
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Rename(target, filepath.Join(s.Root, "old-backups")); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(outside, target); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := s.RestoreBackup(backup.ID, preview.Digest); err == nil {
				t.Fatal("accepted symlink")
			}
		})
	}
}

func TestBackupConcurrentSnapshotsAndRestore(t *testing.T) {
	s := testStore(t)
	addAccount(t, s, "one")
	var wg sync.WaitGroup
	errors := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.CreateBackup(); errors <- err }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListBackups()
	if err != nil || len(list) != 12 {
		t.Fatalf("concurrent snapshots: %d, %v", len(list), err)
	}
	preview, err := s.PreviewBackup(list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	errors = make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.RestoreBackup(list[0].ID, preview.Digest); errors <- err }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	list, err = s.ListBackups()
	if err != nil || len(list) != 16 {
		t.Fatalf("concurrent restores: %d, %v", len(list), err)
	}
}

func TestBackupRestorePreservesCorruptAndFutureCurrentBytes(t *testing.T) {
	for _, raw := range []string{"{broken", `{"schemaVersion":99,"accounts":[],"bindings":{}}`, strings.Repeat("x", MaxJSONSize+1)} {
		t.Run(raw[:7], func(t *testing.T) {
			s := testStore(t)
			addAccount(t, s, "one")
			backup, err := s.CreateBackup()
			if err != nil {
				t.Fatal(err)
			}
			preview, err := s.PreviewBackup(backup.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(s.Root, "accounts.json"), []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := s.CreateBackup(); err == nil {
				t.Fatal("manual snapshot accepted invalid registry")
			}
			prior, err := s.RestoreBackup(backup.ID, preview.Digest)
			if err != nil {
				t.Fatal(err)
			}
			if prior.Restorable {
				t.Fatal("forensic metadata marked restorable")
			}
			saved, err := os.ReadFile(filepath.Join(s.Root, "backups", prior.ID+".raw"))
			if err != nil || string(saved) != raw {
				t.Fatal("current raw registry lost")
			}
			if _, err := s.PreviewBackup(prior.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.RestoreBackup(prior.ID, preview.Digest); err == nil {
				t.Fatal("restored forensic backup")
			}
			if r, err := s.Load(); err != nil || len(r.Accounts) != 1 {
				t.Fatalf("recovery failed: %+v, %v", r, err)
			}
		})
	}
}

func TestBackupRestoreBusyRejectsWithoutMutation(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "corrupt"}[corrupt], func(t *testing.T) {
			s := testStore(t)
			addAccount(t, s, "one")
			backup, _ := s.CreateBackup()
			preview, _ := s.PreviewBackup(backup.ID)
			active := addAccount(t, s, "active")
			release, err := s.LockAccount(active.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			if corrupt {
				if err := os.WriteFile(filepath.Join(s.Root, "accounts.json"), []byte("{broken"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(filepath.Join(s.Root, "accounts.json"))
			if _, err := s.RestoreBackup(backup.ID, preview.Digest); !errors.Is(err, ErrBusy) {
				t.Fatalf("busy restore: %v", err)
			}
			after, _ := os.ReadFile(filepath.Join(s.Root, "accounts.json"))
			if !bytes.Equal(before, after) {
				t.Fatal("busy restore changed registry")
			}
			list, err := s.ListBackups()
			if err != nil || len(list) != 1 {
				t.Fatalf("busy restore made backup: %+v %v", list, err)
			}
		})
	}
}

func TestBackupRejectsUnreviewedAndInvalidEnvelope(t *testing.T) {
	s := testStore(t)
	addAccount(t, s, "one")
	backup, _ := s.CreateBackup()
	preview, _ := s.PreviewBackup(backup.ID)
	for _, id := range []string{"../accounts", backup.ID + ".json", "", strings.ToUpper(backup.ID), "/absolute"} {
		if _, err := s.PreviewBackup(id); err == nil {
			t.Errorf("accepted id %q", id)
		}
	}
	path := filepath.Join(s.Root, "backups", backup.ID+".json")
	original, _ := os.ReadFile(path)
	if _, err := s.RestoreBackup(backup.ID, ""); err == nil {
		t.Fatal("unreviewed restore accepted")
	}
	if err := os.WriteFile(path, append(append([]byte{}, original...), '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestoreBackup(backup.ID, preview.Digest); err == nil {
		t.Fatal("changed snapshot restored")
	}
	var doc map[string]any
	if err := json.Unmarshal(original, &doc); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"schemaVersion", "rootFingerprint", "id", "registry"} {
		t.Run(field, func(t *testing.T) {
			var attack map[string]any
			json.Unmarshal(original, &attack)
			switch field {
			case "schemaVersion":
				attack[field] = 99
			case "rootFingerprint":
				attack[field] = strings.Repeat("a", 64)
			case "id":
				attack[field] = "../escape"
			case "registry":
				attack[field].(map[string]any)["schemaVersion"] = 99
			}
			data, _ := json.Marshal(attack)
			os.WriteFile(path, data, 0600)
			if _, err := s.PreviewBackup(backup.ID); err == nil {
				t.Fatal("accepted hostile backup")
			}
		})
	}
	os.WriteFile(path, original, 0600)
	other := testStore(t)
	if err := os.Mkdir(filepath.Join(other.Root, "backups"), 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(other.Root, "backups", backup.ID+".json"), original, 0600)
	if _, err := other.PreviewBackup(backup.ID); err == nil {
		t.Fatal("accepted backup from different root")
	}
}
