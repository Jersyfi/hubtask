// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package clamd

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// daemon plays clamd on the far end of a pipe: it reads the command and the chunks, keeps what was
// streamed and answers the reply it was given.
func daemon(t *testing.T, reply string, streamed *bytes.Buffer) Dial {
	t.Helper()
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		concurrency.Go(ctx, "clamd.test", func(context.Context) {
			defer func() { _ = server.Close() }()
			command := make([]byte, len("zINSTREAM\x00"))
			if _, err := io.ReadFull(server, command); err != nil || string(command) != "zINSTREAM\x00" {
				return
			}
			for {
				prefix := make([]byte, 4)
				if _, err := io.ReadFull(server, prefix); err != nil {
					return
				}
				size := binary.BigEndian.Uint32(prefix)
				if size == 0 {
					break
				}
				if _, err := io.CopyN(streamed, server, int64(size)); err != nil {
					return
				}
			}
			_, _ = io.WriteString(server, reply+"\x00")
		})
		return client, nil
	}
}

func TestAFileIsStreamedWholeAndItsVerdictRead(t *testing.T) {
	body := bytes.Repeat([]byte("x"), chunkSize*2+17)
	for _, tc := range []struct {
		reply     string
		malicious bool
		signature string
	}{
		{"stream: OK", false, ""},
		{"stream: Eicar-Test-Signature FOUND", true, "Eicar-Test-Signature"},
	} {
		streamed := &bytes.Buffer{}
		verdict, err := Scanner{Address: "clamd:3310", Dial: daemon(t, tc.reply, streamed)}.Scan(t.Context(), bytes.NewReader(body))
		if err != nil || verdict.Malicious != tc.malicious || verdict.Signature != tc.signature {
			t.Errorf("%q: %+v, %v", tc.reply, verdict, err)
		}
		if !bytes.Equal(streamed.Bytes(), body) {
			t.Errorf("%q: the daemon received %d of %d bytes", tc.reply, streamed.Len(), len(body))
		}
	}
}

// No verdict - a daemon that answers an error, or none that can be reached - is unavailable, which
// holds the file.
func TestNoVerdictIsUnavailable(t *testing.T) {
	streamed := &bytes.Buffer{}
	_, err := Scanner{Dial: daemon(t, "INSTREAM size limit exceeded. ERROR", streamed)}.Scan(t.Context(), strings.NewReader("x"))
	if !errors.Is(err, shared.ErrUnavailable) || shared.AsError(err).DetailCode != "dependency.scanner_unavailable" {
		t.Errorf("an error reply was answered %v", err)
	}
	refused := func(context.Context, string, string) (net.Conn, error) { return nil, errors.New("connection refused") }
	if _, err := (Scanner{Dial: refused}).Scan(t.Context(), strings.NewReader("x")); !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("an unreachable daemon was answered %v", err)
	}
}

// A TCP daemon is dialled through the guard: an address the guard refuses - a metadata address,
// a host off the allowlist - is never connected, so no byte of the file leaves, and the refusal
// is unavailable, which holds the file.
func TestARefusedAddressIsNeverDialled(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	accepted := make(chan struct{}, 1)
	concurrency.Go(t.Context(), "clamd.test.listener", func(context.Context) {
		if conn, err := listener.Accept(); err == nil {
			accepted <- struct{}{}
			_ = conn.Close()
		}
	})
	_, port, _ := net.SplitHostPort(listener.Addr().String())

	metadata := httpclient.NewGuard(env.OutboundConfig{}).ForOperatorService().
		WithResolver(func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("169.254.169.254")}, nil
		})
	for name, tc := range map[string]Scanner{
		"metadata address":      {Address: "169.254.169.254:80", Guard: httpclient.NewGuard(env.OutboundConfig{}).ForOperatorService()},
		"resolves to metadata":  {Address: "clamav:" + port, Guard: metadata},
		"off the allowlist":     {Address: "127.0.0.1:" + port, Guard: httpclient.NewGuard(env.OutboundConfig{AllowedHosts: []string{"clamav"}}).ForOperatorService()},
		"private, guard closed": {Address: "127.0.0.1:" + port, Guard: httpclient.NewGuard(env.OutboundConfig{})},
		"no guard":              {Address: "127.0.0.1:" + port},
	} {
		_, err := tc.Scan(t.Context(), strings.NewReader("the file"))
		if !errors.Is(err, shared.ErrUnavailable) || shared.AsError(err).DetailCode != "dependency.scanner_unavailable" {
			t.Errorf("%s: answered %v", name, err)
		}
	}
	select {
	case <-accepted:
		t.Error("a refused address was connected")
	case <-time.After(50 * time.Millisecond):
	}
}

// An allowed daemon on the operator's own network is dialled through the guard and scans.
func TestAnAllowedDaemonIsDialledThroughTheGuard(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	streamed := &bytes.Buffer{}
	serve := daemon(t, "stream: OK", streamed)
	concurrency.Go(t.Context(), "clamd.test.listener", func(ctx context.Context) {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		pipe, _ := serve(ctx, "", "")
		concurrency.Go(ctx, "clamd.test.copy", func(context.Context) { _, _ = io.Copy(pipe, conn) })
		_, _ = io.Copy(conn, pipe)
		_ = conn.Close()
	})
	guard := httpclient.NewGuard(env.OutboundConfig{AllowedHosts: []string{"127.0.0.1"}}).ForOperatorService()
	verdict, err := Scanner{Address: listener.Addr().String(), Guard: guard, Timeout: 5 * time.Second}.
		Scan(t.Context(), strings.NewReader("the file"))
	if err != nil || verdict.Malicious || streamed.String() != "the file" {
		t.Errorf("an allowed daemon answered %+v, %v, streamed %q", verdict, err, streamed.String())
	}
}
