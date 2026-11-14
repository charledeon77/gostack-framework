package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var (
	ErrResetTokenExpired = errors.New("password reset token has expired")
	ErrResetTokenInvalid = errors.New("password reset token is invalid")
	ErrResetThrottled    = errors.New("password reset request throttled, please wait before retrying")
)

// PasswordResetRecord holds metadata for a reset request.
// Only the hashed token is saved (never the raw token).
type PasswordResetRecord struct {
	Identifier string    `json:"identifier"`
	TokenHash  string    `json:"token_hash"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// PasswordResetBroker defines the contract for handling password resets (PAR-03).
type PasswordResetBroker struct {
	mu       sync.RWMutex
	records  map[string]PasswordResetRecord // identifier -> record
	throttle map[string]time.Time           // identifier -> last request time
	ttl      time.Duration
	cooldown time.Duration
}

// NewPasswordResetBroker creates a broker with specified TTL and cooldown duration.
// Defaults: TTL = 60m, Cooldown = 60s.
func NewPasswordResetBroker(ttl, cooldown time.Duration) *PasswordResetBroker {
	if ttl <= 0 {
		ttl = 60 * time.Minute
	}
	if cooldown <= 0 {
		cooldown = 60 * time.Second
	}
	return &PasswordResetBroker{
		records:  make(map[string]PasswordResetRecord),
		throttle: make(map[string]time.Time),
		ttl:      ttl,
		cooldown: cooldown,
	}
}

// HashToken hashes a raw token using SHA-256 for secure at-rest storage.
func HashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// CreateToken creates a cryptographically secure random token for an identifier,
// hashes it for storage, and returns the raw token to be emailed to the user.
// Enforces rate-limiting / cooldown per identifier.
func (b *PasswordResetBroker) CreateToken(identifier string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if lastReq, exists := b.throttle[identifier]; exists {
		if now.Sub(lastReq) < b.cooldown {
			return "", ErrResetThrottled
		}
	}

	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", err
	}
	rawToken := hex.EncodeToString(rawBytes)
	tokenHash := HashToken(rawToken)

	b.records[identifier] = PasswordResetRecord{
		Identifier: identifier,
		TokenHash:  tokenHash,
		CreatedAt:  now,
		ExpiresAt:  now.Add(b.ttl),
	}
	b.throttle[identifier] = now

	return rawToken, nil
}

// VerifyToken checks whether a raw token matches the stored hash and is not expired.
func (b *PasswordResetBroker) VerifyToken(identifier, rawToken string) (bool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	rec, exists := b.records[identifier]
	if !exists {
		return false, ErrResetTokenInvalid
	}

	if time.Now().After(rec.ExpiresAt) {
		return false, ErrResetTokenExpired
	}

	if HashToken(rawToken) != rec.TokenHash {
		return false, ErrResetTokenInvalid
	}

	return true, nil
}

// ConsumeToken verifies and immediately deletes the reset token to ensure single use.
func (b *PasswordResetBroker) ConsumeToken(identifier, rawToken string) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	rec, exists := b.records[identifier]
	if !exists {
		return false, ErrResetTokenInvalid
	}

	if time.Now().After(rec.ExpiresAt) {
		delete(b.records, identifier)
		return false, ErrResetTokenExpired
	}

	if HashToken(rawToken) != rec.TokenHash {
		return false, ErrResetTokenInvalid
	}

	// Single use: destroy record
	delete(b.records, identifier)
	return true, nil
}

// InvalidateTokens invalidates any active reset tokens for an identifier (e.g. after password change).
func (b *PasswordResetBroker) InvalidateTokens(identifier string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.records, identifier)
}
