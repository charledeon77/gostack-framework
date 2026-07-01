/*
Purpose:
This file defines the abstract contract interfaces for the GoStack framework.
It establishes unified behaviors for database connectivity, transactions, and session storage.

Philosophy:
We believe core framework components should remain decoupled from concrete backend adapters.
By defining abstract contracts, GoStack allows developers to swap database drivers or session
backends without modifying application-level controller code.

Architecture:
These contracts form the interfaces consumed by HTTP handlers, schema builders, and kernel layers.
Adapters (like SQLAdapter and MemorySessionStore) implement these contracts to register themselves.

Choice:
We consolidated three small contract files (database.go, session.go, tx.go) into a single, cohesive
contracts.go file to reduce directory fragmentation and simplify framework navigation.

Implementation:
- Database: Defines connection handshake, query execution, and transaction hooks.
- Tx: Defines Exec, Query, Commit, and Rollback hooks for transactional database queries.
- Session: Defines key-value accessors for request-scoped user state.
- SessionStore: Defines Load, Save, and Destroy hooks for session lifecycle storage.
*/
package contract

import (
	"net/http"
	"time"
)

// Database defines the behaviors required for any database driver 
// used within the framework. By using this interface, we ensure that
// the QueryBuilder can interact with any database system that implements
// these core operations.
type Database interface {
	// Connect initializes the connection to the underlying database driver.
	// It should return an error if the connection fails or if the 
	// credentials provided are invalid.
	Connect() error

	// Query executes a parameterized SQL string against the database and returns 
	// the result set or an error. 
	Query(sql string, args ...any) (interface{}, error)

	// Exec executes a parameterized SQL statement that returns no rows (e.g. DDL, INSERT, UPDATE).
	Exec(sql string, args ...any) error

	// BeginTx opens a new database transaction and returns it as a Tx interface.
	BeginTx() (Tx, error)

	// Driver returns the driver identifier name string (e.g. "postgres" or "mysql").
	Driver() string

	// Close releases the underlying database connection pool.
	// It should be called during graceful shutdown.
	Close() error
}

// ConnectionPooler defines the connection pooling methods that are optional
// for implementations of the Database interface.
type ConnectionPooler interface {
	SetMaxOpenConns(n int)
	SetMaxIdleConns(n int)
	SetConnMaxLifetime(d time.Duration)
}

// Tx defines the interface for a database transaction execution context.
//
// DESIGN RATIONALE:
// By wrapping *sql.Tx behind this interface, the schema builder and migrator
// remain fully decoupled from the concrete database driver. Any adapter that
// supports transactions (MySQL, PostgreSQL, SQLite) can implement this contract.
type Tx interface {
	// Exec executes a parameterized SQL statement inside the transaction that returns no rows.
	Exec(sql string, args ...any) error

	// Query executes a parameterized SQL query inside the transaction and returns result rows.
	Query(sql string, args ...any) (interface{}, error)

	// Commit finalizes the transaction, permanently applying all changes.
	Commit() error

	// Rollback aborts the transaction, discarding all changes made within it.
	// This should be safe to call after Commit (where it will return sql.ErrTxDone or be a no-op).
	Rollback() error
}

// Session defines the contract for accessing and modifying request-scoped session state.
//
// DESIGN RATIONALE:
// By abstracting session data operations, GoStack controllers can perform state operations
// (like user authentication flags, flash messages, or shopping carts) in a pure, standard-library
// map fashion without database-specific query syntax.
type Session interface {
	// ID returns the unique session token identifier.
	ID() string

	// Get retrieves a value stored in the session by key. Returns nil if missing.
	Get(key string) any

	// Set stores a key-value pair in the session state.
	Set(key string, val any)

	// Delete removes a specific key-value pair from the session.
	Delete(key string)

	// Clear purges all key-value data from the session.
	Clear()
}

// SessionStore defines the contract for persisting and loading session states across HTTP lifecycles.
//
// DESIGN RATIONALE:
// Because Go HTTP servers handle multiple requests concurrently, the storage backend must manage
// session lifecycles (instantiation, serialization/deserialization, and deletion) safely and cleanly.
type SessionStore interface {
	// Load retrieves an existing session from the store by its ID.
	// If the session does not exist, a new Session instance with the provided ID is returned.
	Load(id string) (Session, error)

	// Save commits the current state of a session to the storage backend.
	Save(s Session) error

	// Destroy invalidates and removes a session from the storage backend.
	Destroy(id string) error
}

// SessionContextKeyType is the context key under which the session is bound.
type SessionContextKeyType string
const SessionContextKey SessionContextKeyType = "gostack_session"

// UserContextKeyType is the context key under which the authenticated user is cached.
type UserContextKeyType string
const UserContextKey UserContextKeyType = "gostack_auth_user"

// AuthCache holds request-scoped cached user.
type AuthCache struct {
	User   Authenticatable
	Loaded bool
}

// Authenticatable is implemented by any User model that can be authenticated.
type Authenticatable interface {
	// GetID returns the primary identifier of the user (e.g. database ID).
	GetID() any

	// GetEmail returns the email address or username identifier of the user.
	GetEmail() string

	// GetPassword returns the bcrypt-hashed password string of the user.
	GetPassword() string
}

