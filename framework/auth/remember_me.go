package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RememberToken represents a stored persistent login series/token pair (PAR-05).
type RememberToken struct {
	UserID    string    `json:"user_id"`
	Series    string    `json:"series"`
	TokenHash string    `json:"token_hash"`
	ExpiresAt time.Time `json:"expires_at"`
}

// RememberMeManager manages persistent "remember me" cookies and tokens.
type RememberMeManager struct {
	mu         sync.RWMutex
	tokens     map[string]RememberToken // series -> token
	userSeries map[string][]string      // userID -> []series
	cookieName string
	ttl        time.Duration
}

// NewRememberMeManager creates a manager for persistent remember me sessions.
func NewRememberMeManager(cookieName string, ttl time.Duration) *RememberMeManager {
	if cookieName == "" {
		cookieName = "remember_token"
	}
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour // 30 days
	}
	return &RememberMeManager{
		tokens:     make(map[string]RememberToken),
		userSeries: make(map[string][]string),
		cookieName: cookieName,
		ttl:        ttl,
	}
}

// CreateRememberCookie generates a new series and token, stores the hash, and sets the cookie.
func (m *RememberMeManager) CreateRememberCookie(w http.ResponseWriter, userID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	seriesBytes := make([]byte, 16)
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(seriesBytes); err != nil {
		return "", err
	}
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}

	series := hex.EncodeToString(seriesBytes)
	rawToken := hex.EncodeToString(tokenBytes)
	tokenHash := HashToken(rawToken)

	expiresAt := time.Now().Add(m.ttl)
	m.tokens[series] = RememberToken{
		UserID:    userID,
		Series:    series,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	}
	m.userSeries[userID] = append(m.userSeries[userID], series)

	cookieVal := fmt.Sprintf("%s|%s", series, rawToken)
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    cookieVal,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})

	return cookieVal, nil
}

// ValidateAndRotate validates the cookie value, rotates the token (single use), and updates the cookie.
func (m *RememberMeManager) ValidateAndRotate(w http.ResponseWriter, cookieVal string) (string, error) {
	parts := strings.Split(cookieVal, "|")
	if len(parts) != 2 {
		return "", errors.New("malformed remember me cookie")
	}
	series, rawToken := parts[0], parts[1]

	m.mu.Lock()
	defer m.mu.Unlock()

	stored, exists := m.tokens[series]
	if !exists {
		return "", errors.New("remember token series not found")
	}

	if time.Now().After(stored.ExpiresAt) {
		delete(m.tokens, series)
		return "", errors.New("remember token expired")
	}

	if HashToken(rawToken) != stored.TokenHash {
		// Possible token theft or replay attack: invalidate all series for this user
		m.revokeAllUnsafe(stored.UserID)
		return "", errors.New("remember token mismatch (possible replay detected)")
	}

	// Rotate token within the same series
	newTokenBytes := make([]byte, 16)
	if _, err := rand.Read(newTokenBytes); err != nil {
		return "", err
	}
	newRawToken := hex.EncodeToString(newTokenBytes)
	stored.TokenHash = HashToken(newRawToken)
	stored.ExpiresAt = time.Now().Add(m.ttl)
	m.tokens[series] = stored

	newCookieVal := fmt.Sprintf("%s|%s", series, newRawToken)
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    newCookieVal,
		Path:     "/",
		Expires:  stored.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return stored.UserID, nil
}

// Revoke deletes a specific series and clears the cookie.
func (m *RememberMeManager) Revoke(w http.ResponseWriter, cookieVal string) {
	parts := strings.Split(cookieVal, "|")
	if len(parts) == 2 {
		series := parts[0]
		m.mu.Lock()
		delete(m.tokens, series)
		m.mu.Unlock()
	}

	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
	})
}

// RevokeAll revokes all remember tokens for a user (e.g. on logout or password change).
func (m *RememberMeManager) RevokeAll(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revokeAllUnsafe(userID)
}

func (m *RememberMeManager) revokeAllUnsafe(userID string) {
	seriesList := m.userSeries[userID]
	for _, s := range seriesList {
		delete(m.tokens, s)
	}
	delete(m.userSeries, userID)
}

