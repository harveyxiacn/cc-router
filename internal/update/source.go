package update

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

const maxManifestBytes = 1 << 20
const maxArchiveBytes int64 = 128 << 20

// Source has no external endpoint or key configuration. Production requests use
// the public repository and its embedded verification key, without credentials.
type Source struct {
	client   *http.Client
	key      ed25519.PublicKey
	keyError error
}

func NewSource() *Source {
	key, err := TrustedPublicKey()
	return &Source{client: &http.Client{Timeout: 2 * time.Minute}, key: key, keyError: err}
}

func (s *Source) httpClient() *http.Client {
	client := *s.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 8 {
			return errors.New("too many update redirects")
		}
		if !trustedURL(req.URL) {
			return errors.New("untrusted update redirect")
		}
		return nil
	}
	return &client
}
func trustedURL(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" && u.Port() != "443" {
		return false
	}
	switch u.Hostname() {
	case "github.com", "api.github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	}
	return false
}
func (s *Source) request(ctx context.Context, address string) (*http.Response, error) {
	u, err := url.Parse(address)
	if err != nil || !trustedURL(u) {
		return nil, errors.New("untrusted update URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, errors.New("cannot prepare update request")
	}
	req.Header.Set("User-Agent", "cc-router-update")
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := s.httpClient().Do(req)
	if err != nil {
		return nil, errors.New("update request failed; check network access to GitHub")
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("GitHub update request returned HTTP %d", res.StatusCode)
	}
	return res, nil
}
func (s *Source) get(ctx context.Context, address string, limit int64) ([]byte, error) {
	res, err := s.request(ctx, address)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.ContentLength > limit {
		return nil, errors.New("update response exceeds size limit")
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, errors.New("cannot read update response")
	}
	if int64(len(b)) > limit {
		return nil, errors.New("update response exceeds size limit")
	}
	return b, nil
}
func releaseAssetURL(version, name string) string {
	return "https://github.com/" + Repository + "/releases/download/v" + url.PathEscape(version) + "/" + url.PathEscape(name)
}

func (s *Source) Fetch(ctx context.Context, currentVersion string) (*Candidate, error) {
	if s.keyError != nil || len(s.key) != ed25519.PublicKeySize {
		return nil, errors.New("update verification key is unavailable")
	}
	current, err := parseVersion(currentVersion)
	if err != nil {
		return nil, err
	}
	b, err := s.get(ctx, "https://api.github.com/repos/"+Repository+"/releases?per_page=100", maxManifestBytes)
	if err != nil {
		return nil, err
	}
	var releases []struct {
		Tag        string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
	}
	if err = json.Unmarshal(b, &releases); err != nil || len(releases) > 100 {
		return nil, errors.New("invalid GitHub releases response")
	}
	latest := ""
	best := current
	for _, r := range releases {
		if r.Draft || !strings.HasPrefix(r.Tag, "v") {
			continue
		}
		v, err := parseVersion(r.Tag)
		if err != nil {
			continue
		}
		if len(current.pre) == 0 && (r.Prerelease || len(v.pre) > 0) {
			continue
		}
		if compareVersions(v, best) > 0 {
			best = v
			latest = strings.TrimPrefix(r.Tag, "v")
		}
	}
	if latest == "" {
		return nil, nil
	}
	manifestBytes, err := s.get(ctx, releaseAssetURL(latest, ManifestName), maxManifestBytes)
	if err != nil {
		return nil, err
	}
	signatureFile, err := s.get(ctx, releaseAssetURL(latest, SignatureName), 1024)
	if err != nil {
		return nil, err
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(string(signatureFile)))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return nil, errors.New("invalid detached update signature")
	}
	m, err := VerifyManifest(manifestBytes, signature, s.key, time.Now())
	if err != nil {
		return nil, err
	}
	if m.Version != latest {
		return nil, errors.New("update manifest does not match release tag")
	}
	for _, a := range m.Assets {
		if a.OS == runtime.GOOS && a.Arch == runtime.GOARCH {
			return &Candidate{Manifest: m, Asset: a, ManifestBytes: manifestBytes, Signature: signature}, nil
		}
	}
	return nil, errors.New("signed release has no update for this platform")
}