// UserProvider retrieves authenticatable user entities from storage.
type UserProvider interface {
	// RetrieveByID finds a user by their unique primary key.
	RetrieveByID(id any) (Authenticatable, error)

	// RetrieveByCredentials finds a user matching custom attributes (like email or API tokens).
	RetrieveByCredentials(credentials map[string]any) (Authenticatable, error)

	// ValidateCredentials checks if the authenticating credentials match the user's password.
	ValidateCredentials(user Authenticatable, credentials map[string]any) bool
}

// Guard defines the behavior of an authentication strategy (session, token-based, etc).
type Guard interface {
	// Check determines if the current request context contains an authenticated user.
	Check(r *http.Request) bool

	// User retrieves the authenticated user if logged in, returning false if not authenticated.
	User(r *http.Request) (Authenticatable, bool)

	// Login manually signs in a user and establishes authentication context (e.g. cookie sessions).
	Login(w http.ResponseWriter, r *http.Request, user Authenticatable) error

	// Logout invalidates the authenticated session/state.
	Logout(w http.ResponseWriter, r *http.Request) error
}

// Hasher defines standard methods for hashing and verifying passwords securely.
type Hasher interface {
	// Hash produces a secure hash of a plain text string.
	Hash(plain string) (string, error)

	// Verify compares a plain text password against a hash.
	Verify(plain, hashed string) bool
}

// Mailer defines the contract for sending emails.
type Mailer interface {
	// Send sends the given message (typically a mail.Message).
	Send(msg any) error
}

// Job defines the functional contract for all background tasks in GoStack.
type Job interface {
	// Handle executes the job's business logic.
	Handle() error

	// Name returns the unique type identifier for this job, facilitating reflection-based JSON deserialization.
	Name() string
}

// Retryable is implemented by Jobs that support automatic retry logic.
type Retryable interface {
	// MaxAttempts returns the maximum number of processing attempts before failing the job.
	MaxAttempts() int
}

// Queue defines the contract for managing background tasks.
type Queue interface {
	// Push dispatches a job to the background queue immediately.
	Push(job Job) error

	// PushDelayed dispatches a job to the queue, delaying execution by a set duration.
	PushDelayed(job Job, delay time.Duration) error

	// StartWorkers starts a pool of background worker goroutines to process jobs.
	StartWorkers(workers int)
}

// QueueStats holds the metrics for the active queue engine.
type QueueStats struct {
	Driver  string `json:"driver"`
	Pending int64  `json:"pending"`
	Delayed int64  `json:"delayed"`
	Failed  int64  `json:"failed"`
}

// FailedJob represents metadata of a background job that failed execution and exceeded retries.
type FailedJob struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Payload  string    `json:"payload"`
	Attempts int       `json:"attempts"`
	Error    string    `json:"error"`
	FailedAt time.Time `json:"failed_at"`
}

// QueueInspector provides capabilities to introspect the queue state and manage failed jobs.
type QueueInspector interface {
	// GetStats retrieves overview counters of the queue state.
	GetStats() (QueueStats, error)

	// GetFailedJobs lists failed jobs in the queue.
	GetFailedJobs() ([]FailedJob, error)

	// RetryJob re-queues a failed job by its ID.
	RetryJob(id string) error

	// DeleteFailedJob permanently removes a failed job by its ID.
	DeleteFailedJob(id string) error
}

// Listener is a handler function registered to process an event.
type Listener func(event any) error

// EventDispatcher manages the application-wide Publish/Subscribe (Pub/Sub) event network.
type EventDispatcher interface {
	// Listen registers a listener function for a specific named event category.
	Listen(eventName string, listener Listener)

	// Dispatch broadcasts an event category payload to all registered listeners.
	Dispatch(eventName string, event any) error
}

// Storage abstracts filesystem driver operations.
type Storage interface {
	// Put writes data bytes to a target filepath.
	Put(path string, contents []byte) error

	// Get retrieves data bytes from a target filepath.
	Get(path string) ([]byte, error)

	// Delete removes a file at a target filepath.
	Delete(path string) error

	// Exists checks for the presence of a file at a target filepath.
	Exists(path string) bool
}

// CacheStore defines the contract for an underlying caching driver (Memory, Redis).
// It works strictly with interface{} (any) to remain implementation agnostic.
// The developer-facing API will wrap this to provide Generic type safety.
type CacheStore interface {
	// Get retrieves a value from the cache by key. Returns false if not found.
	Get(key string) (any, bool)

	// Put stores a value in the cache for a given duration.
	Put(key string, val any, ttl time.Duration)

	// Forget removes a specific key from the cache.
	Forget(key string)

	// Flush completely clears the cache.
	Flush()
}

// Translator defines the contract for resolving localized message keys.
type Translator interface {
	// Trans resolves the localized string for a key in a given locale.
	// The replace map holds key-value placeholders to dynamically interpolate (e.g. replacing {{name}} with value).
	Trans(locale string, key string, replace map[string]string) string
}
