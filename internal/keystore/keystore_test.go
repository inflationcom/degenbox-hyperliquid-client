package keystore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Well-known test private key (Hardhat account #0)
const testPrivateKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
const testAddress = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"

func TestEncryptDecryptRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".keystore.json")
	passphrase := []byte("test-passphrase-123")

	err := Encrypt(testPrivateKey, passphrase, path)
	require.NoError(t, err)

	key, addr, err := Decrypt(path, passphrase)
	require.NoError(t, err)
	assert.Equal(t, testPrivateKey, key)
	assert.Equal(t, testAddress, addr)
}

func TestDecryptWrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".keystore.json")

	err := Encrypt(testPrivateKey, []byte("correct"), path)
	require.NoError(t, err)

	_, _, err = Decrypt(path, []byte("wrong"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "wrong passphrase")
}

func TestDecryptCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".keystore.json")

	err := os.WriteFile(path, []byte("not json"), 0600)
	require.NoError(t, err)

	_, _, err = Decrypt(path, []byte("anything"))
	assert.Error(t, err)
}

func TestEncryptFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".keystore.json")

	err := Encrypt(testPrivateKey, []byte("pass"), path)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestReadAddress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".keystore.json")

	err := Encrypt(testPrivateKey, []byte("pass"), path)
	require.NoError(t, err)

	addr, err := ReadAddress(path)
	require.NoError(t, err)
	assert.Equal(t, testAddress, addr)
}

func TestEncryptInvalidKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".keystore.json")

	err := Encrypt("not-hex", []byte("pass"), path)
	assert.Error(t, err)
}
