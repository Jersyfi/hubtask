// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import "strings"

// ID identifies anything the system stores. The values are UUIDv7, generated in the application
// through the IDGenerator port rather than by the database (db/schema.sql conventions, arc42
// §8.13): time-ordered, so an index stays dense, and known before the insert, so an event can
// reference the row it describes.
//
// A distinct type rather than a string, because a tenant ID and an item ID are both 36
// characters and nothing but the type stops one being passed where the other belongs.
type ID string

// ParseID accepts the canonical form 8-4-4-4-12 with lower-case hex digits. Upper case is
// rejected rather than folded: two spellings of one identifier turn into two cache keys, two
// index entries and one hard afternoon.
func ParseID(raw string) (ID, error) {
	if !looksLikeUUID(raw) {
		return "", ErrValidation.
			WithDetail("shared.id_malformed").
			WithParams(map[string]string{"value": raw})
	}
	return ID(raw), nil
}

// MustParseID is for constants and test fixtures, where a malformed literal is a programming
// error rather than input.
func MustParseID(raw string) ID {
	id, err := ParseID(raw)
	if err != nil {
		panic(err)
	}
	return id
}

func (id ID) String() string { return string(id) }

// IsZero reports the absent identifier. Used for optional references - an actor is absent when
// the system itself acts.
func (id ID) IsZero() bool { return id == "" }

const uuidLength = 36

func looksLikeUUID(raw string) bool {
	if len(raw) != uuidLength {
		return false
	}
	for i, r := range raw {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			isDigit := r >= '0' && r <= '9'
			isLowerHex := r >= 'a' && r <= 'f'
			if !isDigit && !isLowerHex {
				return false
			}
		}
	}
	return true
}

// IsUUIDv7 reports whether the version nibble says 7. Not enforced by ParseID: identifiers from
// an import or an older installation may carry another version, and rejecting them at the door
// would make a migration impossible.
func (id ID) IsUUIDv7() bool {
	return len(id) == uuidLength && strings.HasPrefix(string(id[14:]), "7")
}
