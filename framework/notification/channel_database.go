/*
Purpose:
Implements the persistent database delivery channel for the notification system.

Philosophy:
In-app notification centers are crucial for modern SaaS apps. We persist notification payloads in a
relational database using the framework's core database driver. This ensures we don't depend on raw
SQL drivers directly, letting MySQL, PostgreSQL, and SQLite work automatically.

Architecture:
Implements the Channel interface. It extracts custom state payloads as JSON, generates a secure,
traceable UUID for the alert, and stores the metadata under the `notifications` table.
*/
package notification

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

// DatabaseChannel handles writing notifications to a sql database.
type DatabaseChannel struct {
	db contract.Database
}

// NewDatabaseChannel initializes a database channel.
func NewDatabaseChannel(db contract.Database) *DatabaseChannel {
	return &DatabaseChannel{db: db}
}

// Send serializes and commits the notification metadata into the notifications table.
func (c *DatabaseChannel) Send(notifiable any, notification Notification) error {
	dbNotification, ok := notification.(DatabaseNotification)
	if !ok {
		return fmt.Errorf("notification does not implement DatabaseNotification interface")
	}

	receiver, ok := notifiable.(Notifiable)
	if !ok {
		return fmt.Errorf("receiver does not implement Notifiable interface")
	}

	dataMap, err := dbNotification.ToDatabase(notifiable)
	if err != nil {
		return fmt.Errorf("notification db template generation failed: %w", err)
	}

	payloadBytes, err := json.Marshal(dataMap)
	if err != nil {
		return fmt.Errorf("notification db payload serialization failed: %w", err)
	}

	// Determine notification type based on struct implementation
	notificationType := fmt.Sprintf("%T", notification)

	// Build secure transaction entry payload
	notifID := generateUUID()
	now := time.Now().UTC()

	query := `INSERT INTO notifications (id, type, notifiable_id, data, read_at, created_at) VALUES (?, ?, ?, ?, ?, ?)`

	// Convert receiver ID to a string representation for consistent DB persistence
	notifiableID := fmt.Sprintf("%v", receiver.GetID())

	err = c.db.Exec(query, notifID, notificationType, notifiableID, string(payloadBytes), nil, now)
	if err != nil {
		return fmt.Errorf("failed to save notification to database: %w", err)
	}

	return nil
}

// generateUUID creates a quick, unique hex-encoded identifier for notification rows.
func generateUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate secure notification ID")
	}
	return hex.EncodeToString(b)
}
