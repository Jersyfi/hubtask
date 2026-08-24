// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package mail is the outbound adapter for the mail port: SMTP, and the health probe that reads
// the same breaker it trips (C-09).
//
// The standard library's net/smtp is what carries it. It is frozen rather than maintained, which
// is precisely the argument for using it here: what this adapter needs is EHLO, STARTTLS, AUTH and
// DATA, none of which has changed since the package was frozen - and a third-party mailer would be
// a supply chain decision (CLAUDE.md, "What you do not decide yourself") bought for nothing.
package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	port "github.com/Jersyfi/hubtask/core/port/mail"
)

// Dependency is the name this adapter appears under in the health report and in the metrics. Short
// and without spaces, as core/port/health requires.
const Dependency = "smtp"

// NotificationsFeature is what a person cares about when the mail server is gone: being told
// things, not SMTP (observability-reliability.md §7). The bare feature name the degradation report
// carries.
const NotificationsFeature = "notifications"

// Dial opens the connection to the mail server. A field rather than a call to net.Dial, so that a
// test can put a scripted server on the other end of a real socket without the adapter knowing -
// the alternative is an SMTP adapter proved only by reading it.
type Dial func(ctx context.Context, network, address string) (net.Conn, error)

// SMTP sends through a mail server.
//
// One value for the whole process, holding no connection: a mail server is reached rarely enough
// that a pool would mostly be idle sockets a firewall eventually drops, and reconnecting per
// message is what makes an outage a failed attempt rather than a stuck one.
type SMTP struct {
	config env.MailConfig
	dial   Dial
}

// NewSMTP builds the sender from the installation's configuration.
func NewSMTP(config env.MailConfig) SMTP {
	return SMTP{config: config, dial: dialTCP}
}

// WithDial replaces the dialler. For the adapter's own tests, and for nothing else.
func (s SMTP) WithDial(dial Dial) SMTP {
	s.dial = dial
	return s
}

var _ port.Sender = SMTP{}

// Send delivers one message.
//
// Every step is inside the caller's deadline, or inside the configured timeout where the caller
// brought none (rule 7). The connection carries it as a socket deadline rather than as a
// cancellation watcher, because net/smtp has no context: a deadline on the connection is what
// actually stops a server that accepted the TCP handshake and then says nothing.
func (s SMTP) Send(ctx context.Context, message port.Message) error {
	if err := validate(message); err != nil {
		return err
	}
	if s.config.Host == "" {
		// Not configured is a dependency that is down rather than a programming error: an
		// installation that never set HUBTASK_SMTP_HOST is one whose notifications wait, which is
		// what /meta/health says about it (observability-reliability.md §7).
		return unavailable(errors.New("no mail server is configured"))
	}

	ctx, cancel := s.deadline(ctx)
	defer cancel()

	address := net.JoinHostPort(s.config.Host, strconv.Itoa(s.config.Port))
	conn, err := s.connect(ctx, address)
	if err != nil {
		return unavailable(err)
	}
	// Closed unconditionally as well as by Quit below: Quit is the polite ending and this is the
	// one that runs when the server hangs up mid-conversation.
	defer func() { _ = conn.Close() }()

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return unavailable(err)
		}
	}

	client, err := smtp.NewClient(conn, s.config.Host)
	if err != nil {
		return unavailable(err)
	}
	defer func() { _ = client.Close() }()

	if err := s.negotiate(client); err != nil {
		return unavailable(err)
	}
	return s.deliver(client, message)
}

