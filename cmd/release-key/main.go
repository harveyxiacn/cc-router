// release-key provisions a maintainer key locally and exports only its public half.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"github.com/harveyxiacn/cc-router/internal/releasekey"
	"os"
	"strings"
)

func main() {
	path := "internal/update/release-public-key.txt"
	old, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "run from the repository root")
		os.Exit(1)
	}
	key, err := releasekey.Key(strings.TrimSpace(string(old)) == "")
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot access maintainer signing key:", err)
		os.Exit(1)
	}
	public := base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	if strings.TrimSpace(string(old)) != "" && strings.TrimSpace(string(old)) != public {
		fmt.Fprintln(os.Stderr, "existing public key differs; explicit key rotation required")
		os.Exit(1)
	}
	if err := os.WriteFile(path, []byte(public+"\n"), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Public release key written; private key remains in the maintainer's protected configuration directory.")
}
