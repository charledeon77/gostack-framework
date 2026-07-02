/*
Purpose:
This file defines the core contracts and interfaces for GoStack's Multi-Channel Notification System.

Philosophy:
We believe user alerting should be expressive, simple, and extensible. By decoupling notifications
from delivery channels, developers can write clean notification structures that target multiple platforms
(Mail, Database, SMS) using a unified, fluent execution flow.

Architecture:
Implements abstract contracts for Notifiable receivers, custom Notifications, and Channel drivers.
This allows GoStack applications to register and swap custom notification adapters at boot time.

Choice:
We chose a Laravel-inspired `Via()` pattern, translated to idiomatic Go. Receivers implement
the simple `Notifiable` interface to keep things fully decoupled.
*/
package notification

import (
	"github.com/charledeon77/gostack-framework/framework/mail"
)

// Notifiable defines the receiver of a notification.
type Notifiable interface {
	// GetEmail returns the primary target email address.
	GetEmail() string
	// GetID returns the primary key identifier of the receiver.
	GetID() any
}

// Notification defines the contract for an application-defined notification.
type Notification interface {
	// Via defines the channels to send this notification through (e.g. []string{"mail", "database"})
	Via(notifiable any) []string
}

// MailableNotification is implemented by notifications that support the "mail" channel.
type MailableNotification interface {
	Notification
	// ToMail constructs the mail message payload.
	ToMail(notifiable any) (*mail.Message, error)
}

// DatabaseNotification is implemented by notifications that support the "database" channel.
type DatabaseNotification interface {
	Notification
	// ToDatabase constructs the database payload to be serialized into JSON.
	ToDatabase(notifiable any) (map[string]any, error)
}

// Channel defines the interface for a delivery channel.
type Channel interface {
	// Send delivers the notification to the target notifiable entity.
	Send(notifiable any, notification Notification) error
}
