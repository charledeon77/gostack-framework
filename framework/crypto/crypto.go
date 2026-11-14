/*
Purpose:
Package crypto provides production-grade AES-256-GCM envelope encryption for GoStack.
It encrypts and decrypts arbitrary strings and byte slices using the application key (APP_KEY).
The payload is encoded as base64 containing the nonce and ciphertext, verified with an authentication tag.

Philosophy:
Sensitive data (API tokens, PII, secrets) stored in databases or sent across distrusted channels
must be encrypted at rest using modern authenticated encryption with associated data (AEAD).
Zero-dependency, utilizing standard library crypto/aes and crypto/cipher.
*/
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrInvalidKey        = errors.New("crypto: encryption key must be 32 bytes (AES-256)")
	ErrCiphertextTooShort = errors.New("crypto: ciphertext payload is too short")
	ErrDecryptionFailed  = errors.New("crypto: message authentication failed or corrupt ciphertext")
)

// Encrypter manages encryption and decryption operations with a fixed key.
type Encrypter struct {
	key []byte
}

// NewEncrypter creates an Encrypter with a 32-byte AES-256 key.
// The key can be passed as raw 32 bytes, a 32-char string, or a "base64:" prefixed string.
func NewEncrypter(keyStr string) (*Encrypter, error) {
	var key []byte
	if strings.HasPrefix(keyStr, "base64:") {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(keyStr, "base64:"))
		if err != nil {
			return nil, fmt.Errorf("crypto: invalid base64 key: %w", err)
		}
		key = decoded
	} else {
		key = []byte(keyStr)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("%w: got %d bytes", ErrInvalidKey, len(key))
	}

	return &Encrypter{key: key}, nil
}

// Encrypt encrypts a plaintext string and returns a base64 encoded ciphertext string.
func (e *Encrypter) Encrypt(plaintext string) (string, error) {
	encBytes, err := e.EncryptBytes([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encBytes), nil
}

// EncryptBytes encrypts raw plaintext bytes using AES-256-GCM.
// Prepend the 12-byte GCM nonce to the ciphertext.
func (e *Encrypter) EncryptBytes(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("crypto: failed to init cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: failed to init GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: failed to generate nonce: %w", err)
	}

	// Seal appends the ciphertext and tag to nonce
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decodes and decrypts a base64 encoded ciphertext string.
func (e *Encrypter) Decrypt(ciphertextBase64 string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return "", fmt.Errorf("crypto: invalid base64 payload: %w", err)
	}

	plainBytes, err := e.DecryptBytes(data)
	if err != nil {
		return "", err
	}

	return string(plainBytes), nil
}

// DecryptBytes decrypts raw bytes containing a prepended nonce and GCM ciphertext.
func (e *Encrypter) DecryptBytes(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("crypto: failed to init cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: failed to init GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrCiphertextTooShort
	}

	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

