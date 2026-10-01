package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

const maxExtractedBytes int64 = 512 << 20
const maxExtractedFiles = 512

func safeArchivePath(name string) (string, error) {
	if name == "" || len(name) > 1024 || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") {
		return "", errors.New("unsafe update archive path")
	}
	for _, c := range name {
		if c < 32 || c == 127 {
			return "", errors.New("unsafe update archive path")
		}
	}
	for strings.HasPrefix(name, "./") {
		name = strings.TrimPrefix(name, "./")
	}
	name = strings.TrimSuffix(name, "/")
	if name == "" || name == "." {
		return ".", nil
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return "", errors.New("unsafe update archive path")
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return "", errors.New("reserved update archive path")
		}
	}
	return name, nil
}

type extractor struct {
	root      *os.Root
	asset     Asset
	seen      map[string]bool
	spellings map[string]string
	created   []string
	files     []File
	total     int64
	entries   int
}

// Extract verifies the compressed bytes and confines all writes to an initially
// empty directory. No links, device files, unshipped root entries, or executable
// entrypoint omissions are permitted, on any platform.
func Extract(archivePath, dest string, asset Asset) (files []File, result error) {
	if err := validateAsset(asset); err != nil {
		return nil, err
	}
	info, err := os.Lstat(archivePath)
	if err != nil || !info.Mode().IsRegular() || info.Size() != asset.Size {
		return nil, errors.New("update archive is not a regular file of signed size")
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, errors.New("cannot open update archive")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || opened.Size() != asset.Size {
		return nil, errors.New("update archive changed before verification")
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, asset.Size+1))
	if err != nil || n != asset.Size || hex.EncodeToString(h.Sum(nil)) != asset.SHA256 {
		return nil, errors.New("update archive SHA256 verification failed")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	dinfo, err := os.Lstat(dest)
	if os.IsNotExist(err) {
		if err = os.Mkdir(dest, 0700); err != nil {
			return nil, errors.New("cannot create update staging directory")
		}
		dinfo, err = os.Lstat(dest)
	}
	if err != nil || !dinfo.IsDir() || dinfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("update staging directory must be a real directory")
	}
	root, err := os.OpenRoot(dest)
	if err != nil {
		return nil, errors.New("cannot open update staging directory")
	}
	defer root.Close()
	d, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	entries, err := d.Readdirnames(1)
	d.Close()
	if err != io.EOF || len(entries) > 0 {
		return nil, errors.New("update staging directory must be empty")
	}
	e := &extractor{root: root, asset: asset, seen: map[string]bool{}, spellings: map[string]string{}}
	defer func() {
		if result != nil {
			for i := len(e.created) - 1; i >= 0; i-- {
				root.Remove(e.created[i])
			}
		}
	}()
	if strings.HasSuffix(asset.Name, ".zip") {
		zr, err := zip.NewReader(f, asset.Size)
		if err != nil {
			return nil, errors.New("invalid update zip archive")
		}
		if len(zr.File) > maxExtractedFiles {
			return nil, errors.New("too many update archive entries")
		}
		for _, entry := range zr.File {
			mode := entry.Mode()
			if !mode.IsRegular() && !mode.IsDir() {
				return nil, errors.New("update archive contains a link or special file")
			}
			if entry.UncompressedSize64 > uint64(maxArchiveBytes) {
				return nil, errors.New("update archive entry exceeds size limit")
			}
			r, err := entry.Open()
			if err != nil {
				return nil, errors.New("cannot read update archive entry")
			}
			err = e.write(entry.Name, mode, int64(entry.UncompressedSize64), r)
			r.Close()
			if err != nil {
				return nil, err
			}
		}
	} else {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, errors.New("invalid update gzip archive")
		}
		defer gz.Close()
		gz.Multistream(false)
		tr := tar.NewReader(gz)
		for {
			entry, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, errors.New("invalid update tar archive")
			}
			var mode os.FileMode
			switch entry.Typeflag {
			case tar.TypeReg, tar.TypeRegA:
				mode = os.FileMode(entry.Mode) & 0777
			case tar.TypeDir:
				mode = os.ModeDir | 0755
			default:
				return nil, errors.New("update archive contains a link or special file")
			}
			if err = e.write(entry.Name, mode, entry.Size, tr); err != nil {
				return nil, err
			}
		}
		// Read the gzip footer rather than accepting a tar stream with a bad CRC.
		if n, err := io.Copy(io.Discard, io.LimitReader(gz, 1<<20)); err != nil || n >= 1<<20 {
			return nil, errors.New("invalid trailing update archive data")
		}
	}
	gui, cli := false, false
	for _, f := range e.files {
		if f.Path == asset.GUI {
			gui = true
			if asset.OS != "windows" && f.Mode&0111 == 0 {
				return nil, errors.New("GUI entrypoint is not executable")
			}
		}
		if f.Path == asset.CLI {
			cli = true
			if asset.OS != "windows" && f.Mode&0111 == 0 {
				return nil, errors.New("CLI entrypoint is not executable")
			}
		}
	}
	if !gui || !cli {
		return nil, errors.New("update archive is missing GUI or CLI entrypoint")
	}
	sort.Slice(e.files, func(i, j int) bool { return e.files[i].Path < e.files[j].Path })
	return e.files, nil
}
func (e *extractor) allowed(name string, isDir bool) bool {
	if name == "." {
		return isDir
	}
	if name == e.asset.GUI || name == e.asset.CLI {
		return !isDir
	}
	for _, doc := range []string{"LICENSE", "README.md", "DESKTOP.md", "compatibility.md", "THIRD_PARTY_NOTICES.txt"} {
		if name == doc {
			return !isDir
		}
	}
	if e.asset.OS == "darwin" {
		app := strings.Split(e.asset.GUI, "/")[0]
		return name == app && isDir || strings.HasPrefix(name, app+"/Contents/") || name == app+"/Contents" && isDir
	}
	return false
}
func (e *extractor) recordSpelling(name string) error {
	parts := strings.Split(name, "/")
	for i := range parts {
		p := strings.Join(parts[:i+1], "/")
		key := strings.ToLower(p)
		if previous, ok := e.spellings[key]; ok && previous != p {
			return errors.New("update archive contains case-colliding paths")
		}
		e.spellings[key] = p
	}
	return nil
}
func (e *extractor) parents(name string) error {
	dir := path.Dir(name)
	if dir == "." {
		return nil
	}
	parts := strings.Split(dir, "/")
	for i := range parts {
		p := strings.Join(parts[:i+1], "/")
		info, err := e.root.Lstat(p)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("update archive path conflicts with a file")
			}
			continue
		}
		if !os.IsNotExist(err) {
			return err
		}
		if err = e.root.Mkdir(p, 0755); err != nil {
			return errors.New("cannot create update staging directory")
		}
		e.created = append(e.created, p)
	}
	return nil
}
func (e *extractor) write(name string, mode os.FileMode, size int64, r io.Reader) error {
	e.entries++
	if e.entries > maxExtractedFiles || size < 0 || size > maxArchiveBytes || e.total+size > maxExtractedBytes {
		return errors.New("update extraction limit exceeded")
	}
	name, err := safeArchivePath(name)
	if err != nil {
		return err
	}
	if !e.allowed(name, mode.IsDir()) {
		return errors.New("update archive contains an unexpected root entry")
	}
	if err = e.recordSpelling(name); err != nil {
		return err
	}
	if name == "." {
		return nil
	}
	if e.seen[name] {
		return errors.New("update archive contains duplicate entries")
	}
	e.seen[name] = true
	if err = e.parents(name); err != nil {
		return err
	}
	if mode.IsDir() {
		if size != 0 {
			return errors.New("update directory contains unexpected data")
		}
		if info, err := e.root.Lstat(name); err == nil {
			if !info.IsDir() {
				return errors.New("update directory conflicts with a file")
			}
			return nil
		}
		if err = e.root.Mkdir(name, 0755); err != nil {
			return errors.New("cannot create update staging directory")
		}
		e.created = append(e.created, name)
		return nil
	}
	permission := os.FileMode(0644) | (mode.Perm() & 0111)
	f, err := e.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, permission)
	if err != nil {
		return errors.New("cannot create update staging file")
	}
	e.created = append(e.created, name)
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, size+1))
	syncErr := f.Sync()
	closeErr := f.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || n != size {
		return errors.New("update archive entry is truncated or invalid")
	}
	e.total += n
	e.files = append(e.files, File{Path: name, SHA256: hex.EncodeToString(h.Sum(nil)), Size: n, Mode: uint32(permission)})
	return nil
}
