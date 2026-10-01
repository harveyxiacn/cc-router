package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/harveyxiacn/cc-router/internal/localdata"
	"github.com/harveyxiacn/cc-router/internal/state"
)

const workDirectory = ".cc-router-update-work"

var transactionID = regexp.MustCompile(`^[a-f0-9]{32}$`)

type journalEntry struct {
	New File  `json:"new"`
	Old *File `json:"old"`
}
type transaction struct {
	Schema  int            `json:"schema"`
	ID      string         `json:"id"`
	Phase   string         `json:"phase"`
	Entries []journalEntry `json:"entries"`
	Root    string         `json:"-"`
}

func plainParents(root *os.Root, name string, create bool) error {
	name = filepath.ToSlash(name)
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:\x00") || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") || strings.Contains(name, "/./") {
		return errors.New("invalid update file path")
	}
	parts := strings.Split(name, "/")
	for i := 1; i < len(parts); i++ {
		parent := strings.Join(parts[:i], "/")
		st, err := root.Lstat(parent)
		if errors.Is(err, os.ErrNotExist) && create {
			if err = root.Mkdir(parent, 0755); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			st, err = root.Lstat(parent)
		}
		if err != nil {
			return err
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return errors.New("update paths cannot traverse links or non-directories")
		}
	}
	if st, err := root.Lstat(name); err == nil {
		if !st.Mode().IsRegular() {
			return errors.New("update target must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
func measure(root *os.Root, name string) (File, error) {
	var result File
	if err := plainParents(root, name, false); err != nil {
		return result, err
	}
	f, err := root.Open(name)
	if err != nil {
		return result, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return result, err
	}
	if !st.Mode().IsRegular() || st.Size() > 128*1024*1024 {
		return result, errors.New("update file is not regular or exceeds limit")
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, 128*1024*1024+1))
	if err != nil {
		return result, err
	}
	if n != st.Size() {
		return result, errors.New("update file changed while reading")
	}
	return File{Path: filepath.ToSlash(name), SHA256: hex.EncodeToString(h.Sum(nil)), Size: n, Mode: uint32(st.Mode().Perm())}, nil
}
func sameFileData(a, b File) bool { return a.SHA256 == b.SHA256 && a.Size == b.Size }
func copyVerified(src *os.Root, source string, dst *os.Root, destination string, expected File) error {
	actual, err := measure(src, source)
	if err != nil {
		return err
	}
	if !sameFileData(actual, expected) {
		return errors.New("update file checksum changed")
	}
	if err := plainParents(dst, destination, true); err != nil {
		return err
	}
	in, err := src.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := dst.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(in, expected.Size+1))
	if err == nil && (n != expected.Size || hex.EncodeToString(h.Sum(nil)) != expected.SHA256) {
		err = errors.New("update file changed while copying")
	}
	if err == nil {
		err = out.Chmod(os.FileMode(expected.Mode) & 0777)
	}
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = dst.Remove(destination)
	}
	return err
}
func workRoot(directory string) (*os.Root, error) {
	s, err := state.Open(filepath.Join(directory, workDirectory))
	if err != nil {
		return nil, err
	}
	return os.OpenRoot(s.Root)
}
func (t *transaction) save() error {
	root, err := workRoot(t.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return localdata.Write(root, t.ID+"/journal.json", b)
}
func beginTransaction(directory, id, staged string, files []File) (*transaction, error) {
	if !transactionID.MatchString(id) || len(files) == 0 || len(files) > 512 {
		return nil, errors.New("invalid transaction")
	}
	work, err := workRoot(directory)
	if err != nil {
		return nil, err
	}
	defer work.Close()
	if err := work.Mkdir(id, 0700); err != nil {
		return nil, err
	}
	if err := work.Mkdir(id+"/backup", 0700); err != nil {
		return nil, err
	}
	if err := work.Mkdir(id+"/new", 0700); err != nil {
		return nil, err
	}
	install, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer install.Close()
	source, err := os.OpenRoot(staged)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	t := &transaction{Schema: 1, ID: id, Root: directory, Phase: "preparing", Entries: []journalEntry{}}
	seen := map[string]bool{}
	for _, f := range files {
		if seen[strings.ToLower(f.Path)] {
			return nil, errors.New("duplicate update file")
		}
		seen[strings.ToLower(f.Path)] = true
		if strings.HasPrefix(f.Path, ".") {
			return nil, errors.New("update cannot replace hidden installation state")
		}
		if err := copyVerified(source, f.Path, work, id+"/new/"+f.Path, f); err != nil {
			return nil, err
		}
		entry := journalEntry{New: f}
		// Missing parent directories are created only when installing the new files.
		old, err := measure(install, f.Path)
		if err == nil {
			entry.Old = &old
			if err := copyVerified(install, f.Path, work, id+"/backup/"+f.Path, old); err != nil {
				return nil, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		t.Entries = append(t.Entries, entry)
	}
	t.Phase = "prepared"
	if err := t.save(); err != nil {
		return nil, err
	}
	return t, nil
}
func openTransaction(directory, id string) (*transaction, error) {
	if !transactionID.MatchString(id) {
		return nil, errors.New("invalid transaction ID")
	}
	root, err := workRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, err := localdata.Read(root, id+"/journal.json", 1024*1024)
	if err != nil {
		return nil, err
	}
	var t transaction
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	if t.Schema != 1 || t.ID != id || len(t.Entries) == 0 || len(t.Entries) > 512 {
		return nil, errors.New("invalid update journal")
	}
	seen := map[string]bool{}
	for _, entry := range t.Entries {
		if entry.New.Path == "" || strings.HasPrefix(entry.New.Path, ".") || seen[strings.ToLower(entry.New.Path)] {
			return nil, errors.New("invalid journal path")
		}
		seen[strings.ToLower(entry.New.Path)] = true
		if entry.Old != nil && entry.Old.Path != entry.New.Path {
			return nil, errors.New("invalid backup path")
		}
	}
	t.Root = directory
	return &t, nil
}
func (t *transaction) install() error {
	if t.Phase != "prepared" {
		return errors.New("transaction is not prepared")
	}
	root, err := os.OpenRoot(t.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	// Recheck the private transaction copy immediately before the first mutation.
	for _, entry := range t.Entries {
		actual, err := measure(root, workDirectory+"/"+t.ID+"/new/"+entry.New.Path)
		if err != nil || !sameFileData(actual, entry.New) {
			return errors.New("prepared update files changed before installation")
		}
	}
	t.Phase = "installing"
	if err := t.save(); err != nil {
		return err
	}
	for _, entry := range t.Entries {
		name := entry.New.Path
		if err := plainParents(root, name, true); err != nil {
			return errors.Join(err, t.rollback())
		}
		source := workDirectory + "/" + t.ID + "/new/" + name
		if err := root.Rename(source, name); err != nil {
			return errors.Join(fmt.Errorf("cannot replace %s: %w", name, err), t.rollback())
		}
	}
	t.Phase = "probation"
	return t.save()
}
func (t *transaction) rollback() error {
	root, err := os.OpenRoot(t.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	t.Phase = "restoring"
	if err := t.save(); err != nil {
		return err
	}
	for _, entry := range t.Entries {
		name := entry.New.Path
		if err := plainParents(root, name, true); err != nil {
			return err
		}
		if entry.Old == nil {
			if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		backup := workDirectory + "/" + t.ID + "/backup/" + name
		temp := workDirectory + "/" + t.ID + "/restore/" + name
		// A prior interrupted restore may leave only its owned temporary copy.
		if err := plainParents(root, temp, true); err != nil {
			return err
		}
		if err := root.Remove(temp); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := copyVerified(root, backup, root, temp, *entry.Old); err != nil {
			return err
		}
		if err := root.Rename(temp, name); err != nil {
			return err
		}
	}
	t.Phase = "rolled-back"
	return t.save()
}
func (t *transaction) finish(health func() error) error {
	if t.Phase != "probation" {
		return errors.New("transaction has not installed all files")
	}
	if err := health(); err != nil {
		return errors.Join(err, t.rollback())
	}
	t.Phase = "complete"
	return t.save()
}
