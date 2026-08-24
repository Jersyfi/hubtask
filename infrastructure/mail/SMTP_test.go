// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail_test

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	port "github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/infrastructure/mail"
)

// server is a scripted SMTP server on a real socket. Enough of RFC 5321 for the adapter's
// conversation and nothing more - the point is to prove what the adapter writes, not to be a mail
// server.
type server struct {
	listener net.Listener
	mu       sync.Mutex
	received []string
	done     chan struct{}
	// refuseRecipient makes the server answer RCPT the way a real one refuses an address: with
	// the address quoted back in the reply text.
	refuseRecipient bool
}

func newServer(t *testing.T) *server {
	t.Helper()
	var config net.ListenConfig
	listener, err := config.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	s := &server{listener: listener, done: make(chan struct{})}
	// Through the guard like every other goroutine in this codebase (ADR-0016, rule 5): a panic in
	// a test's fake server would otherwise take the whole package down with no line number.
	concurrency.Go(context.Background(), "test.smtp_server", func(context.Context) { s.accept() })
	t.Cleanup(func() {
		_ = listener.Close()
		<-s.done
	})
	return s
}

func (s *server) address() (string, int) {
	addr := s.listener.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

func (s *server) accept() {
	defer close(s.done)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.converse(conn)
	}
}

func (s *server) converse(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	reader := bufio.NewReader(conn)
	write := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	write("220 test.invalid ESMTP")

	var body strings.Builder
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		if inData {
			if line == "." {
				inData = false
				s.mu.Lock()
				s.received = append(s.received, body.String())
				s.mu.Unlock()
				body.Reset()
				write("250 2.0.0 Ok")
				continue
			}
			body.WriteString(line + "\n")
			continue
		}

		switch {
		case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
			// No STARTTLS and no AUTH advertised: this server is the "none" configuration, which
			// is the one a test can speak without a certificate.
			write("250-test.invalid")
			write("250 SIZE 10240000")
		case strings.HasPrefix(line, "RCPT TO"):
			if s.refuseRecipient {
				write("550 5.1.1 <anna@test.invalid>: Recipient address rejected")
				continue
			}
			write("250 2.0.0 Ok")
		case strings.HasPrefix(line, "MAIL FROM"):
			write("250 2.0.0 Ok")
		case line == "DATA":
			inData = true
			write("354 End data with <CR><LF>.<CR><LF>")
		case line == "QUIT":
			write("221 2.0.0 Bye")
			return
		default:
			write("250 2.0.0 Ok")
		}
	}
}

func (s *server) messages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.received...)
}

func configFor(s *server) env.MailConfig {
	host, port := s.address()
	return env.MailConfig{
		Host: host, Port: port, From: "hubtask@test.invalid",
		Security: env.MailSecurityNone, Timeout: 5 * time.Second,
	}
}

func TestAMessageReachesTheServer(t *testing.T) {
	smtpServer := newServer(t)
	sender := mail.NewSMTP(configFor(smtpServer))

	err := sender.Send(context.Background(), port.Message{
		To:      "anna@test.invalid",
		Subject: "Anna commented on Review the quote",
		Body:    "Anna wrote on “Review the quote”.\nhttps://hub.test.invalid/items/1",
	})
	if err != nil {
		t.Fatalf("sending: %v", err)
	}

	messages := smtpServer.messages()
	if len(messages) != 1 {
		t.Fatalf("the server received %d messages, want 1", len(messages))
	}
	message := messages[0]

	for _, want := range []string{
		"From: hubtask@test.invalid",
		"To: anna@test.invalid",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		// RFC 3834: so that an out-of-office reply does not bounce around a notification.
		"Auto-Submitted: auto-generated",
		"https://hub.test.invalid/items/1",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("the message does not carry %q:\n%s", want, message)
		}
	}
	if !strings.Contains(message, "Subject: ") {
		t.Errorf("no subject header:\n%s", message)
	}
}

// A title is user content, and content is not ASCII. An unencoded subject reaches a mail client as
// mojibake, which is a bug report about the sender rather than about the title.
func TestANonASCIISubjectIsEncoded(t *testing.T) {
	smtpServer := newServer(t)
	sender := mail.NewSMTP(configFor(smtpServer))

	if err := sender.Send(context.Background(), port.Message{
		To: "anna@test.invalid", Subject: "Angebot prüfen — jetzt", Body: "…",
	}); err != nil {
		t.Fatalf("sending: %v", err)
	}

	message := smtpServer.messages()[0]
	subject := headerOf(t, message, "Subject")
	if strings.Contains(subject, "prüfen") {
		t.Errorf("the subject travelled unencoded: %q", subject)
	}
	if !strings.HasPrefix(subject, "=?utf-8?") {
		t.Errorf("the subject is not RFC 2047 encoded: %q", subject)
	}
}

