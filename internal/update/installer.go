package update

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/harveyxiacn/cc-router/internal/localdata"
)

const maxInstallerTransactions = 128
const maxInstallerNodes = 32768
const maxInstallerDepth = 64
const maxInstallerMetadata = 16 << 20

// RetireInstallerState removes only recognized terminal OTA work data before a
// user-selected manual install or uninstall. It takes its own exclusive lease;
// callers must release any installer lease before invoking this operation.
// Application files, account data and update caches are never accessed.
func RetireInstallerState(directory string) error {
	if !filepath.IsAbs(directory) {
		return errors.New("installation directory must be absolute")
	}
	directory = filepath.Clean(directory)
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("installation directory must be a real directory")
	}
	release, err := AcquireLease(directory, true)
	if err != nil {
		return err
	}
	defer release()
	install, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer install.Close()
	opened, err := install.Open(".")
	if err != nil {
		return err
	}
	actual, statErr := opened.Stat()
	opened.Close()
	if statErr != nil {
		return statErr
	}
	if !os.SameFile(info, actual) {
		return errors.New("installation directory changed")
	}
	workInfo, err := install.Lstat(workDirectory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !workInfo.IsDir() || workInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("update work directory must be a real directory")
	}
	work, err := install.OpenRoot(workDirectory)
	if err != nil {
		return err
	}
	openedWork, err := work.Open(".")
	if err != nil {
		work.Close()
		return err
	}
	actualWork, statErr := openedWork.Stat()
	openedWork.Close()
	if statErr != nil {
		work.Close()
		return statErr
	}
	if !os.SameFile(workInfo, actualWork) {
		work.Close()
		return errors.New("update work directory changed before preflight")
	}
	err = preflightInstallerWork(work)
	closeErr := work.Close()
	if err != nil {
		return fmt.Errorf("update state cannot be safely retired: %w", err)
	}
	if closeErr != nil {
		return closeErr
	}
	// Check the exact constant target's identity again before any recursive delete.
	current, err := install.Lstat(workDirectory)
	if err != nil {
		return err
	}
	if !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(current, workInfo) {
		return errors.New("update work directory changed during preflight")
	}
	return install.RemoveAll(workDirectory)
}

func installerReadJSON(root *os.Root, name string, limit int64, out any) (int, error) {
	data, err := localdata.Read(root, name, limit)
	if err != nil {
		return 0, err
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return 0, errors.New("update metadata must be a JSON object")
	}
	if err := rejectDuplicateKeys(data); err != nil {
		return 0, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return 0, errors.New("invalid update metadata fields")
	}
	return len(data), nil
}

// Each allowed path is either a directory or a regular file. Parent directories
// are admitted only because a journal names an exact descendant file.
func installerAllow(allowed map[string]bool, name string, directory bool) error {
	if previous, exists := allowed[name]; exists && previous != directory {
		return errors.New("conflicting journal file and directory paths")
	}
	allowed[name] = directory
	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		if previous, exists := allowed[parent]; exists && !previous {
			return errors.New("journal file used as an ancestor directory")
		}
		allowed[parent] = true
	}
	return nil
}

func installerValidFile(file File) error {
	canonical, err := safeArchivePath(file.Path)
	if err != nil || canonical != file.Path || canonical == "." || strings.HasPrefix(canonical, ".") || strings.Count(canonical, "/")+3 > maxInstallerDepth {
		return errors.New("unsafe installer journal path")
	}
	hash, err := hex.DecodeString(file.SHA256)
	if err != nil || len(hash) != 32 || strings.ToLower(file.SHA256) != file.SHA256 || file.Size < 0 || file.Size > 128<<20 || file.Mode&^0777 != 0 {
		return errors.New("invalid installer journal file metadata")
	}
	return nil
}

