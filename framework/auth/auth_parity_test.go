package auth

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/charledeon77/gostack-framework/framework/http/urlsign"
)

// mockVerifiableUser implements both contract.Authenticatable and MustVerifyEmail.
type mockVerifiableUser struct {
	id       string
	email    string
	password string
	verified bool
}

func (m *mockVerifiableUser) GetID() any      { return m.id }
func (m *mockVerifiableUser) GetEmail() string { return m.email }
func (m *mockVerifiableUser) GetPassword() string { return m.password }
func (m *mockVerifiableUser) HasVerifiedEmail() bool { return m.verified }
func (m *mockVerifiableUser) MarkEmailAsVerified() error {
	m.verified = true
	return nil
}

func TestPAR03_PasswordReset(t *testing.T) {
	broker := NewPasswordResetBroker(100*time.Millisecond, 20*time.Millisecond)

	// 1. Create token
	rawToken, err := broker.CreateToken("user@example.com")
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}
	if rawToken == "" {
		t.Fatal("expected non-empty raw token")
	}

	// 2. Stored token must be hashed, not raw
	broker.mu.RLock()
	rec := broker.records["user@example.com"]
	broker.mu.RUnlock()
	if rec.TokenHash == rawToken {
		t.Fatal("security violation: raw token stored instead of hash")
	}

	// 3. Verify token works
	valid, err := broker.VerifyToken("user@example.com", rawToken)
	if err != nil || !valid {
		t.Fatalf("expected token to verify: %v", err)
	}

	// 4. Bad token fails
	valid, err = broker.VerifyToken("user@example.com", "wrong-token")
	if valid || err == nil {
		t.Fatal("expected wrong token to fail")
	}

	// 5. Consume token (single-use)
	valid, err = broker.ConsumeToken("user@example.com", rawToken)
	if err != nil || !valid {
		t.Fatalf("failed to consume token: %v", err)
	}

	// 6. Replay fails
	valid, err = broker.ConsumeToken("user@example.com", rawToken)
	if valid || err == nil {
		t.Fatal("expected consumed token replay to fail")
	}

	// 7. Invalidate tokens on password change
	token2, _ := broker.CreateToken("user2@example.com")
	broker.InvalidateTokens("user2@example.com")
	valid, _ = broker.VerifyToken("user2@example.com", token2)
	if valid {
		t.Fatal("expected invalidated token to fail")
	}
}

func TestPAR04_EmailVerification(t *testing.T) {
	signer := urlsign.NewURLSigner("super-secret-key-for-url-signing")
	mgr := NewEmailVerificationManager(signer, 1*time.Hour)

	user := &mockVerifiableUser{
		id:       "42",
		email:    "jane@example.com",
		verified: false,
	}

	// 1. Generate signed URL
	verifyURL, err := mgr.GenerateVerificationURL("https://example.com/verify-email", user.GetID(), user.GetEmail())
	if err != nil {
		t.Fatalf("failed to generate verification url: %v", err)
	}

	// 2. Validate valid URL
	valid, err := mgr.VerifyVerificationURL(verifyURL, user)
	if err != nil || !valid {
		t.Fatalf("expected verification to pass: %v", err)
	}

	// 3. Tampered URL fails
	tamperedURL := verifyURL + "&tampered=1"
	valid, err = mgr.VerifyVerificationURL(tamperedURL, user)
	if valid || err == nil {
		t.Fatal("expected tampered URL to fail verification")
	}
}

func TestPAR05_RememberMe(t *testing.T) {
	mgr := NewRememberMeManager("remember_test", 1*time.Hour)
	w := httptest.NewRecorder()

	// 1. Create cookie
	cookieVal, err := mgr.CreateRememberCookie(w, "user-123")
	if err != nil {
		t.Fatalf("failed to create remember cookie: %v", err)
	}

	// 2. Validate and Rotate (Single-Use Token Rotation)
	w2 := httptest.NewRecorder()
	userID, err := mgr.ValidateAndRotate(w2, cookieVal)
	if err != nil || userID != "user-123" {
		t.Fatalf("failed to validate remember cookie: %v", err)
	}

	// 3. Replay with old token must fail (token was rotated)
	w3 := httptest.NewRecorder()
	_, err = mgr.ValidateAndRotate(w3, cookieVal)
	if err == nil {
		t.Fatal("expected replay of old rotated token to fail")
	}

	// 4. RevokeAll clears everything for user
	cookieVal2, _ := mgr.CreateRememberCookie(httptest.NewRecorder(), "user-456")
	mgr.RevokeAll("user-456")
	_, err = mgr.ValidateAndRotate(httptest.NewRecorder(), cookieVal2)
	if err == nil {
		t.Fatal("expected revoked token to fail")
	}
}