// The SMTP half of T-06's reasoning: a header separator in user content ends the header and starts
// whatever follows it.
func TestHeaderInjectionIsRefusedBeforeAnythingIsDialled(t *testing.T) {
	// No server at all: a refusal that reached the network would be a refusal that came too late.
	sender := mail.NewSMTP(env.MailConfig{
		Host: "127.0.0.1", Port: 1, From: "hubtask@test.invalid",
		Security: env.MailSecurityNone, Timeout: time.Second,
	})

	for _, tc := range []struct {
		name    string
		message port.Message
		detail  string
	}{
		{"a newline in the subject",
			port.Message{To: "anna@test.invalid", Subject: "Hello\r\nBcc: everyone@test.invalid"},
			"mail.header_injection"},
		{"a newline in the address",
			port.Message{To: "anna@test.invalid\nBcc: everyone@test.invalid", Subject: "Hello"},
			"mail.header_injection"},
		{"nowhere to send", port.Message{To: "   ", Subject: "Hello"},
			"mail.recipient_missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := sender.Send(context.Background(), tc.message)
			if err == nil {
				t.Fatal("accepted")
			}
			if got := shared.AsError(err).DetailCode; got != tc.detail {
				t.Errorf("detail %q, want %q", got, tc.detail)
			}
		})
	}
}

// An unreachable server is a dependency that is down, coded as such rather than surfaced as a
// transport message (T-18).
func TestAnUnreachableServerIsACodedDependencyFailure(t *testing.T) {
	sender := mail.NewSMTP(env.MailConfig{
		Host: "127.0.0.1", Port: 1, From: "hubtask@test.invalid",
		Security: env.MailSecurityNone, Timeout: time.Second,
	}).WithDial(func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("connection refused")
	})

	err := sender.Send(context.Background(), port.Message{
		To: "anna@test.invalid", Subject: "Hello", Body: "there",
	})
	if err == nil {
		t.Fatal("an unreachable server was reported as a success")
	}
	domainErr := shared.AsError(err)
	if domainErr.DetailCode != "dependency.unavailable" {
		t.Errorf("detail %q, want dependency.unavailable", domainErr.DetailCode)
	}
	if domainErr.Params["dependency"] != "smtp" {
		t.Errorf("the failure does not name the dependency: %v", domainErr.Params)
	}
}

// The reason replyFailure exists: a server refusing a recipient quotes the address back, and an
// address in a log is personal data in a log (rule 10). The reply code survives; the text does not.
func TestARefusedRecipientDoesNotCarryTheAddressOut(t *testing.T) {
	smtpServer := newServer(t)
	smtpServer.refuseRecipient = true

	err := mail.NewSMTP(configFor(smtpServer)).Send(context.Background(), port.Message{
		To: "anna@test.invalid", Subject: "Hello", Body: "there",
	})
	if err == nil {
		t.Fatal("a refused recipient was reported as a success")
	}
	if strings.Contains(err.Error(), "anna@test.invalid") {
		t.Errorf("the address travelled out with the error: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "550") {
		t.Errorf("the reply code did not survive, and it is what an operator acts on: %q",
			err.Error())
	}
}

// An installation that never set HUBTASK_SMTP_HOST is not broken; it is one whose notifications
// wait. Reported as the dependency being down, which is what /meta/health says about it.
func TestNoServerConfiguredIsADependencyThatIsDown(t *testing.T) {
	err := mail.NewSMTP(env.MailConfig{From: "hubtask@test.invalid"}).
		Send(context.Background(), port.Message{To: "anna@test.invalid", Subject: "Hello"})
	if err == nil {
		t.Fatal("a message was accepted with no mail server configured")
	}
	if got := shared.AsError(err).DetailCode; got != "dependency.unavailable" {
		t.Errorf("detail %q, want dependency.unavailable", got)
	}
}

// Rule 7: a call with no deadline of its own still has one.
func TestACallWithoutADeadlineGetsTheConfiguredOne(t *testing.T) {
	var seen bool
	sender := mail.NewSMTP(env.MailConfig{
		Host: "127.0.0.1", Port: 1, From: "hubtask@test.invalid",
		Security: env.MailSecurityNone, Timeout: 250 * time.Millisecond,
	}).WithDial(func(ctx context.Context, _, _ string) (net.Conn, error) {
		_, seen = ctx.Deadline()
		return nil, errors.New("refused")
	})

	_ = sender.Send(context.Background(), port.Message{To: "anna@test.invalid", Subject: "Hi"})
	if !seen {
		t.Error("the dial ran with no deadline")
	}
}

// STARTTLS asked for and not offered is refused rather than downgraded: a server that does not
// offer it is either misconfigured or not the server the operator thinks it is.
func TestStartTLSIsNotDowngraded(t *testing.T) {
	smtpServer := newServer(t)
	config := configFor(smtpServer)
	config.Security = env.MailSecurityStartTLS

	err := mail.NewSMTP(config).Send(context.Background(), port.Message{
		To: "anna@test.invalid", Subject: "Hello", Body: "there",
	})
	if err == nil {
		t.Fatal("the adapter sent in clear text after being asked for STARTTLS")
	}
	if got := shared.AsError(err).DetailCode; got != "dependency.unavailable" {
		t.Errorf("detail %q, want dependency.unavailable", got)
	}
	if len(smtpServer.messages()) != 0 {
		t.Error("a message reached the server over an unprotected connection")
	}
}

func headerOf(t *testing.T, message, name string) string {
	t.Helper()
	for _, line := range strings.Split(message, "\n") {
		if strings.HasPrefix(line, name+": ") {
			return strings.TrimSpace(strings.TrimPrefix(line, name+": "))
		}
	}
	t.Fatalf("no %s header in:\n%s", name, message)
	return ""
}