// connect opens the socket, wrapped in TLS from the first byte where the configuration says so.
func (s SMTP) connect(ctx context.Context, address string) (net.Conn, error) {
	conn, err := s.dial(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	if s.config.Security != env.MailSecurityTLS {
		return conn, nil
	}

	// Implicit TLS: the server speaks TLS from the first byte, so the handshake happens here
	// rather than after EHLO. The server name is the configured host and never the address, so
	// that a certificate is checked against what the operator named.
	tlsConn := tls.Client(conn, &tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return tlsConn, nil
}

// negotiate raises the connection to TLS where it is not already, and authenticates.
func (s SMTP) negotiate(client *smtp.Client) error {
	if s.config.Security == env.MailSecurityStartTLS {
		ok, _ := client.Extension("STARTTLS")
		if !ok {
			// Refused rather than downgraded. A server that does not offer STARTTLS where the
			// operator asked for it is either misconfigured or not the server they think it is,
			// and sending anyway is the failure mode STARTTLS exists to prevent.
			return errors.New("the mail server does not offer STARTTLS")
		}
		if err := client.StartTLS(
			&tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12},
		); err != nil {
			return err
		}
	}

	if s.config.Username == "" {
		return nil
	}
	// PLAIN over a protected connection. net/smtp refuses to send it in clear text unless the
	// server is localhost, which is the check this adapter would otherwise have to write itself.
	auth := smtp.PlainAuth("", s.config.Username, s.config.Password.Reveal(), s.config.Host)
	if ok, _ := client.Extension("AUTH"); !ok {
		return errors.New("the mail server offers no authentication and credentials are configured")
	}
	return client.Auth(auth)
}

// deliver writes the envelope and the message.
func (s SMTP) deliver(client *smtp.Client, message port.Message) error {
	if err := client.Mail(s.config.From); err != nil {
		return replyFailure("MAIL", err)
	}
	if err := client.Rcpt(message.To); err != nil {
		return replyFailure("RCPT", err)
	}

	writer, err := client.Data()
	if err != nil {
		return replyFailure("DATA", err)
	}
	if _, err := writer.Write([]byte(s.compose(message))); err != nil {
		return replyFailure("DATA", err)
	}
	if err := writer.Close(); err != nil {
		return replyFailure("DATA", err)
	}
	if err := client.Quit(); err != nil {
		return replyFailure("QUIT", err)
	}
	return nil
}

// compose builds the message: the few headers an email needs and the body.
//
// The subject is encoded per RFC 2047, because a title is user content and user content is not
// ASCII. Everything else is generated here - there is no header a caller can set, which is what
// makes header injection a matter of validating two fields rather than auditing a map.
func (s SMTP) compose(message port.Message) string {
	var out strings.Builder
	out.WriteString("From: " + s.config.From + "\r\n")
	out.WriteString("To: " + message.To + "\r\n")
	out.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", message.Subject) + "\r\n")
	out.WriteString("MIME-Version: 1.0\r\n")
	out.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	// So that a mail client and an autoresponder both know this is machine-generated: RFC 3834
	// asks for it, and it is what stops an out-of-office reply bouncing around a notification.
	out.WriteString("Auto-Submitted: auto-generated\r\n")
	out.WriteString("\r\n")
	// Dot-stuffing: a line that is a single dot would end the DATA command. net/smtp's writer
	// does this itself, so the body goes in as it stands.
	out.WriteString(strings.ReplaceAll(message.Body, "\n", "\r\n"))
	return out.String()
}

// deadline gives the call the configured timeout where the caller brought none (rule 7).
func (s SMTP) deadline(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	timeout := s.config.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

func dialTCP(ctx context.Context, network, address string) (net.Conn, error) {
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, address)
}

// validate refuses what must not reach a header.
//
// A rendered subject is a catalogue sentence with a title substituted into it, and a title is user
// content: a newline in one would end the Subject header and start whatever the rest of the title
// says. The same reasoning as T-06 about SQL, applied to a protocol whose header separator is a
// line break.
func validate(message port.Message) error {
	if strings.TrimSpace(message.To) == "" {
		return port.ErrRecipientMissing
	}
	if containsLineBreak(message.To) || containsLineBreak(message.Subject) {
		return port.ErrHeaderInjection
	}
	return nil
}

func containsLineBreak(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}

// unavailable is the error contract: an unreachable server is ErrUnavailable with a `dependency.`
// detail code, never a raw SMTP reply (T-18). The cause is carried rather than printed, so it
// reaches a log without reaching a client - a 5xx reply can quote the recipient address back.
func unavailable(cause error) error {
	return shared.ErrUnavailable.
		WithDetail("dependency.unavailable").
		WithParams(map[string]string{"dependency": Dependency}).
		WithCause(cause)
}

// replyFailure is what a refusal from the server becomes: the step and the reply code, never the
// reply text.
//
// The text is the reason for the whole function. A server answering RCPT quotes the address back -
// "550 5.1.1 <anna@example.com>: Recipient address rejected" - and that address in a log is
// personal data in a log (rule 10). The number says as much as an operator can act on, and the
// step says where the conversation stopped. The same reasoning the S3 adapter applies to an error
// body that carries the key and the bucket.
func replyFailure(step string, err error) error {
	var reply *textproto.Error
	if errors.As(err, &reply) {
		return unavailable(fmt.Errorf("the mail server answered %d at %s", reply.Code, step))
	}
	// Not a reply but a transport failure: the connection went away mid-conversation, and what it
	// carries is this side's view of a socket rather than anything the server said.
	return unavailable(fmt.Errorf("the connection failed at %s: %w", step, err))
}
