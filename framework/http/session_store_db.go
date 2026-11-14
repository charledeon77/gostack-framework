/*
Purpose:
Implements a database-backed session store for GoStack applications that need
persistent sessions across server restarts or multi-instance deployments.

Philosophy:
The in-memory store is ideal for development, but production applications need
sessions that survive restarts and work across multiple server instances. By
implementing the same contract.SessionStore interface, the database store is a
zero-change drop-in replacement — no application code changes required.

Architecture:
Sessions are serialised to JSON and stored in a `sessions` table with an
expiry timestamp. A background garbage-collection ticker periodically purges
expired rows, preventing unbounded table growth.

Required migration:
  CREATE TABLE sessions (
      id           VARCHAR(128)  NOT NULL PRIMARY KEY,
      payload      TEXT          NOT NULL,
      last_activity INTEGER      NOT NULL
  );

Implementation:
- DBSession:      in-memory session, serialised to DB on Save.
- DBSessionStore: loads, saves, destroys, and GC-sweeps sessions via SQL.
*/
package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	netHTTP "net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

var validTableRegex = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func isPostgresDriver(driver string) bool {
	d := strings.ToLower(driver)
	return strings.Contains(d, "postgres") || strings.Contains(d, "cockroach") || strings.Contains(d, "pq") || strings.Contains(d, "pgx")
}

// DBSession is a session implementation whose data is serialised to a
// database row on each Save. In-memory access is protected by a RWMutex.
type DBSession struct {
	mu   sync.RWMutex
	id   string
	data map[string]any
}

func newDBSession(id string) *DBSession {
	return &DBSession{id: id, data: make(map[string]any)}
}

func (s *DBSession) ID() string { return s.id }

func (s *DBSession) Get(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[key]
}

func (s *DBSession) Set(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = val
}

func (s *DBSession) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

func (s *DBSession) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string]any)
}

func (s *DBSession) Flash(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data["__flash__"+key] = val
}

func (s *DBSession) GetFlash(key string) any {
	s.mu.Lock()
	defer s.mu.Unlock()
	flashKey := "__flash__" + key
	val, exists := s.data[flashKey]
	if !exists {
		return nil
	}
	delete(s.data, flashKey)
	return val
}

// All returns a shallow copy of all key-value pairs stored in the session.
func (s *DBSession) All() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make(map[string]any, len(s.data))
	for k, v := range s.data {
		res[k] = v
	}
	return res
}

// serialise marshals the session data map into JSON bytes.
func (s *DBSession) serialise() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return json.Marshal(s.data)
}

// DBSessionStore implements contract.SessionStore using a *sql.DB backend.
//
// Usage:
//
//	store := http.NewDBSessionStore(db, "sessions", 120*time.Minute, "")
//	router.Use(http.SessionMiddleware(store, "gostack_session"))
type DBSessionStore struct {
	db         *sql.DB
	driver     string
	table      string
	ttl        time.Duration
	signingKey []byte // optional HMAC signing key for session ID verification
	stopGC     chan struct{}
}

// NewDBSessionStore creates a database session store.
// table is the SQL table name (default "sessions").
// ttl controls the session lifetime (default 2 hours).
// signingKey optionally signs session IDs with HMAC-SHA256 to detect tampering.
func NewDBSessionStore(db *sql.DB, table string, ttl time.Duration, signingKey string) *DBSessionStore {
	if table == "" {
		table = "sessions"
	}
	if !validTableRegex.MatchString(table) {
		table = "sessions"
	}
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	var drv string
	if db != nil && db.Driver() != nil {
		drv = fmt.Sprintf("%T", db.Driver())
	}
	store := &DBSessionStore{
		db:         db,
		driver:     drv,
		table:      table,
		ttl:        ttl,
		stopGC:     make(chan struct{}),
	}
	if signingKey != "" {
		store.signingKey = []byte(signingKey)
	}
	go store.runGC()
	return store
}

