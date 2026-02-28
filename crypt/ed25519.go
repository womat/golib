package crypt

import (
	"bytes"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mikesmitty/edkey"
	"golang.org/x/crypto/ssh"
)

import (
	"golang.org/x/crypto/ed25519"
)

// GenerateEd25519KeyFiles generates an Ed25519 private/public key pair and writes them to dir/filename
// and dir/filename.pub. If dir is empty, os.TempDir() is used. If filename is empty, "id_ed25519" is used.
// Returns the fully qualified path to the public key file.
// Source: https://github.com/mikesmitty/edkey/blob/master/edkey.go
func GenerateEd25519KeyFiles(dir string, filename string) (string, error) {

	if dir == "" {
		dir = os.TempDir()
	}

	if filename == "" {
		filename = "id_ed25519"
	}

	privFileName := filepath.Join(filepath.ToSlash(dir), filename)
	pubFileName := privFileName + ".pub"

	// Refuse to overwrite existing files
	for _, f := range []string{privFileName, pubFileName} {
		if fi, err := os.Stat(f); err == nil && !fi.IsDir() {
			return "", fmt.Errorf("file already exists: %s", f)
		}
	}

	// Generate a new private/public keypair for OpenSSH
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}

	publicKey, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return "", fmt.Errorf("create public key: %w", err)
	}

	// Encode private key as PEM
	pemKey := &pem.Block{
		Type:  "OPENSSH PRIVATE KEY",
		Bytes: edkey.MarshalED25519PrivateKey(privKey),
	}
	privateKey := pem.EncodeToMemory(pemKey)

	// Encode public key with timestamp
	authorizedKey := ssh.MarshalAuthorizedKey(publicKey)
	b := bytes.NewBuffer(authorizedKey[:len(authorizedKey)-1])
	b.WriteString(" " + time.Now().Format(time.RFC3339))
	b.WriteByte('\n')

	// Write private key
	if err = os.WriteFile(privFileName, privateKey, 0600); err != nil {
		return "", fmt.Errorf("write private key: %w", err)
	}

	// Write public key, cleanup private key on failure
	if err = os.WriteFile(pubFileName, b.Bytes(), 0644); err != nil {
		_ = os.Remove(privFileName)
		return "", fmt.Errorf("write public key: %w", err)
	}

	return pubFileName, nil
}
