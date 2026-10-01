// Package update implements signed portable desktop updates without handling account data.
package update

import "time"

const Repository = "harveyxiacn/cc-router"
const ReleasePage = "https://github.com/" + Repository + "/releases"
const ManifestName = "cc-router-update.json"
const SignatureName = "cc-router-update.sig"

type Manifest struct {
	Schema      int       `json:"schema"`
	Version     string    `json:"version"`
	PublishedAt time.Time `json:"publishedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Notes       string    `json:"notes"`
	Assets      []Asset   `json:"assets"`
}
type Asset struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	GUI    string `json:"gui"`
	CLI    string `json:"cli"`
}
type Candidate struct {
	Manifest      Manifest
	Asset         Asset
	ManifestBytes []byte
	Signature     []byte
}
type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   uint32 `json:"mode"`
}

type Status struct {
	CurrentVersion  string  `json:"currentVersion"`
	LatestVersion   string  `json:"latestVersion"`
	ReleaseURL      string  `json:"releaseURL"`
	Notes           string  `json:"notes"`
	CheckedAt       string  `json:"checkedAt"`
	UpdateAvailable bool    `json:"updateAvailable"`
	Prerelease      bool    `json:"prerelease"`
	AutoUpdate      bool    `json:"autoUpdate"`
	Phase           string  `json:"phase"`
	Progress        float64 `json:"progress"`
	Message         string  `json:"message"`
	Error           string  `json:"error"`
	CanRollback     bool    `json:"canRollback"`
	RollbackVersion string  `json:"rollbackVersion"`
}
