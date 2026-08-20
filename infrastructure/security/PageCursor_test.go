// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"encoding/base64"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Internal, like TokenHasher's own test, and for the reason that matters here: the interesting
// refusals are payloads that carry a *valid* signature and are still unusable. Producing one needs
// the key, so a test outside the package could only reach the cases the signature already rejects.
func codec() CursorCodec {
	return NewCursorCodec(secret.New("an installation secret of sufficient length"))
}

const sampleID = "0192f000-0000-7000-8000-00000000000a"

// The round trip is the whole contract: what a page reports as its boundary is what the next query
// compares against, so a codec that lost either half would page from the wrong place.
func TestABoundarySurvivesTheRoundTrip(t *testing.T) {
	cases := map[string]Position{
		"an ordinary rank key":   At("a0", shared.MustParseID(sampleID)),
		"a fractional key":       At("a1V", shared.MustParseID(sampleID)),
		"a negative integer key": At("A00000000000000000000000000", shared.MustParseID(sampleID)),
		// Not a key the domain produces today, and the codec must not be what assumes it never
		// will: the next thing paged on may be a timestamp or a name.
		"a key with punctuation": At("2026-08-18T00:00:00Z", shared.MustParseID(sampleID)),
		"an empty key":           At("", shared.MustParseID(sampleID)),
		// The query language sorts by up to four fields at once, so a boundary is the whole tuple
		// (api-guidelines.md §3). The one shape that has to survive is a key that is itself empty
		// in the middle of others - which is how a sort by a field with no value records itself.
		"several keys": {
			Keys: []string{"vfalse", "", "v2026-08-18T00:00:00Z"}, ID: shared.MustParseID(sampleID),
		},
	}

	for name, position := range cases {
		t.Run(name, func(t *testing.T) {
			decoded, err := codec().Decode(codec().Encode(position))
			if err != nil {
				t.Fatalf("decoding what Encode produced: %v", err)
			}
			if !slices.Equal(decoded.Keys, position.Keys) || decoded.ID != position.ID {
				t.Errorf("round trip gave %+v, want %+v", decoded, position)
			}
		})
	}
}

// "There is no next page" is the absence of a cursor. A cursor that decoded to nothing would make
// every client's "is there more" check a two-step one.
func TestTheEmptyPositionHasNoCursor(t *testing.T) {
	if cursor := codec().Encode(Position{Keys: []string{"a0"}}); cursor != "" {
		t.Errorf("a position with no identifier produced the cursor %q", cursor)
	}
}

// The point of signing: a client cannot move the boundary. Every one of these is a forgery, a
// corruption, or a value from somewhere else, and all of them have to come back as one refusal -
// saying which would tell whoever is probing how far their attempt got.
func TestAnUnusableCursorIsRefused(t *testing.T) {
	valid := codec().Encode(At("a0", shared.MustParseID(sampleID)))

	cases := map[string]string{
		"empty":                 "",
		"not base64":            "not a cursor!!",
		"shorter than the tag":  base64.RawURLEncoding.EncodeToString([]byte("short")),
		"payload with no tag":   base64.RawURLEncoding.EncodeToString([]byte("a0\x00" + sampleID)),
		"a flipped payload bit": flipByte(t, valid, -1),
		"a flipped tag bit":     flipByte(t, valid, 0),
		"truncated":             valid[:len(valid)-4],
		// Signed by this very codec, so the signature passes and the parser is what has to refuse.
		"no separator":      codec().signed("a0" + sampleID),
		"an unparseable id": codec().signed("a0\x00not-a-uuid"),
		"no id at all":      codec().signed("a0\x00"),
	}

	for name, cursor := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := codec().Decode(cursor)
			if err == nil {
				t.Fatal("accepted")
			}
			if !errors.Is(err, shared.ErrValidation) {
				t.Errorf("refused with %v, want a validation error", err)
			}
			if got := shared.AsError(err).DetailCode; got != "shared.cursor_invalid" {
				t.Errorf("detail code %q, want shared.cursor_invalid", got)
			}
		})
	}
}

// A cursor is bound to the installation that issued it, so a value from staging, or from before a
// key rotation, is not honoured by whatever receives it.
func TestACursorFromAnotherSecretIsRefused(t *testing.T) {
	issued := codec().Encode(At("a0", shared.MustParseID(sampleID)))

	elsewhere := NewCursorCodec(secret.New("a different installation secret entirely"))
	if _, err := elsewhere.Decode(issued); err == nil {
		t.Error("a cursor signed with another key was accepted")
	}
}

// The installation secret must not be derivable from a cursor, and must not be reusable across
// purposes: the same secret also peppers token hashes, and a tag from one is not a value of the
// other (the reason cursorInfo and pepperInfo exist).
func TestTheCursorSharesNoDerivedKeyWithTheTokenHasher(t *testing.T) {
	installation := secret.New("an installation secret of sufficient length")

	cursor := NewCursorCodec(installation)
	hasher := NewTokenHasher(installation)

	payload := "a0\x00" + sampleID
	if string(cursor.tag([]byte(payload))) == string(hasher.Hash(payload)[:cursorTagLength]) {
		t.Error("the cursor tag and the token hash are the same construction over the same input")
	}
}

// The cursor travels in a query string and has to survive one unescaped. The assertion is here so
// that a switch to StdEncoding fails loudly rather than in a client's URL handling.
func TestACursorIsSafeInAURL(t *testing.T) {
	cursor := codec().Encode(At("2026-08-18T00:00:00Z", shared.MustParseID(sampleID)))

	if strings.ContainsAny(cursor, "+/=&?#% ") {
		t.Errorf("the cursor %q carries a character a URL would escape", cursor)
	}
}

// signed is Encode without its structure: an arbitrary payload with a valid tag, which is how the
// parsing failures are reached at all.
func (c CursorCodec) signed(payload string) string {
	return base64.RawURLEncoding.EncodeToString(append(c.tag([]byte(payload)), payload...))
}

// flipByte corrupts one byte of a cursor. A negative index counts from the end, so the caller can
// name the tag (0) or the payload (-1) without knowing either length.
func flipByte(t *testing.T, cursor string, index int) string {
	t.Helper()

	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatalf("the codec produced something undecodable: %v", err)
	}
	if index < 0 {
		index += len(raw)
	}
	raw[index] ^= 0x01
	return base64.RawURLEncoding.EncodeToString(raw)
}