func preflightInstallerWork(root *os.Root) error {
	entries, err := installerReadDir(root, ".", maxInstallerTransactions+1)
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	transactions := map[string]bool{}
	var active *reference
	metadataBytes := 0
	for _, entry := range entries {
		name := entry.Name()
		info, err := root.Lstat(name)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("update state contains a symbolic link")
		}
		if name == "active.json" {
			var ref reference
			size, err := installerReadJSON(root, name, 64<<10, &ref)
			if err != nil {
				return err
			}
			metadataBytes += size
			if metadataBytes > maxInstallerMetadata {
				return errors.New("retained update metadata exceeds limit")
			}
			if !transactionID.MatchString(ref.ID) || !filepath.IsAbs(ref.DataRoot) || filepath.Clean(ref.DataRoot) != ref.DataRoot {
				return errors.New("invalid active update reference")
			}
			active = &ref
			if err := installerAllow(allowed, name, false); err != nil {
				return err
			}
			continue
		}
		if !transactionID.MatchString(name) || !info.IsDir() {
			return errors.New("unrecognized update work entry")
		}
		transactions[name] = true
		if len(transactions) > maxInstallerTransactions {
			return errors.New("too many retained update transactions")
		}
		if err := installerAllow(allowed, name, true); err != nil {
			return err
		}
		var journal transaction
		size, err := installerReadJSON(root, name+"/journal.json", 1<<20, &journal)
		if err != nil {
			return err
		}
		metadataBytes += size
		if metadataBytes > maxInstallerMetadata {
			return errors.New("retained update metadata exceeds limit")
		}
		if journal.Schema != 1 || journal.ID != name || (journal.Phase != "complete" && journal.Phase != "rolled-back") || len(journal.Entries) == 0 || len(journal.Entries) > maxExtractedFiles {
			return errors.New("update transaction is incomplete or invalid")
		}
		if err := installerAllow(allowed, name+"/journal.json", false); err != nil {
			return err
		}
		for _, bucket := range []string{"new", "backup", "restore"} {
			if err := installerAllow(allowed, name+"/"+bucket, true); err != nil {
				return err
			}
		}
		seen := map[string]bool{}
		var total int64
		for _, entry := range journal.Entries {
			if err := installerValidFile(entry.New); err != nil {
				return err
			}
			folded := strings.ToLower(entry.New.Path)
			if seen[folded] {
				return errors.New("duplicate installer journal file")
			}
			seen[folded] = true
			total += entry.New.Size
			if total > maxExtractedBytes {
				return errors.New("installer journal content exceeds limit")
			}
			if err := installerAllow(allowed, name+"/new/"+entry.New.Path, false); err != nil {
				return err
			}
			if entry.Old != nil {
				if entry.Old.Path != entry.New.Path {
					return errors.New("installer journal backup path mismatch")
				}
				if err := installerValidFile(*entry.Old); err != nil {
					return err
				}
				for _, bucket := range []string{"backup", "restore"} {
					if err := installerAllow(allowed, name+"/"+bucket+"/"+entry.New.Path, false); err != nil {
						return err
					}
				}
			}
		}
	}
	if active != nil && !transactions[active.ID] {
		return errors.New("active update transaction is missing")
	}
	visited := 0
	return installerWalk(root, ".", allowed, 0, &visited)
}

func installerReadDir(root *os.Root, name string, limit int) ([]os.DirEntry, error) {
	directory, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > limit {
		return nil, errors.New("update directory contains too many entries")
	}
	return entries, nil
}

func installerWalk(root *os.Root, name string, allowed map[string]bool, depth int, visited *int) error {
	if depth > maxInstallerDepth {
		return errors.New("update work tree exceeds depth limit")
	}
	entries, err := installerReadDir(root, name, maxInstallerNodes-*visited)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		*visited = *visited + 1
		if *visited > maxInstallerNodes {
			return errors.New("update work tree exceeds entry limit")
		}
		relative := entry.Name()
		if name != "." {
			relative = name + "/" + relative
		}
		directory, recognized := allowed[relative]
		if !recognized {
			return errors.New("unrecognized file or directory in update transaction")
		}
		info, err := root.Lstat(relative)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("update transaction contains a symbolic link")
		}
		if directory {
			if !info.IsDir() {
				return errors.New("update transaction directory is not a directory")
			}
			if err := installerWalk(root, relative, allowed, depth+1, visited); err != nil {
				return err
			}
		} else if !info.Mode().IsRegular() {
			return errors.New("update transaction file is not regular")
		}
	}
	return nil
}
