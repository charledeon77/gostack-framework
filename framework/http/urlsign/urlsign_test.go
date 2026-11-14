package urlsign

import (
	"errors"
	"testing"
	"time"
)

func TestURLSigner_SignAndVerify(t *testing.T) {
	signer := NewURLSigner("super-secret-app-key-123456789012")

	rawURL := "https://example.com/verify-email?id=10"
	signedURL, err := signer.Sign(rawURL)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Verification of unmodified URL must succeed
	if err := signer.Verify(signedURL); err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	// Tampered query param must fail
	tamperedURL := signedURL + "&hacked=true"
	if err := signer.Verify(tamperedURL); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got: %v", err)
	}

	// Altered ID must fail
	alteredURL := "https://example.com/verify-email?id=99" + signedURL[len(rawURL):]
	if err := signer.Verify(alteredURL); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature on altered ID, got: %v", err)
	}
}

func TestURLSigner_TemporarySign_Expiration(t *testing.T) {
	signer := NewURLSigner("secret-key")

	rawURL := "https://example.com/download/report.pdf"
	// Sign with 1 second TTL
	signedURL, err := signer.TemporarySign(rawURL, 1*time.Second)
	if err != nil {
		t.Fatalf("TemporarySign failed: %v", err)
	}

	// Immediate verification should pass
	if err := signer.Verify(signedURL); err != nil {
		t.Fatalf("immediate verify failed: %v", err)
	}

	// Expired URL signed in the past
	pastURL, err := signer.TemporarySignAt(rawURL, time.Now().Add(-10*time.Second))
	if err != nil {
		t.Fatalf("TemporarySignAt: %v", err)
	}

	if err := signer.Verify(pastURL); !errors.Is(err, ErrURLExpired) {
		t.Fatalf("expected ErrURLExpired, got: %v", err)
	}
}
