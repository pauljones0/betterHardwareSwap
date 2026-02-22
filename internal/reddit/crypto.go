package reddit

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

// getEncryptionKey returns the AES 256 key from the environment.
func getEncryptionKey() ([]byte, error) {
	keyHex := os.Getenv("BACKEND_API_ENCRYPTION_KEY_HEX")
	if keyHex == "" {
		return nil, errors.New("BACKEND_API_ENCRYPTION_KEY_HEX is not set")
	}

	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid BACKEND_API_ENCRYPTION_KEY_HEX: %w", err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("BACKEND_API_ENCRYPTION_KEY_HEX must be exactly 64 hex characters (32 bytes), got %d bytes", len(key))
	}

	return key, nil
}

// Encrypt encrypts a plaintext string using AES-GCM and the environment's encryption key.
// It returns a hex-encoded string containing the IV + ciphertext.
func Encrypt(plaintext string) (string, error) {
	key, err := getEncryptionKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	// Never use more than 2^32 random nonces with a given key because of the risk of a repeat.
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	ciphertext := aesgcm.Seal(nil, nonce, []byte(plaintext), nil)
	
	// Prepend nonce to ciphertext
	encryptedBytes := append(nonce, ciphertext...)
	return hex.EncodeToString(encryptedBytes), nil
}

// Decrypt decrypts a hex-encoded cipher string (IV + ciphertext) using the environment's encryption key.
func Decrypt(cryptoTextHex string) (string, error) {
	key, err := getEncryptionKey()
	if err != nil {
		return "", err
	}

	encryptedBytes, err := hex.DecodeString(cryptoTextHex)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	if len(encryptedBytes) < aesgcm.NonceSize() {
		return "", errors.New("malformed ciphertext")
	}

	nonce, ciphertext := encryptedBytes[:aesgcm.NonceSize()], encryptedBytes[aesgcm.NonceSize():]
	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
