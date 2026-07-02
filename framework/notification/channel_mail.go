/*
Purpose:
Implements the SMTP email delivery channel for the notification system.

Philosophy:
E-mail remains the most critical standard channel for web communication. Rather than reinventing
the SMTP handler, this channel delegates message formatting to custom Notification interfaces and
hands the final payload off to GoStack's existing robust Mailer service.

Architecture:
Implements the Channel interface. It inspects whether the notification implements the
MailableNotification interface, builds the *mail.Message payload, and dispatches it.
*/
package notification

import (
	"fmt"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

// MailChannel handles SMTP delivery using the framework's registered Mailer.
type MailChannel struct {
	mailer contract.Mailer
}

// NewMailChannel initializes a new Mail delivery channel.
func NewMailChannel(mailer contract.Mailer) *MailChannel {
	return &MailChannel{mailer: mailer}
}

// Send formats and transmits the notification as an email to the target receiver.
func (c *MailChannel) Send(notifiable any, notification Notification) error {
	mailable, ok := notification.(MailableNotification)
	if !ok {
		return fmt.Errorf("notification does not implement MailableNotification interface")
	}

	receiver, ok := notifiable.(Notifiable)
	if !ok {
		return fmt.Errorf("receiver does not implement Notifiable interface")
	}

	msg, err := mailable.ToMail(notifiable)
	if err != nil {
		return fmt.Errorf("notification mail template generation failed: %w", err)
	}

	if len(msg.To) == 0 {
		email := receiver.GetEmail()
		if email == "" {
			return fmt.Errorf("receiver has no valid email address configured")
		}
		msg.To = []string{email}
	}

	return c.mailer.Send(msg)
}
