package keystore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/argon2"
)

const (
	argon2Time    = 3
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4
	keyLen        = 32 // AES-256
	saltLen       = 32
	nonceLen      = 12 // AES-GCM standard
)

type File struct {
	Version    int       `json:"version"`
	Algorithm  string    `json:"algorithm"`
	KDF        string    `json:"kdf"`
	KDFParams  KDFParams `json:"kdf_params"`
	Salt       string    `json:"salt"`       // hex
	Nonce      string    `json:"nonce"`      // hex
	Ciphertext string    `json:"ciphertext"` // hex (includes GCM auth tag)
	Address    string    `json:"address"`    // wallet address for verification
}

type KDFParams struct {
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
	KeyLen  uint32 `json:"key_len"`
}

func Encrypt(privateKeyHex string, passphrase []byte, path string) error {
	keyBytes, err := hex.DecodeString(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil || len(keyBytes) != 32 {
		return fmt.Errorf("invalid private key: must be 32 bytes hex")
	}

	ecKey, err := crypto.ToECDSA(keyBytes)
	if err != nil {
		return fmt.Errorf("invalid private key: %w", err)
	}
	address := crypto.PubkeyToAddress(ecKey.PublicKey).Hex()

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("generate salt: %w", err)
	}

	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	derived := argon2.IDKey(passphrase, salt, argon2Time, argon2Memory, argon2Threads, keyLen)
	defer zeroBytes(derived)
	defer zeroBytes(keyBytes)

	block, err := aes.NewCipher(derived)
	if err != nil {
		return fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("create GCM: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, keyBytes, nil)

	f := File{
		Version:   1,
		Algorithm: "aes-256-gcm",
		KDF:       "argon2id",
		KDFParams: KDFParams{
			Time:    argon2Time,
			Memory:  argon2Memory,
			Threads: argon2Threads,
			KeyLen:  keyLen,
		},
		Salt:       hex.EncodeToString(salt),
		Nonce:      hex.EncodeToString(nonce),
		Ciphertext: hex.EncodeToString(ciphertext),
		Address:    address,
	}

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal keystore: %w", err)
	}

	return atomicWriteFile(path, data, 0600)
}

func Decrypt(path string, passphrase []byte) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read keystore: %w", err)
	}

	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return "", "", fmt.Errorf("invalid keystore format: %w", err)
	}

	if f.Version != 1 {
		return "", "", fmt.Errorf("unsupported keystore version: %d", f.Version)
	}

	salt, err := hex.DecodeString(f.Salt)
	if err != nil {
		return "", "", fmt.Errorf("decode salt: %w", err)
	}

	nonce, err := hex.DecodeString(f.Nonce)
	if err != nil {
		return "", "", fmt.Errorf("decode nonce: %w", err)
	}

	ciphertext, err := hex.DecodeString(f.Ciphertext)
	if err != nil {
		return "", "", fmt.Errorf("decode ciphertext: %w", err)
	}

	derived := argon2.IDKey(passphrase, salt, f.KDFParams.Time, f.KDFParams.Memory, f.KDFParams.Threads, f.KDFParams.KeyLen)
	defer zeroBytes(derived)

	block, err := aes.NewCipher(derived)
	if err != nil {
		return "", "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", fmt.Errorf("create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", "", fmt.Errorf("decryption failed (wrong passphrase?)")
	}
	defer zeroBytes(plaintext)

	ecKey, err := crypto.ToECDSA(plaintext)
	if err != nil {
		return "", "", fmt.Errorf("decrypted data is not a valid key")
	}

	address := crypto.PubkeyToAddress(ecKey.PublicKey).Hex()
	if address != f.Address {
		return "", "", fmt.Errorf("address mismatch after decryption")
	}

	return hex.EncodeToString(plaintext), address, nil
}

func ReadAddress(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return "", fmt.Errorf("invalid keystore: %w", err)
	}

	return f.Address, nil
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, path)
}
