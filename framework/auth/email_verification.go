package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
	gostackHTTP "github.com/charledeon77/gostack-framework/framework/http"
	"github.com/charledeon77/gostack-framework/framework/http/urlsign"
)

// hashUserEmail produces a salted hash of the user email bound to their user ID (F-060).
func hashUserEmail(userID any, email string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%v:%s", userID, email)))
	return hex.EncodeToString(sum[:])
}

// MustVerifyEmail represents a user model contract capable of email verification (PAR-04).
type MustVerifyEmail interface {
	contract.Authenticatable
	HasVerifiedEmail() bool
	MarkEmailAsVerified() error
}

// EmailVerificationManager generates and validates signed email verification URLs.
type EmailVerificationManager struct {
	signer *urlsign.URLSigner
	ttl    time.Duration
}

// NewEmailVerificationManager creates an email verification manager.
func NewEmailVerificationManager(signer *urlsign.URLSigner, ttl time.Duration) *EmailVerificationManager {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &EmailVerificationManager{
		signer: signer,
		ttl:    ttl,
	}
}

// GenerateVerificationURL generates a signed HMAC-SHA256 URL for verifying email.
func (m *EmailVerificationManager) GenerateVerificationURL(baseURL string, userID any, email string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("id", fmt.Sprintf("%v", userID))
	q.Set("hash", hashUserEmail(userID, email))
	u.RawQuery = q.Encode()

	return m.signer.TemporarySign(u.String(), m.ttl)
}

// VerifyVerificationURL validates the signature and expiration of the verification URL.
func (m *EmailVerificationManager) VerifyVerificationURL(rawURL string, user MustVerifyEmail) (bool, error) {
	if err := m.signer.Verify(rawURL); err != nil {
		return false, fmt.Errorf("invalid or expired verification signature: %w", err)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false, err
	}
	id := u.Query().Get("id")
	emailHash := u.Query().Get("hash")

	// F-060 & F-061: use salted hash and constant-time comparison to prevent timing attacks
	expectedHash := hashUserEmail(user.GetID(), user.GetEmail())
	if id != fmt.Sprintf("%v", user.GetID()) || subtle.ConstantTimeCompare([]byte(emailHash), []byte(expectedHash)) != 1 {
		return false, errors.New("verification parameters do not match user")
	}

	return true, nil
}

// EmailVerificationMiddleware enforces that authenticated users have verified their email.
func EmailVerificationMiddleware(redirectURL string) gostackHTTP.Middleware {
	return func(ctx *gostackHTTP.Context, next gostackHTTP.NextHandler) error {
		cacheVal := ctx.Request.Context().Value(contract.UserContextKey)
		if cacheVal == nil {
			return ctx.JSON(http.StatusUnauthorized, map[string]string{
				"error": "Authentication required.",
			})
		}
		cache, ok := cacheVal.(*contract.AuthCache)
		if !ok || !cache.Loaded || cache.User == nil {
			return ctx.JSON(http.StatusUnauthorized, map[string]string{
				"error": "Authentication required.",
			})
		}

		if vUser, isV := cache.User.(MustVerifyEmail); isV && !vUser.HasVerifiedEmail() {
			accept := ctx.Request.Header.Get("Accept")
			if strings.Contains(accept, "application/json") {
				return ctx.JSON(http.StatusForbidden, map[string]string{
					"error": "Your email address is not verified.",
				})
			}
			if redirectURL != "" {
				ctx.Writer.Header().Set("Location", redirectURL)
				ctx.Writer.WriteHeader(http.StatusFound)
				return nil
			}
			return ctx.JSON(http.StatusForbidden, map[string]string{
				"error": "Your email address is not verified.",
			})
		}

		return next(ctx)
	}
}
