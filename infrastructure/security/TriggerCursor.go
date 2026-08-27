// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// triggerCursorInfo separates this use of the installation secret from every other one, as
// cursorInfo, streamCursorInfo and pepperInfo do. A page cursor, a stream cursor and a trigger
// cursor are all "a position", and without the separation one would be accepted where another
// belongs - which here would be a poller seeking into somebody else's walk.
const triggerCursorInfo = "hubtask/trigger-cursor/v1"

// triggerCursorParts is how many fields the payload has: the moment and the identifier that break
// the tie within it.
const triggerCursorParts = 2

// triggerCursorFieldSeparator ends the moment inside the payload. A full stop, because the moment
// is decimal digits and cannot contain one, and the identifier that follows is a uuid.
const triggerCursorFieldSeparator = "."

// TriggerCursorCodec turns a position in the outbox into an opaque cursor and back (G-04).
//
// Signed, for the page cursor's reason and more sharply: this value decides where in the event log
// a caller resumes, and a client that could craft one would be asking to be handed events from a
// window it was never given. Unsigned it would be a query parameter in disguise (T-06).
//
// It lives here rather than in the application layer because it is a cryptographic primitive keyed
// on the installation secret, and the secret has no business above the adapters (security.md §8).
// The application holds it behind an interface and never looks inside the string, which is what
// "opaque" means from that side - and what lets the encoding change without a contract change.
//
// Microseconds rather than seconds, unlike the stream cursor. The stream's moment answers a
// question measured in days; this one is half of a sort key, and a cursor rounded to the second
// would re-answer or step over every event sharing that second. It is the resolution PostgreSQL
// stores timestamptz at, so the value round-trips exactly.
type TriggerCursorCodec struct {
	key []byte
}

func NewTriggerCursorCodec(installationSecret secret.Secret) TriggerCursorCodec {
	mac := hmac.New(sha256.New, []byte(installationSecret.Reveal()))
	mac.Write([]byte(triggerCursorInfo))
	return TriggerCursorCodec{key: mac.Sum(nil)}
}

// Encode returns the cursor for a position.
func (c TriggerCursorCodec) Encode(position outbox.Position) string {
	payload := []byte(strconv.FormatInt(position.OccurredAt.UnixMicro(), 10) +
		triggerCursorFieldSeparator +
		position.ID.String())

	// The tag first, so that decoding can cut it off without knowing either half's length.
	return base64.RawURLEncoding.EncodeToString(append(c.tag(payload), payload...))
}

// Decode reads a position back, or reports the cursor as invalid.
//
// Every failure is the same answer, as in the stream codec and for the same reason. Tampered,
// truncated, from another installation, or produced before a key rotation are one situation from
// the caller's point of view - this cursor is not usable, start again - and saying which would tell
// whoever is probing how far their forgery got.
func (c TriggerCursorCodec) Decode(cursor string) (outbox.Position, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || len(raw) < cursorTagLength+1 {
		return outbox.Position{}, errTriggerCursorInvalid
	}

	tag, payload := raw[:cursorTagLength], raw[cursorTagLength:]
	// Constant time, because a byte-by-byte comparison of a tag is what lets a forger find one byte
	// at a time.
	if !hmac.Equal(tag, c.tag(payload)) {
		return outbox.Position{}, errTriggerCursorInvalid
	}

	// SplitN rather than Split: the identifier is a uuid and contains no separator today, but the
	// cut belongs to the first field's grammar rather than to the second's, and a payload with a
	// stray separator should decode or be refused on its signature, not on its field count.
	fields := strings.SplitN(string(payload), triggerCursorFieldSeparator, triggerCursorParts)
	if len(fields) != triggerCursorParts {
		return outbox.Position{}, errTriggerCursorInvalid
	}
	micros, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return outbox.Position{}, errTriggerCursorInvalid
	}
	id, err := shared.ParseID(fields[1])
	if err != nil {
		return outbox.Position{}, errTriggerCursorInvalid
	}
	return outbox.Position{OccurredAt: time.UnixMicro(micros).UTC(), ID: id}, nil
}

func (c TriggerCursorCodec) tag(payload []byte) []byte {
	mac := hmac.New(sha256.New, c.key)
	mac.Write(payload)
	return mac.Sum(nil)[:cursorTagLength]
}

// errTriggerCursorInvalid is a client error rather than an internal one: the cursor came from the
// request, in a query parameter the caller chose to send.
var errTriggerCursorInvalid = shared.ErrValidation.
	WithDetail("triggers.cursor_invalid").
	WithFields(shared.FieldError{Path: "/since", Code: "triggers.cursor_invalid"})
