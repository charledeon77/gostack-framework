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
The mail component defines a `Message` representing SMTP metadata and payload.

Choice:
We chose a direct `net/smtp.SendMail` wrapper because it is fast, simple, and is part of
the Go stdlib. By wrapping it, we support standard auth types and structure the
headers/body automatically to avoid common issues with MIME formatting.

Implementation:
- Config: holds SMTP server information.
- Message: represents the details of an email to be sent.
- Attachment: represents a file to be attached.
- Mailer: holds config and provides Send() method.
- MessageBuilder: fluent builder for constructing messages step-by-step.
*/
package mail

import (
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"path/filepath"
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

// Attachment represents a file to embed in the email as a MIME attachment.
type Attachment struct {
	Filename string // Name as shown to the recipient
	MIMEType string // e.g. "application/pdf"; auto-detected from Filename if empty
	Content  []byte // Raw file bytes
}

// Message represents an individual email message.
type Message struct {
	To          []string
	CC          []string
	BCC         []string
	ReplyTo     string
	Subject     string
	Body        string     // Plain-text body
	HTMLBody    string     // HTML body (when non-empty, a multipart/alternative message is built)
	IsHTML      bool       // Deprecated: prefer HTMLBody; kept for backward compatibility
	Attachments []Attachment
}

// Mailer handles SMTP email dispatch.
type Mailer struct {
	cfg Config
}

// NewMailer creates a new Mailer instance.
func NewMailer(cfg Config) *Mailer {
	return &Mailer{cfg: cfg}
}

// NewMessage returns a fluent MessageBuilder seeded with a subject.
func NewMessage(subject string) *MessageBuilder {
	return &MessageBuilder{msg: Message{Subject: subject}}
}

// Send formats and sends the given Message via SMTP.
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

	// Gather all envelope recipients (To + CC + BCC)
	var recipients []string
	recipients = append(recipients, mailMsg.To...)
	recipients = append(recipients, mailMsg.CC...)
	recipients = append(recipients, mailMsg.BCC...)

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

// BuildRaw formats the Message into a raw SMTP bytes payload with correct MIME headers.
// Supports multipart/alternative (text+HTML), file attachments, BCC, and Reply-To.
func (m *Mailer) BuildRaw(msg Message) []byte {
	boundary := "==GoStack_MIME_Boundary=="
	hasAttachments := len(msg.Attachments) > 0
	hasHTML := msg.HTMLBody != "" || msg.IsHTML

	var b strings.Builder

	// ── Envelope headers ────────────────────────────────────────────────────
	if m.cfg.FromName != "" {
		b.WriteString(fmt.Sprintf("From: %s <%s>\r\n", m.cfg.FromName, m.cfg.FromAddress))
	} else {
		b.WriteString(fmt.Sprintf("From: %s\r\n", m.cfg.FromAddress))
	}
	b.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(msg.To, ", ")))
	if len(msg.CC) > 0 {
		b.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(msg.CC, ", ")))
	}
	// BCC is intentionally omitted from headers (already in SMTP envelope recipients)
	if msg.ReplyTo != "" {
		b.WriteString(fmt.Sprintf("Reply-To: %s\r\n", msg.ReplyTo))
	}
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", msg.Subject))
	b.WriteString("MIME-Version: 1.0\r\n")

	// ── Body ────────────────────────────────────────────────────────────────
	switch {
	case hasAttachments:
		// multipart/mixed wraps both the body content and attachments
		b.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundary))
		b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		if hasHTML {
			b.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
			b.WriteString(msg.HTMLBody)
		} else {
			b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
			b.WriteString(msg.Body)
		}
		b.WriteString("\r\n")
		for _, att := range msg.Attachments {
			mt := att.MIMEType
			if mt == "" {
				mt = mime.TypeByExtension(filepath.Ext(att.Filename))
				if mt == "" {
					mt = "application/octet-stream"
				}
			}
			b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			b.WriteString(fmt.Sprintf("Content-Type: %s\r\n", mt))
			b.WriteString("Content-Transfer-Encoding: base64\r\n")
			b.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n\r\n", att.Filename))
			encoded := base64.StdEncoding.EncodeToString(att.Content)
			// RFC 2045 §6.8: wrap base64 at 76 characters
			for len(encoded) > 76 {
				b.WriteString(encoded[:76] + "\r\n")
				encoded = encoded[76:]
			}
			b.WriteString(encoded + "\r\n")
		}
		b.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	case hasHTML && msg.Body != "":
		// multipart/alternative for simultaneous plain-text and HTML
		b.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))
		b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		b.WriteString(msg.Body)
		b.WriteString("\r\n")
		b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		b.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		b.WriteString(msg.HTMLBody)
		b.WriteString("\r\n")
		b.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	case hasHTML:
		b.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		body := msg.HTMLBody
		if body == "" {
			body = msg.Body
		}
		b.WriteString(body)

	default:
		b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		b.WriteString(msg.Body)
	}

	return []byte(b.String())
}

// ── MessageBuilder ───────────────────────────────────────────────────────────

// MessageBuilder provides a fluent API for composing email messages.
// All methods return the builder itself, enabling chaining.
type MessageBuilder struct {
	msg Message
}

// To sets the primary recipient list.
func (b *MessageBuilder) To(recipients ...string) *MessageBuilder {
	b.msg.To = recipients
	return b
}

// CC sets the carbon-copy recipient list.
func (b *MessageBuilder) CC(recipients ...string) *MessageBuilder {
	b.msg.CC = recipients
	return b
}

// BCC sets the blind carbon-copy recipient list.
func (b *MessageBuilder) BCC(recipients ...string) *MessageBuilder {
	b.msg.BCC = recipients
	return b
}

// ReplyTo sets the Reply-To address.
func (b *MessageBuilder) ReplyTo(addr string) *MessageBuilder {
	b.msg.ReplyTo = addr
	return b
}

// Text sets the plain-text body content.
func (b *MessageBuilder) Text(body string) *MessageBuilder {
	b.msg.Body = body
	return b
}

// HTML sets the HTML body content.
func (b *MessageBuilder) HTML(body string) *MessageBuilder {
	b.msg.HTMLBody = body
	return b
}

// Attach adds a file attachment.
func (b *MessageBuilder) Attach(filename string, content []byte, mimeType ...string) *MessageBuilder {
	att := Attachment{Filename: filename, Content: content}
	if len(mimeType) > 0 {
		att.MIMEType = mimeType[0]
	}
	b.msg.Attachments = append(b.msg.Attachments, att)
	return b
}

// Build returns the fully constructed Message.
func (b *MessageBuilder) Build() Message {
	return b.msg
}

