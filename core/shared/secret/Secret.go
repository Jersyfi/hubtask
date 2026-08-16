// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package secret provides a type that cannot accidentally be written to a log.
//
// Background: threat T-18 (secrets in logs). A `%+v` on a configuration struct is the most
// common way credentials end up in log files. A dedicated type with a masking String() makes
// that mistake impossible instead of merely forbidding it.
package secret

import "fmt"

// Secret wraps a confidential value. The plaintext is reachable only through Reveal.
type Secret struct {
	value string
}

func New(v string) Secret { return Secret{value: v} }

// Reveal returns the plaintext. Calls are deliberately conspicuous in code review.
func (s Secret) Reveal() string { return s.value }

func (s Secret) IsEmpty() bool { return s.value == "" }

// String, GoString and MarshalText all mask - which covers fmt, slog, encoding/json and
// most other serialisers.
func (s Secret) String() string   { return s.masked() }
func (s Secret) GoString() string { return s.masked() }

func (s Secret) MarshalText() ([]byte, error) { return []byte(s.masked()), nil }
func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + s.masked() + `"`), nil }

func (s Secret) masked() string {
	if s.value == "" {
		return "<empty>"
	}
	return "<redacted>"
}

var (
	_ fmt.Stringer   = Secret{}
	_ fmt.GoStringer = Secret{}
)