// SetDriver manually sets or overrides the database driver type (e.g. "postgres", "mysql", "sqlite").
func (store *DBSessionStore) SetDriver(driver string) *DBSessionStore {
	store.driver = driver
	return store
}

// Load retrieves or creates a session from the database.
func (store *DBSessionStore) Load(id string) (contract.Session, error) {
	if !store.verifyID(id) {
		// Tampered or unsigned ID — issue a fresh session.
		id = generateSessionID()
	}

	sess := newDBSession(id)

	query := fmt.Sprintf("SELECT payload FROM %s WHERE id = ? AND last_activity > ?", store.table)
	if isPostgresDriver(store.driver) {
		query = fmt.Sprintf("SELECT payload FROM %s WHERE id = $1 AND last_activity > $2", store.table)
	}

	row := store.db.QueryRow(
		query,
		id, time.Now().Add(-store.ttl).Unix(),
	)

	var payload string
	if err := row.Scan(&payload); err != nil {
		if err == sql.ErrNoRows {
			// New session — nothing to load.
			return sess, nil
		}
		return nil, fmt.Errorf("[DBSessionStore] Load: %w", err)
	}

	if err := json.Unmarshal([]byte(payload), &sess.data); err != nil {
		// Corrupt payload — start fresh.
		sess.data = make(map[string]any)
	}
	return sess, nil
}

// Save serialises and upserts the session into the database with driver awareness (SEC-13).
func (store *DBSessionStore) Save(s contract.Session) error {
	dbSess, ok := s.(*DBSession)
	if !ok {
		return fmt.Errorf("[DBSessionStore] Save: expected *DBSession, got %T", s)
	}

	payload, err := dbSess.serialise()
	if err != nil {
		return fmt.Errorf("[DBSessionStore] Save: marshal failed: %w", err)
	}

	now := time.Now().Unix()

	// PostgreSQL / CockroachDB
	if strings.Contains(strings.ToLower(store.driver), "postgres") || strings.Contains(strings.ToLower(store.driver), "cockroach") || strings.Contains(strings.ToLower(store.driver), "pq") {
		query := fmt.Sprintf(`INSERT INTO %s (id, payload, last_activity) VALUES ($1, $2, $3) ON CONFLICT (id) DO UPDATE SET payload = EXCLUDED.payload, last_activity = EXCLUDED.last_activity`, store.table)
		_, err = store.db.Exec(query, s.ID(), string(payload), now)
		return err
	}

	// SQLite modern standard syntax
	if strings.Contains(strings.ToLower(store.driver), "sqlite") {
		query := fmt.Sprintf(`INSERT INTO %s (id, payload, last_activity) VALUES (?, ?, ?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload, last_activity=excluded.last_activity`, store.table)
		_, err = store.db.Exec(query, s.ID(), string(payload), now)
		return err
	}

	// MySQL / MariaDB
	_, err = store.db.Exec(
		fmt.Sprintf(`INSERT INTO %s (id, payload, last_activity) VALUES (?, ?, ?)
		             ON DUPLICATE KEY UPDATE payload = VALUES(payload), last_activity = VALUES(last_activity)`, store.table),
		s.ID(), string(payload), now,
	)
	return err
}

// Exists reports whether the session exists in the database and is not expired (SEC-16).
func (store *DBSessionStore) Exists(id string) bool {
	query := fmt.Sprintf("SELECT 1 FROM %s WHERE id = ? AND last_activity > ?", store.table)
	if isPostgresDriver(store.driver) {
		query = fmt.Sprintf("SELECT 1 FROM %s WHERE id = $1 AND last_activity > $2", store.table)
	}
	var one int
	err := store.db.QueryRow(query, id, time.Now().Add(-store.ttl).Unix()).Scan(&one)
	return err == nil
}

