// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"time"
	"unicode"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// SetupCodeLifetime is how long the code a fresh installation prints at start stays valid
// (UC-INS-01). A restart prints a new one.
const SetupCodeLifetime = 30 * time.Minute

// SetupCodeBytes is how much entropy a code is drawn from. Sixty of its bits are used: twelve
// characters of a 32-letter alphabet.
const SetupCodeBytes = 8

const (
	setupCodeLength = 12
	setupCodeGroup  = 4
	// setupAlphabet is Crockford's base 32: no I, L, O or U, so nothing a person reads off a console
	// can be mistaken for something else.
	setupAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

// SetupCode is the one-time key to a fresh installation's setup. Its canonical form is the twelve
// characters without the dashes, which is what is hashed and stored.
type SetupCode struct {
	canonical string
}

// NewSetupCode renders drawn bytes as a code.
func NewSetupCode(material []byte) (SetupCode, error) {
	if len(material) < SetupCodeBytes {
		return SetupCode{}, shared.ErrInternal.WithDetail("setup.code_unmintable")
	}
	var bits uint64
	for _, b := range material[:SetupCodeBytes] {
		bits = bits<<8 | uint64(b)
	}
	var out strings.Builder
	for i := 0; i < setupCodeLength; i++ {
		out.WriteByte(setupAlphabet[bits&31])
		bits >>= 5
	}
	return SetupCode{canonical: out.String()}, nil
}

// ParseSetupCode reads a code as a person types or pastes it: any case, with or without the dashes
// and spaces between its groups, and the letters Crockford's alphabet reads as digits (I and L as
// one, O as zero). Anything else is not a code - the same refusal a wrong code gets.
func ParseSetupCode(input string) (SetupCode, error) {
	var out strings.Builder
	for _, r := range strings.ToUpper(input) {
		switch {
		case r == '-' || unicode.IsSpace(r):
			continue
		case r == 'I' || r == 'L':
			r = '1'
		case r == 'O':
			r = '0'
		}
		if !strings.ContainsRune(setupAlphabet, r) {
			return SetupCode{}, SetupCodeWrong()
		}
		out.WriteRune(r)
	}
	if out.Len() != setupCodeLength {
		return SetupCode{}, SetupCodeWrong()
	}
	return SetupCode{canonical: out.String()}, nil
}

// Canonical is the form that is hashed: twelve characters, no dashes.
func (c SetupCode) Canonical() string { return c.canonical }

// Display is the form printed on the console: three groups of four.
func (c SetupCode) Display() string {
	var out strings.Builder
	for i := 0; i < len(c.canonical); i += setupCodeGroup {
		if i > 0 {
			out.WriteByte('-')
		}
		out.WriteString(c.canonical[i : i+setupCodeGroup])
	}
	return out.String()
}

// SetupCodeWrong is the refusal of a code that matches nothing.
func SetupCodeWrong() error {
	return shared.ErrForbidden.WithDetail("setup.code_wrong").
		WithFields(shared.FieldError{Path: "/code", Code: "setup.code_wrong"})
}

// DeriveTenantSlug turns a workspace's name into a short name for single mode, where the person
// setting it up is never asked for one (it routes nothing there). Lower-case letters and digits,
// runs of anything else as one hyphen; `workspace` where too little is left.
func DeriveTenantSlug(name string) string {
	var out strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			hyphen = false
		case out.Len() > 0 && !hyphen:
			out.WriteByte('-')
			hyphen = true
		}
		if out.Len() >= MaxTenantSlugLength {
			break
		}
	}
	slug := strings.TrimRight(out.String(), "-")
	if len(slug) > MaxTenantSlugLength {
		slug = strings.TrimRight(slug[:MaxTenantSlugLength], "-")
	}
	if !validTenantSlug(slug) {
		return "workspace"
	}
	return slug
}
