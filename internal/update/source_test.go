package update

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func signedFixture(t *testing.T, payload []byte) (Manifest, []byte, []byte, ed25519.PublicKey) {
	t.Helper()
	key, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(payload)
	suffix, gui, cli := ".tar.gz", "cc-router-desktop", "cc-router"
	if runtime.GOOS == "windows" {
		suffix, gui, cli = ".zip", gui+".exe", cli+".exe"
	}
	if runtime.GOOS == "darwin" {
		gui, cli = "CC Router.app/Contents/MacOS/cc-router-desktop", "CC Router.app/Contents/MacOS/cc-router"
	}
	m := Manifest{Schema: 1, Version: "1.2.0", PublishedAt: time.Now().UTC().Add(-time.Hour), ExpiresAt: time.Now().UTC().Add(time.Hour), Assets: []Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, Name: "cc-router-desktop-" + runtime.GOOS + "-" + runtime.GOARCH + suffix, Size: int64(len(payload)), SHA256: hex.EncodeToString(hash[:]), GUI: gui, CLI: cli}}}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return m, b, ed25519.Sign(private, b), key
}

func TestVerifyManifestMandatorySignatureAndStrictSchema(t *testing.T) {
	m, b, sig, key := signedFixture(t, []byte("archive"))
	if got, err := VerifyManifest(b, sig, key, time.Now()); err != nil || got.Version != m.Version {
		t.Fatalf("valid: %v %v", got, err)
	}
	for name, mutate := range map[string]func() ([]byte, []byte, ed25519.PublicKey, time.Time){
		"missing signature": func() ([]byte, []byte, ed25519.PublicKey, time.Time) { return b, nil, key, time.Now() },
		"tampered": func() ([]byte, []byte, ed25519.PublicKey, time.Time) {
			return append(append([]byte{}, b...), ' '), sig, key, time.Now()
		},
		"wrong key": func() ([]byte, []byte, ed25519.PublicKey, time.Time) { return b, sig, make([]byte, 32), time.Now() },
		"expired": func() ([]byte, []byte, ed25519.PublicKey, time.Time) {
			return b, sig, key, m.ExpiresAt.Add(time.Second)
		},
	} {
		t.Run(name, func(t *testing.T) {
			d, s, k, n := mutate()
			if _, err := VerifyManifest(d, s, k, n); err == nil {
				t.Fatal("accepted invalid manifest")
			}
		})
	}
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	key = private.Public().(ed25519.PublicKey)
	for _, invalid := range []string{
		strings.Replace(string(b), `"schema":1`, `"schema":2`, 1),
		strings.Replace(string(b), `"schema":1`, `"schema":1,"schema":1`, 1),
		strings.Replace(string(b), `"schema":1`, `"schema":1,"unexpected":true`, 1),
		strings.Replace(string(b), `"version":"1.2.0"`, `"version":"01.2.0"`, 1),
		strings.Replace(string(b), `"name":"cc-router`, `"name":"../cc-router`, 1),
		string(b) + ` {}`, strings.Repeat(" ", 1<<20) + string(b),
	} {
		data := []byte(invalid)
		if _, err := VerifyManifest(data, ed25519.Sign(private, data), key, time.Now()); err == nil {
			t.Fatal("accepted invalid schema or fields")
		}
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixtureSource(t *testing.T, payload []byte, releases string) (*Source, *Candidate) {
	t.Helper()
	m, b, sig, key := signedFixture(t, payload)
	s := NewSource()
	s.key = key
	s.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" {
			t.Fatalf("non-HTTPS %s", r.URL)
		}
		var data []byte
		switch {
		case r.URL.Host == "api.github.com":
			data = []byte(releases)
		case strings.HasSuffix(r.URL.Path, "/"+ManifestName):
			data = b
		case strings.HasSuffix(r.URL.Path, "/"+SignatureName):
			data = []byte(base64.StdEncoding.EncodeToString(sig) + "\n")
		case strings.HasSuffix(r.URL.Path, "/"+m.Assets[0].Name):
			data = payload
		default:
			return nil, fmt.Errorf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header), ContentLength: int64(len(data)), Request: r}, nil
	})}
	return s, &Candidate{Manifest: m, Asset: m.Assets[0], ManifestBytes: b, Signature: sig}
}

