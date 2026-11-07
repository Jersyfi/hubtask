// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package clamd is the upload scanner (core/port/storage.Scanner) against a ClamAV daemon, over
// its INSTREAM command: the bytes stream in length-prefixed chunks, never a path the daemon reads
// (security.md §4, T-11). A daemon that cannot be reached, times out or answers an error is
// ErrUnavailable, which holds the file rather than passing it - except a refusal of the file for its
// size (media.TooLargeToScan), which no later attempt would turn into a verdict. A TCP daemon is dialled through
// the outbound guard (rule 6): the allowlist, resolution and the address at dial time.
package clamd

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// chunkSize stays far below clamd's StreamMaxLength granularity; the daemon caps the whole stream.
const chunkSize = 64 << 10

// Dial opens the connection to the daemon - a field so a test can script the other end.
type Dial func(ctx context.Context, network, address string) (net.Conn, error)

// defaultConnectTimeout bounds the connection on its own where the guard's configuration names
// none, so a daemon that accepts nothing fails fast instead of consuming the scan's budget.
const defaultConnectTimeout = 5 * time.Second

// Scanner scans through one daemon. Address is `host:port` or `unix:/path/to/clamd.sock`.
type Scanner struct {
	Address string
	// Timeout bounds one scan, connection to verdict (rule 7); zero is two minutes.
	Timeout time.Duration
	// ConnectTimeout bounds the connection attempt; zero is five seconds.
	ConnectTimeout time.Duration
	// Guard checks a TCP daemon's host and address; without it a TCP address is never dialled.
	Guard *httpclient.Guard
	Dial  Dial
	// StreamMaxBytes is the daemon's StreamMaxLength; a longer file is refused for its size before
	// it is sent. Zero sends everything and leaves the limit to the daemon's answer.
	StreamMaxBytes int64
}

var _ storage.Scanner = Scanner{}

// Scan streams the content to the daemon and reads its verdict.
func (s Scanner) Scan(ctx context.Context, content io.Reader) (storage.Verdict, error) {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	network, address := "tcp", s.Address
	if path, ok := strings.CutPrefix(s.Address, "unix:"); ok {
		network, address = "unix", path
	}
	conn, err := s.dial(ctx, network, address)
	if err != nil {
		return storage.Verdict{}, unavailable("connecting to the scanner", err)
	}
	defer func() { _ = conn.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return storage.Verdict{}, unavailable("bounding the scan", err)
		}
	}

	if _, err := io.WriteString(conn, "zINSTREAM\x00"); err != nil {
		return storage.Verdict{}, unavailable("starting the scan", err)
	}
	if err := stream(conn, content, s.StreamMaxBytes); err != nil {
		if errors.Is(err, shared.ErrUnavailable) && refusedForSize(conn) {
			// clamd answers a stream over its limit and closes, so the next write fails.
			return storage.Verdict{}, media.TooLargeToScan()
		}
		return storage.Verdict{}, err
	}
	reply, err := bufio.NewReader(conn).ReadString(0)
	if err != nil && !errors.Is(err, io.EOF) {
		return storage.Verdict{}, unavailable("reading the verdict", err)
	}
	return verdictOf(strings.TrimRight(reply, "\x00\n"))
}

// dial connects to the daemon. A unix socket is a file on this host the operator named, not an
// egress, so the guard has nothing to check there; a TCP address passes the same checks
// GuardedClient makes for HTTP, the last of them on the concrete address at dial time.
func (s Scanner) dial(ctx context.Context, network, address string) (net.Conn, error) {
	if s.Dial != nil {
		return s.Dial(ctx, network, address)
	}
	connectTimeout := s.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = defaultConnectTimeout
	}
	if network == "unix" {
		return (&net.Dialer{Timeout: connectTimeout}).DialContext(ctx, network, address)
	}
	if s.Guard == nil {
		return nil, errors.New("no outbound guard is wired")
	}
	return httpclient.NewGuardedDialer(s.Guard, connectTimeout).DialContext(ctx, network, address)
}

// stream writes the content in length-prefixed chunks and the zero-length chunk that ends it.
func stream(conn net.Conn, content io.Reader, limit int64) error {
	buffer := make([]byte, chunkSize)
	prefix := make([]byte, 4)
	var sent int64
	for {
		n, readErr := content.Read(buffer)
		if sent += int64(n); limit > 0 && sent > limit {
			return media.TooLargeToScan()
		}
		if n > 0 {
			binary.BigEndian.PutUint32(prefix, uint32(n)) //nolint:gosec // G115: n <= chunkSize
			if _, err := conn.Write(prefix); err != nil {
				return unavailable("sending to the scanner", err)
			}
			if _, err := conn.Write(buffer[:n]); err != nil {
				return unavailable("sending to the scanner", err)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			// The bytes could not be read: not the scanner's fault, and not a verdict.
			return fmt.Errorf("reading the file to scan: %w", readErr)
		}
	}
	binary.BigEndian.PutUint32(prefix, 0)
	if _, err := conn.Write(prefix); err != nil {
		return unavailable("ending the stream", err)
	}
	return nil
}

// verdictOf reads `stream: OK`, `stream: <signature> FOUND` or an error line.
func verdictOf(reply string) (storage.Verdict, error) {
	answer := strings.TrimSpace(strings.TrimPrefix(reply, "stream:"))
	switch {
	case answer == "OK":
		return storage.Verdict{}, nil
	case strings.HasSuffix(answer, " FOUND"):
		return storage.Verdict{Malicious: true, Signature: strings.TrimSuffix(answer, " FOUND")}, nil
	case sizeRefusal(answer):
		return storage.Verdict{}, media.TooLargeToScan()
	default:
		// Any other error line: no verdict, so the file is held - but the daemon answered, so it
		// is this file's answer, not an outage.
		return storage.Verdict{}, unavailable("the scanner answered without a verdict",
			fmt.Errorf("%w: %s", storage.ErrNoVerdictForFile, answer))
	}
}

// sizeRefusal recognises clamd's "INSTREAM size limit exceeded. ERROR".
func sizeRefusal(answer string) bool {
	return strings.Contains(answer, "size limit exceeded")
}

// refusedForSize reads what the daemon said before a write failed, briefly: an answer is already
// sent or never comes.
func refusedForSize(conn net.Conn) bool {
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		return false
	}
	reply, _ := bufio.NewReader(conn).ReadString(0)
	return sizeRefusal(reply)
}

func unavailable(doing string, err error) error {
	return shared.ErrUnavailable.
		WithDetail("dependency.scanner_unavailable").
		WithCause(fmt.Errorf("%s: %w", doing, err))
}
