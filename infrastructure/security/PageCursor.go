// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// cursorInfo separates this use of the installation secret from every other one, exactly as
// pepperInfo does for token hashes: a value produced for one purpose can then never be replayed as
// a value of another.
const cursorInfo = "hubtask/page-cursor/v1"

// cursorTagLength is how much of the HMAC travels in the cursor.
//
// 128 bits, not 256. A tag is what stops a client forging a boundary, and the thing being protected
// is a sort key plus an identifier the client has already been shown - so the margin only has to
// make forgery infeasible, and doubling it would double the length of a value that appears in every
// paged URL. 128 bits is the length the standard truncation guidance settles on (RFC 2104 §5).
const cursorTagLength = 16

// cursorSeparator ends the sort key inside the payload.
//
// NUL, because it is the one byte neither part can contain: a rank key is base 62 (Ordering.go) and
// an identifier is a UUID in its canonical spelling. The split looks for the *last* one all the
// same - the identifier is the half whose alphabet is certain, so anchoring on it is what keeps a
// sort key that somehow carried the separator from being read as an identifier.
const cursorSeparator = 0x00

// Position is a page boundary: the sort key of the last row of a page, and that row's identifier.
//
// The identifier is not decoration. A sort key alone is only a boundary if it is unique, and a
// fractional index is unique in practice rather than by constraint; the pair is what
// api-guidelines.md §4 means by "the sort key plus id", and what lets the comparison be a strict
// `>` on a total order.
type Position struct {
	SortKey string
	ID      shared.ID
}

// CursorCodec turns a page boundary into an opaque cursor and back.
//
// Opaque and signed, per api-guidelines.md §4. Signing is not about confidentiality - both halves
// are in the page the client just read - it is about the boundary being one this server produced.
// An unsigned cursor is a query parameter in disguise: a client, or an agent generating one, would
// eventually craft a sort key by hand, and the endpoint's contract would quietly have become "seek
// to an arbitrary position" with the index and the collation assumptions that go with it.
//
// It lives here rather than in the application layer because it is a cryptographic primitive keyed
// on the installation secret, and the secret has no business above the adapters (security.md §8) -
// the same reasoning that keeps the token pepper in TokenHasher. The core passes the cursor through
// as an opaque string and never looks inside it.
type CursorCodec struct {
	key []byte
}

func NewCursorCodec(installationSecret secret.Secret) CursorCodec {
	mac := hmac.New(sha256.New, []byte(installationSecret.Reveal()))
	mac.Write([]byte(cursorInfo))
	return CursorCodec{key: mac.Sum(nil)}
}

// Encode returns the cursor for a boundary. The empty position is the empty cursor: "there is no
// next page" is the absence of a cursor rather than a cursor meaning nothing.
func (c CursorCodec) Encode(position Position) string {
	if position.ID.IsZero() {
		return ""
	}

	payload := make([]byte, 0, len(position.SortKey)+1+len(position.ID))
	payload = append(payload, position.SortKey...)
	payload = append(payload, cursorSeparator)
	payload = append(payload, position.ID...)

	// The tag first, so that decoding can cut it off without knowing either half's length.
	return base64.RawURLEncoding.EncodeToString(append(c.tag(payload), payload...))
}

// Decode reads a boundary back, or reports the cursor as invalid.
//
// Every failure is the same answer - a validation error naming the field - and deliberately so.
// Tampered, truncated, from another installation, or produced before a key rotation are one
// situation from the client's point of view: this cursor is not usable, start the walk again. Saying
// which would tell whoever is probing whether their forgery got as far as the signature.
func (c CursorCodec) Decode(cursor string) (Position, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || len(raw) < cursorTagLength+1 {
		return Position{}, errCursorInvalid
	}

	tag, payload := raw[:cursorTagLength], raw[cursorTagLength:]
	// Constant time, because a byte-by-byte comparison of a tag is what lets a forger find one
	// byte at a time.
	if !hmac.Equal(tag, c.tag(payload)) {
		return Position{}, errCursorInvalid
	}

	end := bytes.LastIndexByte(payload, cursorSeparator)
	if end < 0 {
		return Position{}, errCursorInvalid
	}
	id, err := shared.ParseID(string(payload[end+1:]))
	if err != nil {
		return Position{}, errCursorInvalid
	}
	return Position{SortKey: string(payload[:end]), ID: id}, nil
}

func (c CursorCodec) tag(payload []byte) []byte {
	mac := hmac.New(sha256.New, c.key)
	mac.Write(payload)
	return mac.Sum(nil)[:cursorTagLength]
}

// errCursorInvalid is a client error rather than an internal one: the cursor came from the request.
var errCursorInvalid = shared.ErrValidation.
	WithDetail("shared.cursor_invalid").
	WithFields(shared.FieldError{Path: "/cursor", Code: "shared.cursor_invalid"})
