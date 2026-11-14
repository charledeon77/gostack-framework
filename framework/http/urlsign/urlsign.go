/*
Purpose:
Package urlsign provides HMAC-SHA256 URL signing and verification for GoStack (PAR-02).
It enables generating temporary or permanent signed URLs (e.g. email verification, password reset,
one-time download links) that cannot be altered without invalidating the cryptographic signature.

Philosophy:
Pure standard library (crypto/hmac, crypto/sha256). Works seamlessly with GoStack's HTTP router.
*/
package urlsign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrMissingSignature = errors.New("urlsign: missing signature query parameter")
	ErrInvalidSignature = errors.New("urlsign: URL signature verification failed")
	ErrURLExpired       = errors.New("urlsign: URL has expired")
)

// URLSigner manages URL signature generation and validation.
type URLSigner struct {
	key []byte
}

// NewURLSigner creates a URLSigner with an application secret key.
func NewURLSigner(key string) *URLSigner {
	return &URLSigner{key: []byte(key)}
}

// Sign appends an HMAC signature parameter to the target URL.
func (s *URLSigner) Sign(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("urlsign: parse error: %w", err)
	}

	sig := s.computeSignature(u)
	q := u.Query()
	q.Set("signature", sig)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// TemporarySign appends an expiration timestamp and HMAC signature to the target URL.
func (s *URLSigner) TemporarySign(rawURL string, ttl time.Duration) (string, error) {
	return s.TemporarySignAt(rawURL, time.Now().Add(ttl))
}

// TemporarySignAt appends an explicit expiration timestamp and HMAC signature.
func (s *URLSigner) TemporarySignAt(rawURL string, expiresAt time.Time) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("urlsign: parse error: %w", err)
	}

	q := u.Query()
	q.Set("expires", strconv.FormatInt(expiresAt.Unix(), 10))
	u.RawQuery = q.Encode()

	sig := s.computeSignature(u)
	q.Set("signature", sig)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// Verify validates that the URL has not been tampered with and has not expired.
func (s *URLSigner) Verify(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("urlsign: parse error: %w", err)
	}

	q := u.Query()
	sig := q.Get("signature")
	if sig == "" {
		return ErrMissingSignature
	}

	// Check expiration if present
	if expStr := q.Get("expires"); expStr != "" {
		expUnix, err := strconv.ParseInt(expStr, 10, 64)
		if err != nil {
			return ErrInvalidSignature
		}
		if time.Now().Unix() > expUnix {
			return ErrURLExpired
		}
	}

	// Re-compute expected signature without the "signature" parameter
	expectedSig := s.computeSignature(u)
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return ErrInvalidSignature
	}

	return nil
}

func (s *URLSigner) computeSignature(u *url.URL) string {
	// Copy URL and strip existing signature parameter
	clone := *u
	q := clone.Query()
	q.Del("signature")
	clone.RawQuery = q.Encode()

	payload := clone.RequestURI()
	if clone.Host != "" {
		payload = strings.TrimRight(clone.Scheme, ":") + "://" + clone.Host + clone.RequestURI()
	}

	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
