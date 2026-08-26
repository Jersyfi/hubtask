// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package secret

import (
	"crypto/subtle"
	"fmt"
)

// Bytes wraps confidential binary material - a key, a derived key, a nonce that must not be
// reused - the way Secret wraps a confidential string.
//
// A separate type rather than Secret over a string, because key material is not text: converting
// it to a string to store it costs a copy that no longer masks, invites a comparison with `==`,
// and makes a length check read as a character count. The two exist side by side for the same
// reason threat T-18 exists at all - a `%+v` over a struct is how secrets reach log files, and a
// type that cannot print itself makes that mistake impossible rather than merely forbidden.
type Bytes struct {
	value []byte
}

// NewBytes takes ownership of the slice. The caller must not keep or reuse it: two references to
// one key are two places it can be overwritten from, and the copy this would otherwise make would
// leave the original lying about in the caller's frame either way.
func NewBytes(v []byte) Bytes { return Bytes{value: v} }

// Reveal returns the material. Calls are deliberately conspicuous in code review, and the slice
// is the wrapper's own - a caller that writes into it changes the key.
func (b Bytes) Reveal() []byte { return b.value }

func (b Bytes) IsEmpty() bool { return len(b.value) == 0 }

// Len is the length of the material. Safe to log, and the one thing about a key that is: "the key
// is 16 bytes where 32 were expected" is what an operator needs in order to fix a configuration.
func (b Bytes) Len() int { return len(b.value) }

// Equal compares in constant time. The obvious `bytes.Equal` returns as soon as two bytes differ,
// which over repeated calls tells an attacker how much of a guess was right (security.md §8).
func (b Bytes) Equal(other Bytes) bool {
	return subtle.ConstantTimeCompare(b.value, other.value) == 1
}

// String, GoString, MarshalText and MarshalJSON all mask - which covers fmt, slog, encoding/json
// and most other serialisers.
func (b Bytes) String() string   { return b.masked() }
func (b Bytes) GoString() string { return b.masked() }

func (b Bytes) MarshalText() ([]byte, error) { return []byte(b.masked()), nil }
func (b Bytes) MarshalJSON() ([]byte, error) { return []byte(`"` + b.masked() + `"`), nil }

func (b Bytes) masked() string {
	if len(b.value) == 0 {
		return "<empty>"
	}
	return "<redacted>"
}

var (
	_ fmt.Stringer   = Bytes{}
	_ fmt.GoStringer = Bytes{}
)
