// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mailbox

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/mailbox"
)

const (
	// maxHold keeps an IDLE below the 29 minutes after which RFC 2177 lets a server drop it.
	maxHold = 25 * time.Minute
	// signOffTimeout bounds ending the IDLE and signing out once something arrived.
	signOffTimeout = 30 * time.Second
)

// Watch signs in, selects the folder and holds it in IDLE until the server announces a message
// (an EXISTS) or hold passes. It reads nothing: the read stays the job's, which takes what arrived.
func (i IMAP) Watch(ctx context.Context, connection port.Connection, hold time.Duration) (bool, error) {
	if hold <= 0 || hold > maxHold {
		hold = maxHold
	}
	session, err := i.open(ctx, connection, hold+signOffTimeout+sessionTimeout)
	if err != nil {
		return false, err
	}
	defer func() { _ = session.conn.Close() }()
	// The capabilities after signing in are the ones that count (RFC 9051 §6.1.1).
	capabilities, err := session.command("CAPABILITY")
	if err != nil {
		return false, unreachable(err)
	}
	if !idles(capabilities) {
		_, _ = session.command("LOGOUT")
		return false, idleUnsupported()
	}
	// A context that ends while the read blocks ends the read: the deadline moves to now.
	stop := context.AfterFunc(ctx, func() { _ = session.conn.SetReadDeadline(time.Now()) })
	defer stop()

	session.tag++
	tag := "h" + strconv.Itoa(session.tag)
	if _, err := io.WriteString(session.conn, tag+" IDLE\r\n"); err != nil {
		return false, unreachable(err)
	}
	first, err := session.readLine()
	if err != nil {
		return false, cancelledOr(ctx, unreachable(err))
	}
	if !strings.HasPrefix(first, "+") {
		return false, idleUnsupported()
	}
	_ = session.conn.SetReadDeadline(time.Now().Add(hold))
	arrived := false
	for !arrived {
		line, err := session.readLine()
		if err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			var timeout net.Error
			if errors.As(err, &timeout) && timeout.Timeout() {
				break
			}
			return false, unreachable(err)
		}
		arrived = announcesArrival(line)
		if strings.HasPrefix(line, tag+" ") || strings.HasPrefix(line, "* BYE") {
			// The server ended the IDLE itself: whatever it held, a read finds.
			return arrived, nil
		}
	}
	_ = session.conn.SetDeadline(time.Now().Add(signOffTimeout))
	if _, err := io.WriteString(session.conn, "DONE\r\n"); err == nil {
		_, _ = session.answer(tag)
		_, _ = session.command("LOGOUT")
	}
	return arrived, nil
}

// idles reports whether the CAPABILITY answer names IDLE.
func idles(capabilities response) bool {
	for _, line := range capabilities.lines {
		if rest, ok := strings.CutPrefix(strings.ToUpper(line.text), "* CAPABILITY "); ok {
			for _, name := range strings.Fields(rest) {
				if name == "IDLE" {
					return true
				}
			}
		}
	}
	return false
}

// announcesArrival reads "* 4 EXISTS", the line that says the folder holds a new message.
func announcesArrival(line string) bool {
	fields := strings.Fields(line)
	return len(fields) == 3 && fields[0] == "*" && strings.EqualFold(fields[2], "EXISTS")
}

func idleUnsupported() error {
	return shared.ErrUnavailable.WithDetail("mail.mailbox_idle_unsupported")
}

func cancelledOr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