// VerifyManifest verifies the exact downloaded bytes before interpreting JSON.
// Signature is the raw 64-byte Ed25519 signature, not its base64 representation.
func VerifyManifest(data, signature []byte, key ed25519.PublicKey, now time.Time) (Manifest, error) {
	var m Manifest
	if len(data) == 0 || len(data) > maxManifestBytes || len(key) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize || !ed25519.Verify(key, data, signature) {
		return m, errors.New("update manifest signature verification failed")
	}
	if err := rejectDuplicateKeys(data); err != nil {
		return m, errors.New("invalid update manifest JSON")
	}
	if err := exactManifestFields(data); err != nil {
		return m, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, errors.New("invalid update manifest schema")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return m, errors.New("invalid trailing update manifest data")
	}
	if m.Schema != 1 {
		return m, errors.New("unsupported update manifest schema")
	}
	if _, err := parseVersion(m.Version); err != nil || strings.HasPrefix(m.Version, "v") {
		return m, errors.New("invalid update manifest version")
	}
	if m.PublishedAt.IsZero() || m.ExpiresAt.IsZero() || m.PublishedAt.After(now.Add(5*time.Minute)) || !m.ExpiresAt.After(now) || !m.ExpiresAt.After(m.PublishedAt) {
		return m, errors.New("update manifest is expired or has invalid timestamps")
	}
	if len(m.Notes) > 64<<10 || len(m.Assets) == 0 || len(m.Assets) > 32 {
		return m, errors.New("invalid update manifest contents")
	}
	seen := map[string]bool{}
	for _, a := range m.Assets {
		if err := validateAsset(a); err != nil {
			return m, err
		}
		platform := a.OS + "/" + a.Arch
		if seen[platform] {
			return m, errors.New("duplicate update platform")
		}
		seen[platform] = true
	}
	return m, nil
}
func validateAsset(a Asset) error {
	if a.OS != "windows" && a.OS != "linux" && a.OS != "darwin" || a.Arch != "amd64" && a.Arch != "arm64" {
		return errors.New("unsupported update platform")
	}
	if a.Size <= 0 || a.Size > maxArchiveBytes {
		return errors.New("invalid update archive size")
	}
	if len(a.Name) == 0 || len(a.Name) > 180 || strings.ContainsAny(a.Name, "/\\:") || strings.HasPrefix(a.Name, ".") {
		return errors.New("invalid update archive name")
	}
	for _, c := range a.Name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_') {
			return errors.New("invalid update archive name")
		}
	}
	if !strings.HasSuffix(a.Name, ".zip") && !strings.HasSuffix(a.Name, ".tar.gz") {
		return errors.New("unsupported update archive format")
	}
	h, err := hex.DecodeString(a.SHA256)
	if err != nil || len(h) != sha256.Size || a.SHA256 != strings.ToLower(a.SHA256) {
		return errors.New("invalid update archive hash")
	}
	gui, err := safeArchivePath(a.GUI)
	if err != nil || gui != a.GUI || gui == "." {
		return errors.New("invalid GUI entrypoint")
	}
	cli, err := safeArchivePath(a.CLI)
	if err != nil || cli != a.CLI || cli == "." || strings.EqualFold(gui, cli) {
		return errors.New("invalid CLI entrypoint")
	}
	if a.OS == "darwin" {
		gp := strings.Split(gui, "/")
		cp := strings.Split(cli, "/")
		if len(gp) != 4 || len(cp) != 4 || gp[0] != cp[0] || !strings.HasSuffix(gp[0], ".app") || gp[1] != "Contents" || gp[2] != "MacOS" || cp[1] != "Contents" || cp[2] != "MacOS" {
			return errors.New("invalid macOS app entrypoints")
		}
	} else if strings.Contains(gui, "/") || strings.Contains(cli, "/") {
		return errors.New("entrypoints must be at archive root")
	}
	return nil
}

