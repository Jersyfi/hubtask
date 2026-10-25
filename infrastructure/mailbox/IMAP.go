// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package mailbox reads connected mailboxes for the mail intake (ADR-0094): IMAP spoken here, in
// the few commands a read needs, and JMAP over the guarded HTTP client. In-house rather than a
// library, for ADR-0085's reason: what is needed is small, and a dependency for it is a supply
// chain for a handful of commands.
package mailbox

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

const (
	// maxMessageBytes is the largest message taken; a bigger one stays in the mailbox. The intake's
	// own limits bound what it stores; this bounds what is read off the wire at all.
	maxMessageBytes = 30 << 20
	// maxLineBytes bounds a response line, literals aside.
	maxLineBytes = 64 << 10
	dialTimeout  = 20 * time.Second
	// sessionTimeout bounds one read end to end: a server that stops answering holds no worker.
	sessionTimeout = 2 * time.Minute
)

// IMAP reads a mailbox over IMAP with implicit TLS (port 993).
type IMAP struct {
	Guard *httpclient.Guard
	// Dial replaces the guarded TLS dial, for tests that speak to a server in the same process.
	Dial func(ctx context.Context, address, host string) (net.Conn, error)
}

var _ port.Reader = IMAP{}

// Open signs in and selects the folder.
func (i IMAP) Open(ctx context.Context, connection port.Connection) (port.Session, error) {
	box := connection.Mailbox
	conn, err := i.dial(ctx, box.Endpoint(), box.Host)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(sessionTimeout)
	if until, ok := ctx.Deadline(); ok && until.Before(deadline) {
		deadline = until
	}
	_ = conn.SetDeadline(deadline)
	session := &imapSession{conn: conn, reader: bufio.NewReaderSize(conn, 32<<10), done: box.DoneFolder, since: box.CreatedAt}
	if _, err := session.readLine(); err != nil {
		_ = conn.Close()
		return nil, unreachable(err)
	}
	if err := session.login(box.Username, string(connection.Secret.Reveal())); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := session.command("SELECT " + quote(box.Folder)); err != nil {
		_ = session.Close()
		return nil, folderMissing(err)
	}
	return session, nil
}

func (i IMAP) dial(ctx context.Context, address, host string) (net.Conn, error) {
	if i.Dial != nil {
		return i.Dial(ctx, address, host)
	}
	if i.Guard == nil {
		return nil, shared.ErrInternal.WithDetail("mail.mailbox_unwired")
	}
	// The same checks the HTTP adapters get from GuardedClient, applied by hand because IMAP is not
	// HTTP: the allowlist, then resolution, then the address at dial time, which cannot be raced.
	if err := i.Guard.CheckHost(host); err != nil {
		return nil, err
	}
	if _, err := i.Guard.Resolve(ctx, host); err != nil {
		return nil, err
	}
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{
			Timeout: dialTimeout,
			Control: func(network, address string, _ syscall.RawConn) error {
				return i.Guard.CheckControl(network, address)
			},
		},
		Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12},
	}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, unreachable(err)
	}
	return conn, nil
}

type imapSession struct {
	conn   net.Conn
	reader *bufio.Reader
	tag    int
	done   string
	since  time.Time
	// created is whether the done folder is known to exist.
	created bool
	// moves is whether the server took UID MOVE; false falls back to COPY, STORE and EXPUNGE.
	moves *bool
}

// response is what one command answered: its untagged lines, each with the literals it carried.
type response struct {
	lines []responseLine
}

type responseLine struct {
	text     string
	literals [][]byte
}

func (s *imapSession) login(user, password string) error {
	if strings.ContainsAny(user+password, "\r\n\x00") {
		return refused(errors.New("credentials hold a line break"))
	}
	if _, err := s.commandWith("LOGIN", user, password); err != nil {
		var refusal statusError
		if errors.As(err, &refusal) {
			return refused(err)
		}
		return unreachable(err)
	}
	return nil
}

