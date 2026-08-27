// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

func triggerCursors() security.TriggerCursorCodec {
	return security.NewTriggerCursorCodec(secret.New("installation secret"))
}

func triggerPosition(t *testing.T, at time.Time) outbox.Position {
	t.Helper()

	id, err := shared.ParseID("01a04489-d819-752a-91ae-85e8bf4f236b")
	if err != nil {
		t.Fatalf("the fixture identifier does not parse: %v", err)
	}
	return outbox.Position{OccurredAt: at, ID: id}
}

// Microsecond fidelity is the point of the type: the moment is half of a sort key, and a cursor
// rounded to the second would re-answer or step over every event sharing that second.
func TestATriggerCursorRoundTripsToTheMicrosecond(t *testing.T) {
	codec := triggerCursors()

	for _, at := range []time.Time{
		time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 27, 9, 0, 0, 123_456_000, time.UTC),
		time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
	} {
		position := triggerPosition(t, at)
		back, err := codec.Decode(codec.Encode(position))
		if err != nil {
			t.Fatalf("decoding the cursor for %v: %v", at, err)
		}
		if !back.OccurredAt.Equal(at) {
			t.Errorf("occurred at %v, want %v", back.OccurredAt, at)
		}
		if back.ID != position.ID {
			t.Errorf("id %s, want %s", back.ID, position.ID)
		}
	}
}

// It travels as a query parameter, and a poller round-trips it through its own storage. Base64url
// without padding needs no escaping in either.
func TestATriggerCursorIsSafeInAQueryParameter(t *testing.T) {
	cursor := triggerCursors().Encode(
		triggerPosition(t, time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)))

	if cursor == "" {
		t.Fatal("the cursor is empty")
	}
	if escaped := url.QueryEscape(cursor); escaped != cursor {
		t.Errorf("the cursor needs escaping: %q becomes %q", cursor, escaped)
	}
}

// The whole reason it is signed. A caller that could craft one would be asking to be handed events
// from a window it was never given (T-06).
func TestATamperedTriggerCursorIsRefused(t *testing.T) {
	codec := triggerCursors()
	cursor := triggerCursors().Encode(
		triggerPosition(t, time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)))

	mutations := map[string]string{
		"a flipped byte":  flipLast(cursor),
		"truncated":       cursor[:len(cursor)-4],
		"empty":           "",
		"not base64":      "not a cursor!!",
		"only a tag":      cursor[:8],
		"another payload": triggerCursors().Encode(outbox.Position{}),
	}
	for name, forged := range mutations {
		t.Run(name, func(t *testing.T) {
			if _, err := codec.Decode(forged); err == nil {
				t.Errorf("%q was accepted", forged)
			}
		})
	}
}

// A cursor minted by another installation is not a cursor here. Same reasoning as the page and
// stream codecs: the key is derived from the installation secret, so the walk cannot be carried
// from one deployment to another.
func TestATriggerCursorDoesNotTravelBetweenInstallations(t *testing.T) {
	mine := triggerCursors()
	theirs := security.NewTriggerCursorCodec(secret.New("another installation"))

	cursor := theirs.Encode(triggerPosition(t, time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)))
	if _, err := mine.Decode(cursor); err == nil {
		t.Error("a cursor from another installation was accepted")
	}
}

// A stream cursor is also "a position", and the info string is what stops one being accepted where
// the other belongs.
func TestAStreamCursorIsNotATriggerCursor(t *testing.T) {
	cursor := streamCursors().Encode(security.StreamPosition{Seq: 42, IssuedAt: issued})

	if _, err := triggerCursors().Decode(cursor); err == nil {
		t.Error("a stream cursor was accepted as a trigger cursor")
	}
}

// flipLast changes one byte of the encoded value, which is the cheapest forgery there is.
func flipLast(cursor string) string {
	raw := []byte(cursor)
	if raw[len(raw)-1] == 'A' {
		raw[len(raw)-1] = 'B'
	} else {
		raw[len(raw)-1] = 'A'
	}
	return string(raw)
}