func exactManifestFields(data []byte) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return errors.New("invalid update manifest object")
	}
	allowed := map[string]bool{"schema": true, "version": true, "publishedAt": true, "expiresAt": true, "notes": true, "assets": true}
	for key, value := range root {
		if !allowed[key] || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("invalid update manifest field")
		}
	}
	for _, key := range []string{"schema", "version", "publishedAt", "expiresAt", "assets"} {
		if _, ok := root[key]; !ok {
			return errors.New("missing update manifest field")
		}
	}
	var assets []map[string]json.RawMessage
	if err := json.Unmarshal(root["assets"], &assets); err != nil {
		return errors.New("invalid update assets")
	}
	assetKeys := map[string]bool{"os": true, "arch": true, "name": true, "size": true, "sha256": true, "gui": true, "cli": true}
	for _, asset := range assets {
		if len(asset) != len(assetKeys) {
			return errors.New("invalid update asset fields")
		}
		for key, value := range asset {
			if !assetKeys[key] || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return errors.New("invalid update asset field")
			}
		}
	}
	return nil
}

func rejectDuplicateKeys(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 32 {
			return errors.New("JSON too deeply nested")
		}
		tok, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			keys := map[string]bool{}
			for d.More() {
				t, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := t.(string)
				if !ok || keys[key] {
					return errors.New("duplicate JSON key")
				}
				keys[key] = true
				if err = walk(depth + 1); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return errors.New("invalid JSON object")
			}
		case '[':
			for d.More() {
				if err = walk(depth + 1); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') {
				return errors.New("invalid JSON array")
			}
		default:
			return errors.New("invalid JSON")
		}
		return nil
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func (s *Source) Download(ctx context.Context, candidate *Candidate, dest string, progress func(received, total int64)) (result error) {
	if candidate == nil {
		return errors.New("missing update candidate")
	}
	if s.keyError != nil {
		return errors.New("update verification key is unavailable")
	}
	m, err := VerifyManifest(candidate.ManifestBytes, candidate.Signature, s.key, time.Now())
	if err != nil {
		return err
	}
	var verified bool
	for _, a := range m.Assets {
		if a == candidate.Asset && a.OS == runtime.GOOS && a.Arch == runtime.GOARCH {
			verified = true
			break
		}
	}
	if !verified || m.Version != candidate.Manifest.Version {
		return errors.New("update candidate differs from signed manifest")
	}
	a := candidate.Asset
	res, err := s.request(ctx, releaseAssetURL(m.Version, a.Name))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.ContentLength >= 0 && res.ContentLength != a.Size {
		return errors.New("update download size does not match signed manifest")
	}
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("cannot create update download destination")
	}
	defer func() {
		f.Close()
		if result != nil {
			os.Remove(dest)
		}
	}()
	h := sha256.New()
	writer := io.MultiWriter(f, h)
	var received int64
	buf := make([]byte, 32<<10)
	reader := io.LimitReader(res.Body, a.Size+1)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			received += int64(n)
			if received > a.Size {
				return errors.New("update download exceeds signed size")
			}
			if _, err = writer.Write(buf[:n]); err != nil {
				return errors.New("cannot write update download")
			}
			if progress != nil {
				progress(received, a.Size)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return errors.New("update download interrupted")
		}
	}
	if received != a.Size || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		return errors.New("update download size or SHA256 verification failed")
	}
	if err = f.Sync(); err != nil {
		return errors.New("cannot sync update download")
	}
	if err = f.Close(); err != nil {
		return errors.New("cannot finish update download")
	}
	return nil
}
