// release-sign authenticates locally verified release archives. It never uploads a key.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/harveyxiacn/cc-router/internal/releasekey"
	"github.com/harveyxiacn/cc-router/internal/update"
)

var assetName = regexp.MustCompile(`^cc-router-desktop-(windows|linux|darwin)-(amd64|arm64)\.(zip|tar\.gz)$`)

func main() {
	version := flag.String("version", "", "release version without v")
	dir := flag.String("assets", "dist", "directory containing the exact release archives")
	notes := flag.String("notes", "", "UTF-8 release notes file")
	flag.Parse()
	if err := run(*version, *dir, *notes); err != nil {
		fmt.Fprintln(os.Stderr, "release signing:", err)
		os.Exit(1)
	}
}
func run(version, dir, notesPath string) error {
	key, err := releasekey.Key(false)
	if err != nil {
		return err
	}
	public, err := update.TrustedPublicKey()
	if err != nil {
		return err
	}
	if !bytes.Equal(public, key.Public().(ed25519.PublicKey)) {
		return errors.New("maintainer key does not match the embedded verification key")
	}
	notes := ""
	if notesPath != "" {
		b, err := os.ReadFile(notesPath)
		if err != nil {
			return err
		}
		if len(b) > 64<<10 {
			return errors.New("release notes exceed limit")
		}
		notes = string(b)
	}
	manifest, err := buildManifest(version, dir, notes)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	signature := ed25519.Sign(key, data)
	if _, err = update.VerifyManifest(data, signature, public, time.Now()); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, update.ManifestName), data, 0644); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, update.SignatureName), []byte(base64.StdEncoding.EncodeToString(signature)+"\n"), 0644); err != nil {
		return err
	}
	fmt.Printf("Signed %d verified desktop archives for v%s; private key remained local.\n", len(manifest.Assets), version)
	return nil
}
func buildManifest(version, dir, notes string) (update.Manifest, error) {
	now := time.Now().UTC()
	m := update.Manifest{Schema: 1, Version: version, PublishedAt: now, ExpiresAt: now.Add(365 * 24 * time.Hour), Notes: notes, Assets: []update.Asset{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return m, err
	}
	for _, e := range entries {
		parts := assetName.FindStringSubmatch(e.Name())
		if parts == nil {
			continue
		}
		if !e.Type().IsRegular() {
			return m, errors.New("release archive must be a regular file")
		}
		name := filepath.Join(dir, e.Name())
		f, err := os.Open(name)
		if err != nil {
			return m, err
		}
		h := sha256.New()
		n, err := io.Copy(h, io.LimitReader(f, (128<<20)+1))
		f.Close()
		if err != nil {
			return m, err
		}
		if n > 128<<20 {
			return m, errors.New("release archive exceeds updater limit")
		}
		a := update.Asset{OS: parts[1], Arch: parts[2], Name: e.Name(), Size: n, SHA256: hex.EncodeToString(h.Sum(nil)), GUI: "cc-router-desktop", CLI: "cc-router"}
		if a.OS == "windows" {
			a.GUI += ".exe"
			a.CLI += ".exe"
		}
		if a.OS == "darwin" {
			bundle, err := macBundle(name)
			if err != nil {
				return m, err
			}
			a.GUI = bundle + "/Contents/MacOS/cc-router-desktop"
			a.CLI = bundle + "/Contents/MacOS/cc-router"
		}
		temp, err := os.MkdirTemp("", "cc-router-release-check-")
		if err != nil {
			return m, err
		}
		_, err = update.Extract(name, temp, a)
		// This temporary directory was created above, with no caller-controlled path.
		cleanupErr := os.RemoveAll(temp)
		if err != nil {
			return m, err
		}
		if cleanupErr != nil {
			return m, cleanupErr
		}
		m.Assets = append(m.Assets, a)
	}
	if len(m.Assets) == 0 {
		return m, errors.New("no desktop release archives found")
	}
	return m, nil
}
func macBundle(name string) (string, error) {
	found := ""
	visit := func(name string) error {
		for strings.HasPrefix(name, "./") {
			name = strings.TrimPrefix(name, "./")
		}
		parts := strings.Split(name, "/")
		if len(parts) == 4 && strings.HasSuffix(parts[0], ".app") && parts[1] == "Contents" && parts[2] == "MacOS" && parts[3] == "cc-router-desktop" {
			if found != "" {
				return errors.New("multiple app entrypoints")
			}
			found = parts[0]
		}
		return nil
	}
	if strings.HasSuffix(name, ".zip") {
		z, err := zip.OpenReader(name)
		if err != nil {
			return "", err
		}
		defer z.Close()
		if len(z.File) > 512 {
			return "", errors.New("too many archive files")
		}
		for _, f := range z.File {
			if err = visit(f.Name); err != nil {
				return "", err
			}
		}
	} else {
		f, err := os.Open(name)
		if err != nil {
			return "", err
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return "", err
		}
		defer gz.Close()
		tr := tar.NewReader(io.LimitReader(gz, (512<<20)+1))
		for i := 0; ; i++ {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}
			if i >= 512 {
				return "", errors.New("too many archive entries")
			}
			if err = visit(h.Name); err != nil {
				return "", err
			}
		}
	}
	if found == "" {
		return "", errors.New("missing macOS app entrypoint")
	}
	return found, nil
}
