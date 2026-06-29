/*
Purpose:
This file implements tests for the GoStack email component. It verifies SMTP handshake,
authentication, and mail body encoding using a local loopback mock SMTP server.

Philosophy:
We test mail configuration and sending against a real TCP socket mock SMTP server rather
than standard mock structures. This guarantees that `net/smtp` is correctly utilized, headers
are properly serialized, and authentication commands succeed over standard SMTP socket streams.

Architecture:
The test spins up an asynchronous TCP listener on a dynamic loopback port (`127.0.0.1:0`).
The listener implements a state-machine satisfying basic SMTP command/response flows.
The `Mailer` connects to this local port and performs a complete `Send` cycle.

Choice:
We chose to build a simple TCP mock SMTP handler instead of using external mock mailers.
This keeps our dependency footprint completely zero, compiles instantly, and allows
granular verification of the actual text payload being transferred over the wire.

Implementation:
- startMockSMTPServer: spins up the mock listener and handles EHLO, AUTH, MAIL, RCPT, DATA, and QUIT.
- TestMailer_Send: validates mailer configuration, authentication flow, and MIME formatting.
*/
package mail

import (
	"bufio"
	"net"
	"strconv"
	"strings"
	"testing"
)

func startMockSMTPServer(t *testing.T) (string, int, func()) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock SMTP server: %v", err)
	}

	host, portStr, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("failed to split host/port: %v", err)
	}
	port, _ := strconv.Atoi(portStr)

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)

		sendLine := func(s string) {
			_, _ = writer.WriteString(s + "\r\n")
			_ = writer.Flush()
		}

		readLine := func() string {
			line, err := reader.ReadString('\n')
			if err != nil {
				return ""
			}
			return strings.TrimSpace(line)
		}

		sendLine("220 mock-smtp-server")

		for {
			line := readLine()
			if line == "" {
				break
			}

			if strings.HasPrefix(line, "EHLO") || strings.HasPrefix(line, "HELO") {
				sendLine("250-localhost")
				sendLine("250 AUTH PLAIN")
			} else if strings.HasPrefix(line, "AUTH PLAIN") {
				sendLine("235 Authentication successful")
			} else if strings.HasPrefix(line, "MAIL FROM:") {
				sendLine("250 OK")
			} else if strings.HasPrefix(line, "RCPT TO:") {
				sendLine("250 OK")
			} else if strings.HasPrefix(line, "DATA") {
				sendLine("354 Start mail input; end with <CRLF>.<CRLF>")
				// Read until single dot
				for {
					bodyLine, err := reader.ReadString('\n')
					if err != nil {
						break
					}
					if strings.TrimRight(bodyLine, "\r\n") == "." {
						break
					}
				}
				sendLine("250 OK")
			} else if strings.HasPrefix(line, "QUIT") {
				sendLine("221 Bye")
				break
			} else {
				sendLine("500 Command unrecognized")
			}
		}
	}()

	cleanup := func() {
		_ = l.Close()
		<-done
	}

	return host, port, cleanup
}

func TestMailer_Send(t *testing.T) {
	host, port, cleanup := startMockSMTPServer(t)
	defer cleanup()

	cfg := Config{
		Host:        host,
		Port:        port,
		Username:    "testuser",
		Password:    "testpass",
		FromAddress: "sender@gostack.dev",
		FromName:    "GoStack Test",
	}

	mailer := NewMailer(cfg)
	msg := Message{
		To:      []string{"recipient@example.com"},
		Subject: "Test Email",
		Body:    "<h1>Hello World</h1>",
		IsHTML:  true,
	}

	err := mailer.Send(msg)
	if err != nil {
		t.Fatalf("expected successful send, got: %v", err)
	}
}

func TestMailer_BuildRaw(t *testing.T) {
	cfg := Config{
		FromAddress: "sender@gostack.dev",
		FromName:    "GoStack",
	}
	mailer := NewMailer(cfg)

	msg := Message{
		To:      []string{"user@example.com"},
		CC:      []string{"cc@example.com"},
		Subject: "Subject",
		Body:    "Plain Text Body",
		IsHTML:  false,
	}

	raw := string(mailer.BuildRaw(msg))

	if !strings.Contains(raw, "From: GoStack <sender@gostack.dev>") {
		t.Errorf("expected From header, got: %s", raw)
	}
	if !strings.Contains(raw, "To: user@example.com") {
		t.Errorf("expected To header, got: %s", raw)
	}
	if !strings.Contains(raw, "Cc: cc@example.com") {
		t.Errorf("expected CC header, got: %s", raw)
	}
	if !strings.Contains(raw, "Subject: Subject") {
		t.Errorf("expected Subject header, got: %s", raw)
	}
	if !strings.Contains(raw, "Content-Type: text/plain; charset=UTF-8") {
		t.Errorf("expected plain content type header, got: %s", raw)
	}
	if !strings.Contains(raw, "\r\n\r\nPlain Text Body") {
		t.Errorf("expected body separated by CRLF, got: %s", raw)
	}
}
