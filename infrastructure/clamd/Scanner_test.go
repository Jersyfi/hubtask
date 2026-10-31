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
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
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
