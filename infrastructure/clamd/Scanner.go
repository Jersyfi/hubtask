// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package clamd is the upload scanner (core/port/storage.Scanner) against a ClamAV daemon, over
// its INSTREAM command: the bytes stream in length-prefixed chunks, never a path the daemon reads
// (security.md §4, T-11). A daemon that cannot be reached, times out or answers an error is
// ErrUnavailable, which holds the file rather than passing it.
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

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

// chunkSize stays far below clamd's StreamMaxLength granularity; the daemon caps the whole stream.
const chunkSize = 64 << 10

// Dial opens the connection to the daemon - a field so a test can script the other end.
type Dial func(ctx context.Context, network, address string) (net.Conn, error)

// Scanner scans through one daemon. Address is `host:port` or `unix:/path/to/clamd.sock`.
type Scanner struct {
	Address string
	// Timeout bounds one scan, connection to verdict (rule 7); zero is two minutes.
	Timeout time.Duration
	Dial    Dial
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
	dial := s.Dial
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	conn, err := dial(ctx, network, address)
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
	if err := stream(conn, content); err != nil {
		return storage.Verdict{}, err
	}
	reply, err := bufio.NewReader(conn).ReadString(0)
	if err != nil && !errors.Is(err, io.EOF) {
		return storage.Verdict{}, unavailable("reading the verdict", err)
	}
	return verdictOf(strings.TrimRight(reply, "\x00\n"))
}

// stream writes the content in length-prefixed chunks and the zero-length chunk that ends it.
func stream(conn net.Conn, content io.Reader) error {
	buffer := make([]byte, chunkSize)
	prefix := make([]byte, 4)
	for {
		n, readErr := content.Read(buffer)
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
	default:
		// "INSTREAM size limit exceeded. ERROR" among others: no verdict, so the file is held.
		return storage.Verdict{}, unavailable("the scanner answered without a verdict", errors.New(answer))
	}
}

func unavailable(doing string, err error) error {
	return shared.ErrUnavailable.
		WithDetail("dependency.scanner_unavailable").
		WithCause(fmt.Errorf("%s: %w", doing, err))
}
