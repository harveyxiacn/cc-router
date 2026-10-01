package update

import (
	"crypto/ed25519"
	_ "embed"
	"encoding/base64"
	"errors"
	"strings"
)

//go:embed release-public-key.txt
var releasePublicKey string

func TrustedPublicKey() (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(releasePublicKey))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("release verification key is not configured")
	}
	return ed25519.PublicKey(b), nil
}