// Fetch answers the messages that arrived since the mailbox was connected, oldest first: what
// was in it before stays where it is, so connecting a personal mailbox takes nothing that was
// already there.
func (s *imapSession) Fetch(_ context.Context, limit int) ([]port.Message, error) {
	searched, err := s.command("UID SEARCH SINCE " + s.since.UTC().Format("02-Jan-2006"))
	if err != nil {
		return nil, unreachable(err)
	}
	var uids []string
	for _, line := range searched.lines {
		if rest, ok := strings.CutPrefix(line.text, "* SEARCH"); ok {
			uids = append(uids, strings.Fields(rest)...)
		}
	}
	if len(uids) == 0 {
		return nil, nil
	}
	described, err := s.command("UID FETCH " + strings.Join(uids, ",") + " (UID INTERNALDATE RFC822.SIZE)")
	if err != nil {
		return nil, unreachable(err)
	}
	var wanted []string
	for _, line := range described.lines {
		uid, arrived, size, ok := describedMessage(line.text)
		if !ok || arrived.Before(s.since) || size > maxMessageBytes {
			continue
		}
		wanted = append(wanted, uid)
	}
	var out []port.Message
	for _, uid := range wanted {
		if len(out) >= limit {
			break
		}
		fetched, err := s.command("UID FETCH " + uid + " (UID BODY.PEEK[])")
		if err != nil {
			return out, unreachable(err)
		}
		for _, line := range fetched.lines {
			if len(line.literals) > 0 && strings.Contains(line.text, "FETCH") {
				out = append(out, port.Message{Ref: uid, Raw: line.literals[0]})
				break
			}
		}
	}
	return out, nil
}

// Done moves the message to the done folder, creating it the first time; a server without MOVE
// gets COPY, a deleted flag and an expunge of that one message.
func (s *imapSession) Done(_ context.Context, ref string) error {
	if _, err := strconv.ParseUint(ref, 10, 32); err != nil {
		return shared.ErrInternal.WithDetail("mail.mailbox_reference_invalid")
	}
	if !s.created {
		// ALREADYEXISTS is the answer every time but the first; either way the folder is there.
		_, _ = s.command("CREATE " + quote(s.done))
		s.created = true
	}
	if s.moves == nil || *s.moves {
		_, err := s.command("UID MOVE " + ref + " " + quote(s.done))
		if err == nil {
			moves := true
			s.moves = &moves
			return nil
		}
		var refusal statusError
		if !errors.As(err, &refusal) {
			return unreachable(err)
		}
		moves := false
		s.moves = &moves
	}
	if _, err := s.command("UID COPY " + ref + " " + quote(s.done)); err != nil {
		return unreachable(err)
	}
	if _, err := s.command("UID STORE " + ref + " +FLAGS.SILENT (\\Deleted)"); err != nil {
		return unreachable(err)
	}
	if _, err := s.command("UID EXPUNGE " + ref); err != nil {
		// Without UIDPLUS: the message is flagged and goes with the next expunge of the folder.
		if _, err := s.command("EXPUNGE"); err != nil {
			return unreachable(err)
		}
	}
	return nil
}

func (s *imapSession) Close() error {
	_, _ = s.command("LOGOUT")
	return s.conn.Close()
}

// statusError is a tagged NO or BAD: the server understood and refused.
type statusError struct{ status string }

func (e statusError) Error() string { return "imap: " + e.status }

// command sends one tagged command and reads until its tagged answer.
func (s *imapSession) command(text string) (response, error) {
	s.tag++
	tag := "h" + strconv.Itoa(s.tag)
	if _, err := io.WriteString(s.conn, tag+" "+text+"\r\n"); err != nil {
		return response{}, err
	}
	return s.answer(tag)
}

// commandWith sends a command whose arguments are strings: each as a quoted string where it can be
// one, and as a synchronising literal (RFC 9051 §4.3) where it holds what a quoted string cannot -
// a password with an umlaut. Every server takes a synchronising literal; not every one LITERAL+.
func (s *imapSession) commandWith(name string, arguments ...string) (response, error) {
	s.tag++
	tag := "h" + strconv.Itoa(s.tag)
	pending := tag + " " + name
	for _, argument := range arguments {
		if printable(argument) {
			pending += " " + quote(argument)
			continue
		}
		if _, err := io.WriteString(s.conn, pending+" {"+strconv.Itoa(len(argument))+"}\r\n"); err != nil {
			return response{}, err
		}
		line, err := s.readLine()
		if err != nil {
			return response{}, err
		}
		if !strings.HasPrefix(line, "+") {
			return response{}, statusError{status: "NO"}
		}
		pending = argument
	}
	if _, err := io.WriteString(s.conn, pending+"\r\n"); err != nil {
		return response{}, err
	}
	return s.answer(tag)
}

