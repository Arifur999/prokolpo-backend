// Package mail sends the API's transactional email.
package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Message is one plain-text email.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Sender delivers a Message.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// LogSender writes messages to the log instead of sending them. Development
// only: bodies can hold reset links, so config refuses it in production.
type LogSender struct {
	Log *slog.Logger
}

// Send logs msg.
func (s LogSender) Send(ctx context.Context, msg Message) error {
	s.Log.InfoContext(ctx, "email (not sent, log mailer)", "to", msg.To, "subject", msg.Subject, "body", msg.Body)
	return nil
}

// SMTPSender sends through an SMTP server that offers STARTTLS.
type SMTPSender struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// Send delivers msg, giving up after 15 seconds.
func (s SMTPSender) Send(ctx context.Context, msg Message) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(s.Host, s.Port))
	if err != nil {
		return fmt.Errorf("dial smtp: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline) // best effort; a failure surfaces on the next read or write
	}
	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		_ = conn.Close() // the handshake failed; the error below says why
		return fmt.Errorf("smtp handshake: %w", err)
	}
	defer client.Close()

	if err = client.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("smtp starttls: %w", err)
	}
	if s.Username != "" {
		if err = client.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err = client.Mail(s.From); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err = client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err = w.Write(compose(s.From, msg)); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("finish message: %w", err)
	}
	return client.Quit()
}

// compose builds the RFC 5322 message; the Bangla subject is MIME-encoded.
func compose(from string, msg Message) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\n", from, msg.To, mime.BEncoding.Encode("utf-8", msg.Subject))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(strings.ReplaceAll(msg.Body, "\n", "\r\n"))
	return []byte(b.String())
}
