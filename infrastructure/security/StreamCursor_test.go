// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

var issued = time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)

func streamCursors() security.StreamCursorCodec {
	return security.NewStreamCursorCodec(secret.New("installation secret"))
}

func TestAStreamCursorRoundTrips(t *testing.T) {
	codec := streamCursors()

	for _, position := range []security.StreamPosition{
		{Seq: 0, IssuedAt: issued},
		{Seq: 1, IssuedAt: issued},
		{Seq: 9_223_372_036_854_775_807, IssuedAt: issued},
	} {
		cursor := codec.Encode(position)
		back, err := codec.Decode(cursor)
		if err != nil {
			t.Fatalf("decoding %q: %v", cursor, err)
		}
		if back.Seq != position.Seq {
			t.Errorf("seq %d, want %d", back.Seq, position.Seq)
		}
		// Seconds, because the question the moment answers is measured in days.
		if !back.IssuedAt.Equal(position.IssuedAt.Truncate(time.Second)) {
			t.Errorf("issued at %v, want %v", back.IssuedAt, position.IssuedAt)
		}
	}
}

// The cursor travels as `Last-Event-ID`, which a browser resends by itself. It has to survive a
// header and a URL without escaping.
func TestAStreamCursorIsSafeInAHeader(t *testing.T) {
	cursor := streamCursors().Encode(security.StreamPosition{Seq: 42, IssuedAt: issued})

	if cursor == "" {
		t.Fatal("the cursor is empty")
	}
	if strings.ContainsAny(cursor, " \t\r\n:;,=+/") {
		t.Errorf("the cursor needs escaping in a header: %q", cursor)
	}
}

// An unsigned cursor is a query parameter in disguise: a client that could craft one would be
// seeking to an arbitrary position in the log.
func TestAForgedStreamCursorIsRefused(t *testing.T) {
	codec := streamCursors()
	valid := codec.Encode(security.StreamPosition{Seq: 42, IssuedAt: issued})

	for _, tc := range []struct {
		name   string
		cursor string
	}{
		{"empty", ""},
		{"not base64", "!!!!"},
		{"too short", "AAAA"},
		{"a flipped byte", flip(valid)},
		{"the payload without a tag", "NDIuMTc1NjAyMDgwMA"},
		{"from another installation",
			security.NewStreamCursorCodec(secret.New("another installation")).
				Encode(security.StreamPosition{Seq: 42, IssuedAt: issued})},
		{"a page cursor", security.NewCursorCodec(secret.New("installation secret")).
			Encode(security.At("a0", "01936f2a-7c1e-7000-8000-0000000000a1"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := codec.Decode(tc.cursor); err == nil {
				t.Fatalf("%q was accepted", tc.cursor)
			} else if got := shared.AsError(err).DetailCode; got != "sync.cursor_invalid" {
				t.Errorf("detail %q, want sync.cursor_invalid", got)
			}
		})
	}
}

// The domain separation of security.md §8, checked rather than asserted: a value minted for one
// purpose must not be usable as a value of another, even under the same installation secret.
func TestAStreamCursorIsNotAPageCursor(t *testing.T) {
	installation := secret.New("installation secret")
	stream := security.NewStreamCursorCodec(installation)
	pages := security.NewCursorCodec(installation)

	cursor := stream.Encode(security.StreamPosition{Seq: 42, IssuedAt: issued})
	if _, err := pages.Decode(cursor); err == nil {
		t.Error("a stream cursor was accepted as a page cursor")
	}
}

// flip changes one byte of an encoded cursor, leaving its length and alphabet intact.
func flip(cursor string) string {
	bytes := []byte(cursor)
	if bytes[0] == 'A' {
		bytes[0] = 'B'
	} else {
		bytes[0] = 'A'
	}
	return string(bytes)
}