// answer reads until the tagged answer to tag.
func (s *imapSession) answer(tag string) (response, error) {
	var out response
	for {
		line, literals, err := s.readResponseLine()
		if err != nil {
			return response{}, err
		}
		if rest, ok := strings.CutPrefix(line, tag+" "); ok {
			status, _, _ := strings.Cut(rest, " ")
			if strings.EqualFold(status, "OK") {
				return out, nil
			}
			return out, statusError{status: strings.ToUpper(status)}
		}
		if strings.HasPrefix(line, "+") {
			// A continuation nobody asked for: this client sends no synchronising literal.
			return out, errors.New("imap: unexpected continuation")
		}
		out.lines = append(out.lines, responseLine{text: line, literals: literals})
	}
}

// readResponseLine reads one response line with the literals it announces ("{1234}"), which may
// be followed by more of the line.
func (s *imapSession) readResponseLine() (string, [][]byte, error) {
	var text strings.Builder
	var literals [][]byte
	for {
		line, err := s.readLine()
		if err != nil {
			return "", nil, err
		}
		size, ok := literalSize(line)
		if !ok {
			text.WriteString(line)
			return text.String(), literals, nil
		}
		if size > maxMessageBytes {
			return "", nil, errors.New("imap: literal too large")
		}
		text.WriteString(line[:strings.LastIndex(line, "{")])
		data := make([]byte, size)
		if _, err := io.ReadFull(s.reader, data); err != nil {
			return "", nil, err
		}
		literals = append(literals, data)
	}
}

func (s *imapSession) readLine() (string, error) {
	var line []byte
	for {
		chunk, isPrefix, err := s.reader.ReadLine()
		if err != nil {
			return "", err
		}
		line = append(line, chunk...)
		if len(line) > maxLineBytes {
			return "", errors.New("imap: line too long")
		}
		if !isPrefix {
			return string(line), nil
		}
	}
}

// printable reports whether a value can be a quoted string: printable ASCII only.
func printable(value string) bool {
	for _, r := range value {
		if r > 0x7e || r < 0x20 {
			return false
		}
	}
	return true
}

func quote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

// literalSize reads the "{n}" a line ends on.
func literalSize(line string) (int, bool) {
	if !strings.HasSuffix(line, "}") {
		return 0, false
	}
	open := strings.LastIndex(line, "{")
	if open < 0 {
		return 0, false
	}
	size, err := strconv.Atoi(strings.TrimSuffix(line[open+1:len(line)-1], "+"))
	if err != nil || size < 0 {
		return 0, false
	}
	return size, true
}

// describedMessage reads "* 3 FETCH (UID 17 INTERNALDATE "25-Oct-2026 10:00:00 +0200" RFC822.SIZE 2048)".
func describedMessage(line string) (string, time.Time, int, bool) {
	if !strings.Contains(line, " FETCH ") {
		return "", time.Time{}, 0, false
	}
	uid := field(line, "UID ")
	date := ""
	if at := strings.Index(line, `INTERNALDATE "`); at >= 0 {
		rest := line[at+len(`INTERNALDATE "`):]
		if end := strings.Index(rest, `"`); end >= 0 {
			date = rest[:end]
		}
	}
	arrived, err := time.Parse("2-Jan-2006 15:04:05 -0700", strings.TrimSpace(date))
	if err != nil || uid == "" {
		return "", time.Time{}, 0, false
	}
	size, _ := strconv.Atoi(field(line, "RFC822.SIZE "))
	return uid, arrived, size, true
}

func field(line, name string) string {
	at := strings.Index(line, name)
	if at < 0 {
		return ""
	}
	rest := line[at+len(name):]
	end := strings.IndexAny(rest, " )")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func refused(err error) error {
	return shared.ErrValidation.WithDetail("mail.mailbox_refused").
		WithFields(shared.FieldError{Path: "/secret", Code: "mail.mailbox_refused"}).WithCause(err)
}

func unreachable(err error) error {
	var coded *shared.Error
	if errors.As(err, &coded) {
		return err
	}
	return shared.ErrUnavailable.WithDetail("mail.mailbox_unreachable").WithCause(fmt.Errorf("imap: %w", err))
}

func folderMissing(err error) error {
	var refusal statusError
	if errors.As(err, &refusal) {
		return shared.ErrValidation.WithDetail("mail.mailbox_folder_missing").
			WithFields(shared.FieldError{Path: "/folder", Code: "mail.mailbox_folder_missing"}).WithCause(err)
	}
	return unreachable(err)
}
