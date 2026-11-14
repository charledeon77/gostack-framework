package notification

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
	"github.com/charledeon77/gostack-framework/framework/mail"
)

// MockNotifiable implements Notifiable.
type MockNotifiable struct {
	ID    int
	Email string
}

func (m *MockNotifiable) GetEmail() string { return m.Email }
func (m *MockNotifiable) GetID() any       { return m.ID }

// MockNotification implements Notification, MailableNotification, and DatabaseNotification.
type MockNotification struct {
	Subject string
	Message string
}

func (n *MockNotification) Via(notifiable any) []string {
	return []string{"mail", "database"}
}

func (n *MockNotification) ToMail(notifiable any) (*mail.Message, error) {
	return &mail.Message{
		Subject: n.Subject,
		Body:    n.Message,
	}, nil
}

func (n *MockNotification) ToDatabase(notifiable any) (map[string]any, error) {
	return map[string]any{
		"subject": n.Subject,
		"message": n.Message,
	}, nil
}

// MockMailer implements contract.Mailer.
type MockMailer struct {
	SentMessages []any
}

func (m *MockMailer) Send(msg any) error {
	m.SentMessages = append(m.SentMessages, msg)
	return nil
}

// MockDatabase implements contract.Database.
type MockDatabase struct {
	ExecutedQueries []string
	ExecutedArgs    [][]any
}

func (m *MockDatabase) Connect() error                                     { return nil }
func (m *MockDatabase) Query(sql string, args ...any) (interface{}, error) { return nil, nil }
func (m *MockDatabase) Exec(sql string, args ...any) error {
	m.ExecutedQueries = append(m.ExecutedQueries, sql)
	m.ExecutedArgs = append(m.ExecutedArgs, args)
	return nil
}
func (m *MockDatabase) BeginTx() (contract.Tx, error) { return nil, nil } // Match signature exactly
func (m *MockDatabase) Driver() string                { return "sqlite" }
func (m *MockDatabase) Close() error                  { return nil }

func TestNotificationSystem(t *testing.T) {
	dispatcher := NewDispatcher()

	mockMailer := &MockMailer{}
	mockDb := &MockDatabase{}

	// Create and register channels
	dispatcher.Register("mail", NewMailChannel(mockMailer))
	dispatcher.Register("database", NewDatabaseChannel(mockDb))

	notifiable := &MockNotifiable{ID: 1, Email: "test@gostack.dev"}
	notification := &MockNotification{Subject: "Welcome", Message: "Thanks for joining GoStack!"}

	err := dispatcher.Send(notifiable, notification)
	if err != nil {
		t.Fatalf("failed to dispatch notification: %v", err)
	}

	// 1. Verify Mail Delivery
	if len(mockMailer.SentMessages) != 1 {
		t.Errorf("expected 1 mail message sent, got %d", len(mockMailer.SentMessages))
	} else {
		msg, ok := mockMailer.SentMessages[0].(*mail.Message)
		if !ok {
			t.Fatalf("expected mail.Message type, got %T", mockMailer.SentMessages[0])
		}
		if msg.Subject != "Welcome" {
			t.Errorf("expected subject 'Welcome', got '%s'", msg.Subject)
		}
		if msg.Body != "Thanks for joining GoStack!" {
			t.Errorf("expected body 'Thanks for joining GoStack!', got '%s'", msg.Body)
		}
	}

	// 2. Verify Database Persistence
	if len(mockDb.ExecutedQueries) != 1 {
		t.Errorf("expected 1 database query executed, got %d", len(mockDb.ExecutedQueries))
	} else {
		query := mockDb.ExecutedQueries[0]
		if query != "INSERT INTO notifications (id, type, notifiable_id, data, read_at, created_at) VALUES (?, ?, ?, ?, ?, ?)" {
			t.Errorf("unexpected query executed: %s", query)
		}

		args := mockDb.ExecutedArgs[0]
		if len(args) != 6 {
			t.Errorf("expected 6 query arguments, got %d", len(args))
		}
		if args[1] != "*notification.MockNotification" {
			t.Errorf("expected type arg '*notification.MockNotification', got '%s'", args[1])
		}
		if args[2] != "1" {
			t.Errorf("expected notifiable_id arg '1', got '%v'", args[2])
		}
	}
}

// ── PAR-22: Resilient Dispatch Tests ─────────────────────────────────────────

// failingChannel always returns an error on Send.
type failingChannel struct{ name string }

func (f *failingChannel) Send(notifiable any, notification Notification) error {
	return errors.New("simulated " + f.name + " transport error")
}

