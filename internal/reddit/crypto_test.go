package reddit

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
)

func generateTestKey() string {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	return hex.EncodeToString(key)
}

func TestEncryptDecrypt(t *testing.T) {
	// Setup test environment
	originalKey := os.Getenv("BACKEND_API_ENCRYPTION_KEY_HEX")
	defer os.Setenv("BACKEND_API_ENCRYPTION_KEY_HEX", originalKey)

	testKeyHex := generateTestKey()
	os.Setenv("BACKEND_API_ENCRYPTION_KEY_HEX", testKeyHex)

	plaintext := "12345678-abcd-defg-this-is-a-test-token"

	// Test Encryption
	encryptedHex, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	if encryptedHex == plaintext {
		t.Fatalf("Encrypted text matches plaintext")
	}

	if encryptedHex == "" {
		t.Fatalf("Encrypted text is empty")
	}

	// Test Decryption
	decryptedText, err := Decrypt(encryptedHex)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if decryptedText != plaintext {
		t.Fatalf("Decrypted text %q does not match original %q", decryptedText, plaintext)
	}
}

func TestEncryptionKeyMissing(t *testing.T) {
	originalKey := os.Getenv("BACKEND_API_ENCRYPTION_KEY_HEX")
	defer os.Setenv("BACKEND_API_ENCRYPTION_KEY_HEX", originalKey)

	os.Unsetenv("BACKEND_API_ENCRYPTION_KEY_HEX")

	_, err := Encrypt("test")
	if err == nil {
		t.Fatal("Expected error when encryption key is missing")
	}

	_, err = Decrypt("test")
	if err == nil {
		t.Fatal("Expected error when encryption key is missing")
	}
}

func TestEncryptionKeyInvalidLength(t *testing.T) {
	originalKey := os.Getenv("BACKEND_API_ENCRYPTION_KEY_HEX")
	defer os.Setenv("BACKEND_API_ENCRYPTION_KEY_HEX", originalKey)

	// Set invalid key (16 bytes instead of 32)
	os.Setenv("BACKEND_API_ENCRYPTION_KEY_HEX", "0123456789abcdef0123456789abcdef")

	_, err := Encrypt("test")
	if err == nil {
		t.Fatal("Expected error when encryption key is wrong length")
	}
}
