/*
Purpose:
This file implements the GoMail email sending component. It provides an interface
and implementation for sending rich text and HTML emails via SMTP.

Philosophy:
Email sending is a core utility for user communication (verification, reset, notifications).
We provide a production-ready, pure standard library implementation based on `net/smtp`.
This adheres to our zero-dependency framework core while providing a clean, ergonomic API
that handles all low-level SMTP formatting and handshakes.

Architecture:
The mailer exposes a `Mailer` struct which is initialized with `Config`.
The global `gostack.Mail` facade allows easy application-wide usage.
The mail component defines a `Message` representing SMTP metadata and payload (To, CC, Subject, Body, HTML).

Choice:
We chose a direct `net/smtp.SendMail` wrapper because it is fast, simple, and is part of the Go stdlib.
By wrapping it, we support standard auth types (like Plain Auth) and structure the headers/body automatically
to avoid common issues with MIME formatting.

Implementation:
- Config: holds SMTP server information (Host, Port, Username, Password, FromAddress, FromName).
- Message: represents the details of an email to be sent.
- Mailer: holds config and provides Send() method.
- Send(msg Message): translates Message into raw RFC 822 format, establishes connection, authenticates, and sends.
*/
package mail

import (
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// Config defines the configuration properties required to connect to an SMTP server.
type Config struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	FromName    string
}

// Message represents an individual email message.
type Message struct {
	To      []string
	CC      []string
	Subject string
	Body    string
	IsHTML  bool
}

// Mailer handles SMTP email dispatch.
type Mailer struct {
	cfg Config
}

// NewMailer creates a new Mailer instance.
func NewMailer(cfg Config) *Mailer {
	return &Mailer{cfg: cfg}
}

// Send formats and sends the given Message via SMTP. It accepts any to satisfy contract.Mailer.
func (m *Mailer) Send(msg any) error {
	var mailMsg Message
	switch v := msg.(type) {
	case Message:
		mailMsg = v
	case *Message:
		if v == nil {
			return fmt.Errorf("mail: nil Message pointer")
		}
		mailMsg = *v
	default:
		return fmt.Errorf("mail: invalid message type, expected mail.Message or *mail.Message")
	}

	addr := net.JoinHostPort(m.cfg.Host, fmt.Sprintf("%d", m.cfg.Port))
	var auth smtp.Auth
	if m.cfg.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	}

	// Gather recipients
	var recipients []string
	recipients = append(recipients, mailMsg.To...)
	recipients = append(recipients, mailMsg.CC...)

	var cleaned []string
	seen := make(map[string]bool)
	for _, r := range recipients {
		r = strings.TrimSpace(r)
		if r != "" && !seen[r] {
			seen[r] = true
			cleaned = append(cleaned, r)
		}
	}

	if len(cleaned) == 0 {
		return fmt.Errorf("mail: no recipients specified")
	}

	raw := m.BuildRaw(mailMsg)
	return smtp.SendMail(addr, auth, m.cfg.FromAddress, cleaned, raw)
}

// BuildRaw formats the Message into a raw SMTP bytes payload with correct headers.
func (m *Mailer) BuildRaw(msg Message) []byte {
	var headers []string

	if m.cfg.FromName != "" {
		headers = append(headers, fmt.Sprintf("From: %s <%s>", m.cfg.FromName, m.cfg.FromAddress))
	} else {
		headers = append(headers, fmt.Sprintf("From: %s", m.cfg.FromAddress))
	}

	headers = append(headers, fmt.Sprintf("To: %s", strings.Join(msg.To, ", ")))
	if len(msg.CC) > 0 {
		headers = append(headers, fmt.Sprintf("Cc: %s", strings.Join(msg.CC, ", ")))
	}

	headers = append(headers, fmt.Sprintf("Subject: %s", msg.Subject))
	headers = append(headers, "MIME-Version: 1.0")
	if msg.IsHTML {
		headers = append(headers, "Content-Type: text/html; charset=UTF-8")
	} else {
		headers = append(headers, "Content-Type: text/plain; charset=UTF-8")
	}

	rawMsg := strings.Join(headers, "\r\n") + "\r\n\r\n" + msg.Body
	return []byte(rawMsg)
}
