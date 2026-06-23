package data

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/gotd/td/session"
)

// encryptedFileStorage implements session.Storage with AES-256-GCM encryption.
// File layout: [12-byte random nonce][ciphertext+16-byte auth tag].
// Each write generates a fresh nonce, so the on-disk blob is useless without the key.
type encryptedFileStorage struct {
	path string
	key  [32]byte
	mu   sync.Mutex
}

func (s *encryptedFileStorage) LoadSession(_ context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil, session.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.decrypt(data)
}

func (s *encryptedFileStorage) StoreSession(_ context.Context, plaintext []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ciphertext, err := s.encrypt(plaintext)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, ciphertext, 0600)
}

func (s *encryptedFileStorage) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (s *encryptedFileStorage) decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("session file too short to contain nonce")
	}
	return gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
}

// NewSessionStorage returns the same Telegram session storage the userbot uses
// at runtime: AES-256-GCM-encrypted when TG_SESSION_KEY is set, plain otherwise.
// It exists so the one-shot login command (cmd/tg-login) writes a session in the
// exact on-disk format the app later reads — encryption stays single-sourced here.
func NewSessionStorage(path string) (session.Storage, error) {
	return sessionStorageFromEnv(path)
}

// sessionStorageFromEnv returns an encryptedFileStorage when TG_SESSION_KEY is
// set (base64-encoded 32-byte AES key), or a plain session.FileStorage when the
// env var is absent — backward-compatible for local dev and CI.
func sessionStorageFromEnv(path string) (session.Storage, error) {
	keyStr := os.Getenv("TG_SESSION_KEY")
	if keyStr == "" {
		return &session.FileStorage{Path: path}, nil
	}
	keyBytes, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		return nil, fmt.Errorf("TG_SESSION_KEY: invalid base64: %w", err)
	}
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("TG_SESSION_KEY: must decode to exactly 32 bytes, got %d", len(keyBytes))
	}
	var key [32]byte
	copy(key[:], keyBytes)
	return &encryptedFileStorage{path: path, key: key}, nil
}
