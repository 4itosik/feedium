package data

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/gotd/td/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptedFileStorage_RoundTrip(t *testing.T) {
	t.Parallel()

	key := fixedKey(t)
	path := filepath.Join(t.TempDir(), "tg.session")
	s := &encryptedFileStorage{path: path, key: key}

	plaintext := []byte(`{"version":1,"data":{"AuthKey":"abc"}}`)

	require.NoError(t, s.StoreSession(context.Background(), plaintext))

	got, err := s.LoadSession(context.Background())
	require.NoError(t, err)
	assert.Equal(t, plaintext, got)
}

func TestEncryptedFileStorage_WrongKey(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "tg.session")
	s1 := &encryptedFileStorage{path: path, key: fixedKey(t)}
	require.NoError(t, s1.StoreSession(context.Background(), []byte(`{"v":1}`)))

	var otherKey [32]byte
	otherKey[0] = 0xFF
	s2 := &encryptedFileStorage{path: path, key: otherKey}
	_, err := s2.LoadSession(context.Background())
	assert.Error(t, err)
}

func TestEncryptedFileStorage_MissingFile(t *testing.T) {
	t.Parallel()

	s := &encryptedFileStorage{
		path: filepath.Join(t.TempDir(), "nonexistent.session"),
		key:  fixedKey(t),
	}
	_, err := s.LoadSession(context.Background())
	assert.ErrorIs(t, err, session.ErrNotFound)
}

func TestEncryptedFileStorage_TruncatedFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "tg.session")
	require.NoError(t, os.WriteFile(path, []byte("short"), 0600))

	s := &encryptedFileStorage{path: path, key: fixedKey(t)}
	_, err := s.LoadSession(context.Background())
	assert.Error(t, err)
}

func TestSessionStorageFromEnv_NoKey(t *testing.T) {
	t.Setenv("TG_SESSION_KEY", "")

	stor, err := sessionStorageFromEnv("/tmp/tg.session")
	require.NoError(t, err)
	_, ok := stor.(*session.FileStorage)
	assert.True(t, ok, "expected plain FileStorage when TG_SESSION_KEY is unset")
}

func TestSessionStorageFromEnv_ValidKey(t *testing.T) {
	key := make([]byte, 32)
	key[0] = 42
	t.Setenv("TG_SESSION_KEY", base64.StdEncoding.EncodeToString(key))

	stor, err := sessionStorageFromEnv("/tmp/tg.session")
	require.NoError(t, err)
	_, ok := stor.(*encryptedFileStorage)
	assert.True(t, ok, "expected encryptedFileStorage when TG_SESSION_KEY is set")
}

func TestSessionStorageFromEnv_InvalidBase64(t *testing.T) {
	t.Setenv("TG_SESSION_KEY", "not-valid-base64!!!")

	_, err := sessionStorageFromEnv("/tmp/tg.session")
	assert.ErrorContains(t, err, "invalid base64")
}

func TestSessionStorageFromEnv_WrongKeyLength(t *testing.T) {
	t.Setenv("TG_SESSION_KEY", base64.StdEncoding.EncodeToString([]byte("tooshort")))

	_, err := sessionStorageFromEnv("/tmp/tg.session")
	assert.ErrorContains(t, err, "32 bytes")
}

func fixedKey(t *testing.T) [32]byte {
	t.Helper()
	var key [32]byte
	for i := range key {
		key[i] = byte(i + 1)
	}
	return key
}