func TestFetchSelectsVersionChannelAndSignedAsset(t *testing.T) {
	s, _ := fixtureSource(t, []byte("payload"), `[{"tag_name":"v1.1.0"},{"tag_name":"v1.2.0"},{"tag_name":"v9.0.0-beta.1","prerelease":true},{"tag_name":"junk"}]`)
	c, err := s.Fetch(context.Background(), "1.0.0")
	if err != nil || c == nil || c.Manifest.Version != "1.2.0" || len(c.Signature) != 64 {
		t.Fatalf("Fetch: %+v %v", c, err)
	}
	c, err = s.Fetch(context.Background(), "1.2.0")
	if err != nil || c != nil {
		t.Fatalf("no newer: %+v %v", c, err)
	}
	// The same fixture manifest cannot impersonate a selected prerelease tag.
	if _, err = s.Fetch(context.Background(), "1.0.0-alpha.1"); err == nil {
		t.Fatal("accepted manifest/tag mismatch")
	}
}

func TestDownloadChecksExactSizeHashAndDoesNotOverwrite(t *testing.T) {
	payload := []byte("verified archive payload")
	s, c := fixtureSource(t, payload, `[]`)
	dest := filepath.Join(t.TempDir(), "archive.zip")
	var last int64
	if err := s.Download(context.Background(), c, dest, func(n, total int64) {
		last = n
		if total != int64(len(payload)) {
			t.Error("wrong total")
		}
	}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dest)
	if !bytes.Equal(b, payload) || last != int64(len(payload)) {
		t.Fatal("wrong download")
	}
	if err := s.Download(context.Background(), c, dest, nil); err == nil {
		t.Fatal("overwrote existing file")
	}
	s.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("wrong data")), Header: make(http.Header), Request: r}, nil
	})}
	bad := filepath.Join(t.TempDir(), "bad.zip")
	if err := s.Download(context.Background(), c, bad, nil); err == nil {
		t.Fatal("accepted bad download")
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatal("left failed output")
	}
	c.Asset.Name = "../../credentials"
	if err := s.Download(context.Background(), c, bad, nil); err == nil {
		t.Fatal("accepted mutated candidate")
	}
}

func TestDownloadRejectsUntrustedRedirect(t *testing.T) {
	s, c := fixtureSource(t, []byte("payload"), `[]`)
	s.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "github.com" {
			t.Fatal("untrusted redirect requested")
		}
		return &http.Response{StatusCode: 302, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{"Location": []string{"https://evil.invalid/payload"}}, Request: r}, nil
	})}
	if err := s.Download(context.Background(), c, filepath.Join(t.TempDir(), "a.zip"), nil); err == nil {
		t.Fatal("followed untrusted redirect")
	}
}

func TestVerifyManifestRejectsAmbiguousFieldNamesAndPlatformPaths(t *testing.T) {
	_, b, _, _ := signedFixture(t, []byte("payload"))
	key, private, _ := ed25519.GenerateKey(rand.Reader)
	for _, invalid := range []string{
		strings.Replace(string(b), `"schema":1`, `"schema":1,"Schema":1`, 1),
		strings.Replace(string(b), `"schema":1`, `"Schema":1`, 1),
		strings.Replace(string(b), `"gui":"`, `"GUI":"`, 1),
		strings.Replace(string(b), `"schema":1`, `"schema":1.0`, 1),
	} {
		data := []byte(invalid)
		if _, err := VerifyManifest(data, ed25519.Sign(private, data), key, time.Now()); err == nil {
			t.Fatal("accepted ambiguous manifest fields")
		}
	}
}

func TestDownloadCancellationAndBoundedData(t *testing.T) {
	s, c := fixtureSource(t, []byte("payload"), `[]`)
	s.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dest := filepath.Join(t.TempDir(), "cancel.zip")
	if err := s.Download(ctx, c, dest, nil); err == nil {
		t.Fatal("accepted canceled request")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("created canceled output")
	}
	for _, payload := range []string{"payloaX", "payload plus bytes", "short"} {
		s.client = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, ContentLength: -1, Body: io.NopCloser(strings.NewReader(payload)), Header: make(http.Header), Request: r}, nil
		})}
		if err := s.Download(context.Background(), c, dest, nil); err == nil {
			t.Fatal("accepted corrupt response")
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Fatal("left corrupt output")
		}
	}
}

func TestFetchRejectsMissingSignatureAndOversizedResponse(t *testing.T) {
	s, _ := fixtureSource(t, []byte("payload"), `[{"tag_name":"v1.2.0"}]`)
	original := s.client.Transport
	s.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/"+SignatureName) {
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), Request: r}, nil
		}
		return original.RoundTrip(r)
	})
	if _, err := s.Fetch(context.Background(), "1.0.0"); err == nil {
		t.Fatal("accepted missing signature")
	}
	s.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: maxManifestBytes + 1, Body: io.NopCloser(strings.NewReader("[]")), Header: make(http.Header), Request: r}, nil
	})
	if _, err := s.Fetch(context.Background(), "1.0.0"); err == nil {
		t.Fatal("accepted oversized response")
	}
}
