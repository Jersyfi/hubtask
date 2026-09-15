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

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// streamCursorInfo separates this use of the installation secret from every other one, exactly as
// cursorInfo and pepperInfo do: a value produced for one purpose can then never be replayed as a
// value of another. A page cursor and a stream cursor are both "a position", and without the
// separation one would be accepted where the other belongs.
const streamCursorInfo = "hubtask/stream-cursor/v1"

// streamCursorParts is how many fields a delta cursor's payload has: the position and the moment
// it was minted. A walk cursor carries two more: the kind being walked and the key it resumes after.
const (
	streamCursorParts = 2
	walkCursorParts   = 4
)

// streamCursorFieldSeparator ends the position inside the payload. A full stop, because both fields
// are decimal digits and neither can contain one.
const streamCursorFieldSeparator = "."

// StreamPosition is where a client stands in the change log, and when it was told so.
//
// The moment is the point of the type. ADR-0021 refuses timestamps *as* cursors - the position is
// the sequence number and nothing else orders the walk - but "is this cursor older than the
// tombstone period" is a question about time, and the only honest answer needs the moment the
// client was last handed one (offline-sync.md §7). Carrying it inside the signed cursor means the
// client cannot lie about it and the server does not have to remember.
type StreamPosition struct {
	Seq      int64
	IssuedAt time.Time
	// Kind and After are set while an initial synchronisation is under way (N-02): the kind being
	// walked and the key of the last row handed out. Both empty is a delta position - the only
	// kind a stream resumes from. They are inside the signed payload for the same reason the
	// moment is: a client that could edit them could skip a kind and believe itself complete.
	Kind  string
	After string
}

// StreamCursorCodec turns a position in the change log into an opaque cursor and back.
//
// Opaque and signed for the reason the page cursor is: an unsigned cursor is a query parameter in
// disguise, and a client that could craft one would be seeking to an arbitrary position in another
// walk's index. It is doubly so here, because this one travels as `Last-Event-ID` - a header a
// browser resends by itself, which makes it exactly the value nobody is watching.
//
// It lives here rather than in the application layer because it is a cryptographic primitive keyed
// on the installation secret, and the secret has no business above the adapters (security.md §8).
// The application holds it behind an interface of its own and never looks inside the string.
type StreamCursorCodec struct {
	key []byte
}

func NewStreamCursorCodec(installationSecret secret.Secret) StreamCursorCodec {
	mac := hmac.New(sha256.New, []byte(installationSecret.Reveal()))
	mac.Write([]byte(streamCursorInfo))
	return StreamCursorCodec{key: mac.Sum(nil)}
}

// Encode returns the cursor for a position.
//
// Seconds rather than nanoseconds: the question it answers is measured in days, and a shorter value
// is a shorter header on every one of a stream's events.
func (c StreamCursorCodec) Encode(position StreamPosition) string {
	text := strconv.FormatInt(position.Seq, 10) +
		streamCursorFieldSeparator +
		strconv.FormatInt(position.IssuedAt.Unix(), 10)
	if position.Kind != "" {
		text += streamCursorFieldSeparator + position.Kind + streamCursorFieldSeparator + position.After
	}
	payload := []byte(text)

	// The tag first, so that decoding can cut it off without knowing either half's length.
	return base64.RawURLEncoding.EncodeToString(append(c.tag(payload), payload...))
}

// Decode reads a position back, or reports the cursor as invalid.
//
// Every failure is the same answer, deliberately. Tampered, truncated, from another installation,
// or produced before a key rotation are one situation from the client's point of view - this cursor
// is not usable, resynchronise - and saying which would tell whoever is probing whether their
// forgery got as far as the signature.
func (c StreamCursorCodec) Decode(cursor string) (StreamPosition, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || len(raw) < cursorTagLength+1 {
		return StreamPosition{}, errStreamCursorInvalid
	}

	tag, payload := raw[:cursorTagLength], raw[cursorTagLength:]
	// Constant time, because a byte-by-byte comparison of a tag is what lets a forger find one
	// byte at a time.
	if !hmac.Equal(tag, c.tag(payload)) {
		return StreamPosition{}, errStreamCursorInvalid
	}

	fields := strings.Split(string(payload), streamCursorFieldSeparator)
	if len(fields) != streamCursorParts && len(fields) != walkCursorParts {
		return StreamPosition{}, errStreamCursorInvalid
	}
	seq, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil || seq < 0 {
		return StreamPosition{}, errStreamCursorInvalid
	}
	issued, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return StreamPosition{}, errStreamCursorInvalid
	}
	position := StreamPosition{Seq: seq, IssuedAt: time.Unix(issued, 0).UTC()}
	if len(fields) == walkCursorParts {
		if fields[2] == "" {
			return StreamPosition{}, errStreamCursorInvalid
		}
		position.Kind, position.After = fields[2], fields[3]
	}
	return position, nil
}

func (c StreamCursorCodec) tag(payload []byte) []byte {
	mac := hmac.New(sha256.New, c.key)
	mac.Write(payload)
	return mac.Sum(nil)[:cursorTagLength]
}

// errStreamCursorInvalid is a client error rather than an internal one: the cursor came from the
// request, in a header the client chose to send.
var errStreamCursorInvalid = shared.ErrValidation.
	WithDetail("sync.cursor_invalid").
	WithFields(shared.FieldError{Path: "/Last-Event-ID", Code: "sync.cursor_invalid"})
