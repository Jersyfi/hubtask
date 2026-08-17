// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package clock

import (
	cryptorand "crypto/rand"
	"encoding/hex"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/clock"
)

// UUIDv7 mints identifiers in the application rather than in the database (arc42 §8.13).
//
// Time-ordered, so an index stays dense and rows written together sit together on disk; and known
// before the insert, which is what lets one transaction write a row, an event that references it
// and an audit entry about it without a round trip in between.
//
// Version 7 rather than 4 for the first reason and rather than a sequence for the second: a
// sequence is only unique inside one database, and identifiers travel between installations
// through exports, backups and offline clients that mint their own (offline-sync.md §9).
type UUIDv7 struct {
	// Clock is the time source. A port rather than time.Now, so a test can assert on the ordering
	// of the identifiers it gets.
	Clock port.Clock
}

func NewUUIDv7(clock port.Clock) UUIDv7 { return UUIDv7{Clock: clock} }

var _ port.IDGenerator = UUIDv7{}

// NewID returns a UUIDv7 in the canonical lower-case form (RFC 9562 §5.7): 48 bits of
// milliseconds since the epoch, the version, 12 random bits, the variant, and 62 more random bits.
//
// The randomness comes from crypto/rand rather than math/rand. An identifier is a capability in
// several places - a media reference, a feed token's subject, an idempotency key's neighbour - and
// a predictable one is a guessable one (security.md §8).
func (g UUIDv7) NewID() shared.ID {
	var value [16]byte

	copy(value[:6], timestamp(g.Clock.Now().UnixMilli()))

	// crypto/rand.Read is documented never to fail since Go 1.24; it panics inside the standard
	// library if the operating system's source is broken, which is a machine that cannot serve
	// requests anyway.
	_, _ = cryptorand.Read(value[6:])

	// The four version bits and the two variant bits, over the random ones.
	value[6] = (value[6] & 0x0f) | 0x70
	value[8] = (value[8] & 0x3f) | 0x80

	return shared.ID(format(value))
}

// timestamp is the low 48 bits of the millisecond count, most significant byte first. Keeping
// only six of the eight bytes is the format (RFC 9562 §5.7) rather than an oversight: 48 bits of
// milliseconds run to the year 10889.
//
//nolint:gosec // G115: every shift is masked to eight bits by the conversion, which is the point
func timestamp(milliseconds int64) []byte {
	return []byte{
		byte(milliseconds >> 40), byte(milliseconds >> 32), byte(milliseconds >> 24),
		byte(milliseconds >> 16), byte(milliseconds >> 8), byte(milliseconds),
	}
}

// format writes the canonical 8-4-4-4-12 form. Written out rather than assembled with fmt: this
// runs on every write, several times, and the shape is fixed.
func format(value [16]byte) string {
	var out [36]byte
	hex.Encode(out[0:8], value[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], value[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], value[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], value[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], value[10:16])
	return string(out[:])
}
