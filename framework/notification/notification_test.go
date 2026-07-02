package notification

import (
	"testing"

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
