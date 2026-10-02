package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

type archiveEntry struct {
	name, body string
	mode       os.FileMode
}

func zipFixture(t *testing.T, entries []archiveEntry) (string, Asset) {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		h.SetMode(e.mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	data := b.Bytes()
	hash := sha256.Sum256(data)
	p := filepath.Join(t.TempDir(), "fixture.zip")
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	return p, Asset{OS: "windows", Arch: "amd64", Name: "fixture.zip", Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:]), GUI: "cc-router-desktop.exe", CLI: "cc-router.exe"}
}
func TestExtractVerifiedArchiveAndEntrypoints(t *testing.T) {
	p, a := zipFixture(t, []archiveEntry{{"cc-router.exe", "cli", 0755}, {"cc-router-desktop.exe", "gui", 0755}, {"LICENSE", "license", 0644}})
	dest := t.TempDir()
	files, err := Extract(p, dest, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("files=%v", files)
	}
	b, _ := os.ReadFile(filepath.Join(dest, a.CLI))
	if string(b) != "cli" {
		t.Fatal("missing cli")
	}
	if _, err := Extract(p, dest, a); err == nil {
		t.Fatal("accepted nonempty destination")
	}
	a.SHA256 = string(bytes.Repeat([]byte("0"), 64))
	if _, err := Extract(p, t.TempDir(), a); err == nil {
		t.Fatal("accepted invalid hash")
	}
}
func TestExtractRejectsUnsafeEntries(t *testing.T) {
	for _, bad := range []archiveEntry{
		{"../escape", "x", 0644}, {"/absolute", "x", 0644}, {`dir\file`, "x", 0644}, {"C:evil", "x", 0644},
		{"cc-router.exe", "target", os.ModeSymlink | 0777}, {"unshipped.txt", "x", 0644}, {"CC-ROUTER.EXE", "x", 0644},
		{"cc-router.exe ", "x", 0644}, {"cc-router.exe:stream", "x", 0644}, {"CON", "x", 0644},
	} {
		t.Run(bad.name, func(t *testing.T) {
			p, a := zipFixture(t, []archiveEntry{{"cc-router.exe", "cli", 0755}, {"cc-router-desktop.exe", "gui", 0755}, bad})
			dest := t.TempDir()
			if _, err := Extract(p, dest, a); err == nil {
				t.Fatal("accepted unsafe archive")
			}
		})
	}
	p, a := zipFixture(t, []archiveEntry{{"cc-router.exe", "cli", 0755}})
	if _, err := Extract(p, t.TempDir(), a); err == nil {
		t.Fatal("missing GUI accepted")
	}
}
func TestExtractTarAndRejectsHardlink(t *testing.T) {
	for _, hardlink := range []bool{false, true} {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		tw := tar.NewWriter(gz)
		for _, name := range []string{"./cc-router", "./cc-router-desktop"} {
			h := &tar.Header{Name: name, Mode: 0755, Size: 3, Typeflag: tar.TypeReg}
			if hardlink && name == "./cc-router-desktop" {
				h.Size = 0
				h.Typeflag = tar.TypeLink
				h.Linkname = "./cc-router"
			}
			if err := tw.WriteHeader(h); err != nil {
				t.Fatal(err)
			}
			if h.Size > 0 {
				tw.Write([]byte("exe"))
			}
		}
		tw.Close()
		gz.Close()
		data := b.Bytes()
		hash := sha256.Sum256(data)
		p := filepath.Join(t.TempDir(), "fixture.tar.gz")
		os.WriteFile(p, data, 0600)
		a := Asset{OS: "linux", Arch: "amd64", Name: "fixture.tar.gz", Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:]), GUI: "cc-router-desktop", CLI: "cc-router"}
		files, err := Extract(p, t.TempDir(), a)
		if hardlink {
			if err == nil {
				t.Fatal("accepted hardlink")
			}
		} else if err != nil || len(files) != 2 {
			t.Fatalf("tar: %v %v", files, err)
		}
	}
}

func TestExtractCleansStagingOnFailureAndRejectsOversize(t *testing.T) {
	p, a := zipFixture(t, []archiveEntry{{"cc-router.exe", "cli", 0755}, {"../escape", "bad", 0644}})
	dest := t.TempDir()
	if _, err := Extract(p, dest, a); err == nil {
		t.Fatal("accepted escape")
	}
	entries, err := os.ReadDir(dest)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed extraction left files: %v %v", entries, err)
	}
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: "cc-router.exe", Method: zip.Store, UncompressedSize64: uint64(maxArchiveBytes + 1)}
	h.SetMode(0755)
	if _, err := zw.CreateRaw(h); err != nil {
		t.Fatal(err)
	}
	zw.Close()
	data := b.Bytes()
	hash := sha256.Sum256(data)
	p = filepath.Join(t.TempDir(), "large.zip")
	os.WriteFile(p, data, 0600)
	a.Size = int64(len(data))
	a.SHA256 = hex.EncodeToString(hash[:])
	if _, err := Extract(p, dest, a); err == nil {
		t.Fatal("accepted oversized zip entry")
	}
}

func TestExtractMacApplicationTree(t *testing.T) {
	gui := "CC Router.app/Contents/MacOS/cc-router-desktop"
	cli := "CC Router.app/Contents/MacOS/cc-router"
	p, a := zipFixture(t, []archiveEntry{{gui, "gui", 0755}, {cli, "cli", 0755}, {"CC Router.app/Contents/Info.plist", "plist", 0644}, {"CC Router.app/Contents/_CodeSignature/CodeResources", "signature", 0644}})
	a.OS = "darwin"
	a.GUI = gui
	a.CLI = cli
	files, err := Extract(p, t.TempDir(), a)
	if err != nil || len(files) != 4 {
		t.Fatalf("mac app: %v %v", files, err)
	}
	for _, entry := range files {
		if entry.Path == gui && entry.Mode&0111 == 0 {
			t.Fatal("lost executable bits")
		}
	}
	p, a = zipFixture(t, []archiveEntry{{"cc-router.exe", "cli", 0755}, {"cc-router-desktop.exe", "gui", 0755}, {"cc-router.exe/child", "bad", 0644}})
	if _, err := Extract(p, t.TempDir(), a); err == nil {
		t.Fatal("accepted parent-file conflict")
	}
}

func TestMacUpdatesCannotReplaceFilesOutsideTheirBundle(t *testing.T) {
	gui, cli := "CC Router.app/Contents/MacOS/cc-router-desktop", "CC Router.app/Contents/MacOS/cc-router"
	for _, doc := range []string{"README.md", "LICENSE", "DESKTOP.md", "THIRD_PARTY_NOTICES.txt"} {
		p, a := zipFixture(t, []archiveEntry{{gui, "gui", 0755}, {cli, "cli", 0755}, {doc, "shared application folder file", 0644}})
		a.OS = "darwin"
		a.GUI = gui
		a.CLI = cli
		if _, err := Extract(p, t.TempDir(), a); err == nil {
			t.Fatalf("mac update accepted shared parent file %s", doc)
		}
	}
	p, a := zipFixture(t, []archiveEntry{{gui, "gui", 0755}, {cli, "cli", 0755}, {"CC Router.app/Contents/Resources/Documentation/README.md", "owned documentation", 0644}})
	a.OS = "darwin"
	a.GUI = gui
	a.CLI = cli
	if _, err := Extract(p, t.TempDir(), a); err != nil {
		t.Fatal(err)
	}
}