// recordingChannel records every successful send.
type recordingChannel struct {
	mu   sync.Mutex
	sent []any
}

func (r *recordingChannel) Send(notifiable any, notification Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, notifiable)
	return nil
}

// slowRegisterer simulates a channel.Register call that happens concurrently with Send.
type slowRegisterer struct {
	delay time.Duration
	inner Channel
}

func (s *slowRegisterer) Send(notifiable any, notification Notification) error {
	time.Sleep(s.delay)
	return s.inner.Send(notifiable, notification)
}

// twoChannelNotification requests both "mail" and "db" channels.
type twoChannelNotification struct{}

func (n *twoChannelNotification) Via(_ any) []string { return []string{"mail", "db"} }
func (n *twoChannelNotification) ToMail(_ any) (*mail.Message, error) {
	return &mail.Message{Subject: "hi"}, nil
}
func (n *twoChannelNotification) ToDatabase(_ any) (map[string]any, error) {
	return map[string]any{"msg": "hi"}, nil
}

// TestPAR22_MailFailureDoesNotBlockDB asserts that a failing mail channel
// does not prevent the DB channel notification from being written.
func TestPAR22_MailFailureDoesNotBlockDB(t *testing.T) {
	dispatcher := NewDispatcher()
	dbCh := &recordingChannel{}
	dispatcher.Register("mail", &failingChannel{name: "mail"})
	dispatcher.Register("db", dbCh)

	notifiable := &MockNotifiable{ID: 1, Email: "user@example.com"}
	err := dispatcher.Send(notifiable, &twoChannelNotification{})

	// Error must name the failing channel
	if err == nil {
		t.Fatal("expected aggregated error, got nil")
	}
	if !strings.Contains(err.Error(), "mail") {
		t.Errorf("expected error to mention 'mail' channel, got: %v", err)
	}

	// DB notification must still have been written
	dbCh.mu.Lock()
	defer dbCh.mu.Unlock()
	if len(dbCh.sent) != 1 {
		t.Errorf("expected DB channel to receive 1 notification, got %d", len(dbCh.sent))
	}
}

// TestPAR22_SendManyCompletesAllRecipients asserts that SendMany continues
// delivering to all recipients even when one fails.
func TestPAR22_SendManyCompletesAllRecipients(t *testing.T) {
	var sendCount int64
	dispatcher := NewDispatcher()
	dispatcher.Register("mail", &failingChannel{name: "mail"})
	dispatcher.Register("db", Channel(channelFunc(func(notifiable any, n Notification) error {
		atomic.AddInt64(&sendCount, 1)
		return nil
	})))

	notifiables := []any{
		&MockNotifiable{ID: 1, Email: "a@example.com"},
		&MockNotifiable{ID: 2, Email: "b@example.com"},
		&MockNotifiable{ID: 3, Email: "c@example.com"},
	}
	err := dispatcher.SendMany(notifiables, &twoChannelNotification{})
	if err == nil {
		t.Fatal("expected aggregated errors from SendMany, got nil")
	}

	// All 3 DB notifications must have been written despite mail failures
	if atomic.LoadInt64(&sendCount) != 3 {
		t.Errorf("expected 3 successful DB sends, got %d", sendCount)
	}
}

// TestPAR22_NoConcurrentRegisterDeadlock asserts that Register does not deadlock
// while Send is in progress (lock is not held during channel.Send).
func TestPAR22_NoConcurrentRegisterDeadlock(t *testing.T) {
	dispatcher := NewDispatcher()
	rec := &recordingChannel{}
	dispatcher.Register("db", &slowRegisterer{delay: 50 * time.Millisecond, inner: rec})

	type oneChannelNotif struct{}
	dispatcher.Register("extra", rec)

	done := make(chan struct{})
	go func() {
		notifiable := &MockNotifiable{ID: 99}
		// Use a notification that only uses "db" so we hit the slow channel
		type dbOnly struct{}
		_ = dispatcher.Send(notifiable, &struct {
			twoChannelNotification
		}{})
		close(done)
	}()

	// Concurrently register while Send is sleeping inside the slow channel
	time.Sleep(10 * time.Millisecond)
	registerDone := make(chan struct{})
	go func() {
		dispatcher.Register("lateChannel", rec)
		close(registerDone)
	}()

	select {
	case <-registerDone:
		// Good: Register completed without waiting for Send to finish
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Register deadlocked waiting for Send to release the lock")
	}
	<-done
}

// channelFunc is a functional adapter for the Channel interface.
type channelFunc func(notifiable any, n Notification) error

func (f channelFunc) Send(notifiable any, n Notification) error { return f(notifiable, n) }
