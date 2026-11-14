package crypto

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestEncrypter_EncryptDecrypt(t *testing.T) {
	key := "12345678901234567890123456789012" // 32 bytes
	enc, err := NewEncrypter(key)
	if err != nil {
		t.Fatalf("NewEncrypter failed: %v", err)
	}

	secret := "Hello, GoStack Enterprise Security!"
	encrypted, err := enc.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == secret {
		t.Fatal("encrypted text matches secret directly")
	}

	decrypted, err := enc.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != secret {
		t.Fatalf("expected %q, got %q", secret, decrypted)
	}
}

func TestEncrypter_Base64Key(t *testing.T) {
	rawKey := make([]byte, 32)
	for i := range rawKey {
		rawKey[i] = byte(i + 1)
	}
	b64Key := "base64:" + base64.StdEncoding.EncodeToString(rawKey)

	enc, err := NewEncrypter(b64Key)
	if err != nil {
		t.Fatalf("NewEncrypter with base64 failed: %v", err)
	}

	msg := "test-message"
	ciphertext, err := enc.Encrypt(msg)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	res, err := enc.Decrypt(ciphertext)
	if err != nil || res != msg {
		t.Fatalf("Decrypt: %v, got: %s", err, res)
	}
}

func TestEncrypter_TamperFails(t *testing.T) {
	key := "12345678901234567890123456789012"
	enc, _ := NewEncrypter(key)

	ciphertext, _ := enc.Encrypt("my-secret-data")
	rawBytes, _ := base64.StdEncoding.DecodeString(ciphertext)

	// Tamper with the last byte (auth tag)
	rawBytes[len(rawBytes)-1] ^= 0xFF
	tamperedB64 := base64.StdEncoding.EncodeToString(rawBytes)

	_, err := enc.Decrypt(tamperedB64)
	if err == nil {
		t.Fatal("expected decryption of tampered ciphertext to fail, got nil")
	}
}

func TestEncrypter_InvalidKeyLength(t *testing.T) {
	_, err := NewEncrypter("too-short")
	if err == nil {
		t.Fatal("expected error for short key, got nil")
	}
	if !strings.Contains(err.Error(), "must be 32 bytes") {
		t.Errorf("unexpected error: %v", err)
	}
}