// Destroy deletes a session from the database.
func (store *DBSessionStore) Destroy(id string) error {
	placeholder := "?"
	if strings.Contains(strings.ToLower(store.driver), "postgres") || strings.Contains(strings.ToLower(store.driver), "cockroach") || strings.Contains(strings.ToLower(store.driver), "pq") {
		placeholder = "$1"
	}
	_, err := store.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = %s", store.table, placeholder), id)
	return err
}

// Rotate generates a new ID, migrates data, and deletes the old record in the database (SEC-04).
func (store *DBSessionStore) Rotate(oldID string) (contract.Session, error) {
	oldSess, err := store.Load(oldID)
	if err != nil {
		return nil, err
	}

	newID := generateSessionID()
	newSess := newDBSession(newID)

	for k, v := range oldSess.All() {
		newSess.data[k] = v
	}

	_ = store.Destroy(oldID)
	if err := store.Save(newSess); err != nil {
		return nil, err
	}

	return newSess, nil
}

// Stop shuts down the background GC goroutine.
func (store *DBSessionStore) Stop() {
	close(store.stopGC)
}

// runGC runs a periodic sweep to delete expired session rows.
func (store *DBSessionStore) runGC() {
	ticker := time.NewTicker(store.ttl / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cutoff := time.Now().Add(-store.ttl).Unix()
			delQuery := fmt.Sprintf("DELETE FROM %s WHERE last_activity < ?", store.table)
			if isPostgresDriver(store.driver) {
				delQuery = fmt.Sprintf("DELETE FROM %s WHERE last_activity < $1", store.table)
			}
			_, _ = store.db.Exec(delQuery, cutoff)
		case <-store.stopGC:
			return
		}
	}
}

// verifyID returns true if no signing key is set (no verification needed) or if
// the HMAC signature embedded in the session ID is valid.
func (store *DBSessionStore) verifyID(id string) bool {
	if store.signingKey == nil {
		return true
	}
	// Signed IDs are formatted as "rawID.signature"
	if len(id) < 65 { // 64 hex + '.' + signature
		return false
	}
	raw := id[:64]
	sig := id[65:]
	mac := hmac.New(sha256.New, store.signingKey)
	mac.Write([]byte(raw))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

// SignedSessionID generates a session ID signed with the store's key (when set).
func (store *DBSessionStore) SignedSessionID() string {
	raw := generateSessionID()
	if store.signingKey == nil {
		return raw
	}
	mac := hmac.New(sha256.New, store.signingKey)
	mac.Write([]byte(raw))
	sig := hex.EncodeToString(mac.Sum(nil))
	return raw + "." + sig
}

// DBSessionMiddleware is a drop-in for SessionMiddleware that uses DBSessionStore.
// It auto-sets the Secure flag based on TLS, adds SameSite=Lax, and supports
// HMAC-signed session IDs.
func DBSessionMiddleware(store *DBSessionStore, cookieName string) Middleware {
	if cookieName == "" {
		cookieName = "gostack_session"
	}
	return func(ctx *Context, next NextHandler) error {
		var sessionID string

		cookie, err := ctx.Request.Cookie(cookieName)
		if err == nil && cookie != nil && cookie.Value != "" {
			sessionID = cookie.Value
		} else {
			sessionID = store.SignedSessionID()
		}

		sess, err := store.Load(sessionID)
		if err != nil {
			return err
		}

		ctx.Set("session", sess)

		isSecure := ctx.Request.TLS != nil || ctx.Request.Header.Get("X-Forwarded-Proto") == "https"
		netHTTP.SetCookie(ctx.Writer, &netHTTP.Cookie{
			Name:     cookieName,
			Value:    sessionID,
			Path:     "/",
			HttpOnly: true,
			Secure:   isSecure,
			SameSite: netHTTP.SameSiteLaxMode,
		})

		err = next(ctx)

		if saveErr := store.Save(sess); saveErr != nil && err == nil {
			return saveErr
		}
		return err
	}
}
