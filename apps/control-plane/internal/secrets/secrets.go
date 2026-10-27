// Package secrets encrypts values stored at rest (AI provider keys, secret
// environment variables, webhook URLs) with AES-256-GCM.
//
// The key comes from OPENSYNAPSE_SECRET_KEY (32 bytes, base64) — which is how
// the Helm chart supplies it from a Kubernetes Secret — or from a key file in
// the data directory that is created with owner-only permissions on first
// start. See docs/decisions/0011-security-model.md.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const prefix = "enc:v1:"

// Box seals and opens strings.
type Box struct {
	aead cipher.AEAD
}

// New builds a Box from a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secret key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Load returns the installation's Box, creating its key file if needed.
func Load(dataDir string) (*Box, error) {
	if env := os.Getenv("OPENSYNAPSE_SECRET_KEY"); env != "" {
		key, err := base64.StdEncoding.DecodeString(env)
		if err != nil {
			return nil, fmt.Errorf("OPENSYNAPSE_SECRET_KEY is not valid base64: %w", err)
		}
		return New(key)
	}
	path := filepath.Join(dataDir, "secret.key")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
			return nil, fmt.Errorf("write secret key: %w", err)
		}
		return New(key)
	}
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("secret key file %s is corrupt: %w", path, err)
	}
	return New(key)
}

// Seal encrypts plain. The empty string stays empty.
func (b *Box) Seal(plain string) string {
	if b == nil || plain == "" {
		return plain
	}
	nonce := make([]byte, b.aead.NonceSize())
	rand.Read(nonce)
	ct := b.aead.Seal(nonce, nonce, []byte(plain), nil)
	return prefix + base64.StdEncoding.EncodeToString(ct)
}

// Open decrypts a sealed value. Values stored before encryption was
// introduced (no prefix) are returned unchanged so existing data keeps working.
func (b *Box) Open(s string) (string, error) {
	if !strings.HasPrefix(s, prefix) {
		return s, nil
	}
	if b == nil {
		return "", errors.New("encrypted value but no secret key configured")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, prefix))
	if err != nil {
		return "", err
	}
	ns := b.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext too short")
	}
	pt, err := b.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", errors.New("cannot decrypt value: the secret key changed or the data is corrupt")
	}
	return string(pt), nil
}

// IsSealed reports whether s is an encrypted value.
func IsSealed(s string) bool { return strings.HasPrefix(s, prefix) }
