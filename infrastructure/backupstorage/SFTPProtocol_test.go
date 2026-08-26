// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backupstorage

import (
	"errors"
	"testing"
)

// The hand-written SFTP client's framing. That the protocol works against a real server is BK-1's
// job; what is here is the arithmetic that would go wrong silently.

// The frame header is a uint32, and a body that exceeded it would wrap rather than fail - the
// target would then read the next four gigabytes as packets nobody sent. Everything that reaches
// `send` is bounded by construction; this is the bound stated where the header is written rather
// than assumed of every caller.
func TestAPacketLargerThanTheBoundIsRefusedRatherThanWrapped(t *testing.T) {
	session := &sftpSession{stream: refusingStream{}}

	if err := session.send(fxpWrite, make([]byte, maxPacket)); err == nil {
		t.Fatal("a packet that cannot be framed was written")
	} else if errors.Is(err, errStreamRefused) {
		t.Error("a packet that cannot be framed reached the stream")
	}

	// And one that fits reaches the stream, so the bound is a bound rather than a wall.
	if err := session.send(fxpWrite, make([]byte, 8)); !errors.Is(err, errStreamRefused) {
		t.Errorf("a packet that fits failed with %v rather than at the stream", err)
	}
}

// refusingStream is a connection that accepts nothing, so that "reached the stream" is
// distinguishable from "was refused before it".
type refusingStream struct{}

var errStreamRefused = errors.New("the stream refused everything")

func (refusingStream) Read([]byte) (int, error)  { return 0, errStreamRefused }
func (refusingStream) Write([]byte) (int, error) { return 0, errStreamRefused }
func (refusingStream) Close() error              { return nil }
