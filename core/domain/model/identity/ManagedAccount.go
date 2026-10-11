// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// A managed account is a person without a mail address who signs in with a sign-in name and a
// password of their own, after a start password Hubtask drew (ADR-0074, identity.md §13).

// The bounds of a sign-in name, counted after folding.
const (
	MinSignInName = 3
	MaxSignInName = 32
)

// StartPasswordBytes is how much entropy a start password is drawn from: sixteen characters of
// five bits each, eighty bits - enough against a guesser the attempt ledger already slows to a
// crawl, and short enough to type back from a printed slip.
const StartPasswordBytes = 16

// startPasswordAlphabet is Crockford's thirty-two: the digits and the lower-case letters without
// `i`, `l`, `o` and `u`. Without those letters `0` and `1` read one way, which is the point of a
// password a child types from paper. Thirty-two divides 256, so a byte modulo the length is
// uniform.
const startPasswordAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// FoldSignInName brings a typed name to the form a stored one has - compatibility form, lower case,
// trimmed - without judging it: for a lookup, where a name that could never be stored simply matches
// no row. Lower-cased as an address is (emailAddress), so "Lena" and "lena" are one name.
func FoldSignInName(raw string, form text.Normalizer) string {
	folded := strings.TrimSpace(raw)
	if form != nil {
		folded = form.NFKC(folded)
	}
	return strings.TrimSpace(strings.ToLower(folded))
}

// SignInName folds a name and checks it can be one (ADR-0074 §1): 3 to 32 characters, no `@` - so
// a name and an address are told apart by their shape alone - and no whitespace or control
// character, so the name on a printed slip reads one way. Letters of any script are names.
//
// Without a normaliser a name that is not ASCII is refused rather than stored unfolded: two
// spellings of one name would be two accounts for one person.
func SignInName(raw string, form text.Normalizer) (string, error) {
	if form == nil && !isASCII(raw) {
		return "", shared.ErrInternal.WithDetail("text.normalizer_missing")
	}
	name := FoldSignInName(raw, form)
	switch length := utf8.RuneCountInString(name); {
	case length < MinSignInName:
		return "", signInNameRefused("accounts.sign_in_name_too_short", "minimum", MinSignInName)
	case length > MaxSignInName:
		return "", signInNameRefused("accounts.sign_in_name_too_long", "maximum", MaxSignInName)
	}
	for _, r := range name {
		if r == '@' || unicode.In(r, unicode.Z, unicode.C) || unicode.IsSpace(r) {
			return "", signInNameRefused("accounts.sign_in_name_malformed", "", 0)
		}
	}
	return name, nil
}

func signInNameRefused(code, bound string, value int) error {
	refusal := shared.ErrValidation.WithDetail(code).
		WithFields(shared.FieldError{Path: "/sign_in_name", Code: code})
	if bound != "" {
		refusal = refusal.WithParams(map[string]string{bound: itoa(value)})
	}
	return refusal
}

// IsSignInName reports whether a typed identifier names a sign-in name rather than an address: a
// sign-in name never holds `@` (SignInName refuses one), and an address always does.
func IsSignInName(identifier string) bool { return !strings.Contains(identifier, "@") }

// OnlyThePasswordReaches reports whether the password is this account's one way in: a sign-in name
// and no address, so no identity provider can ever recognise it. Where a workspace's methods leave
// the password out, its password doors stay open for such an account and for nobody else
// (identity.md §4.1) - an account that also holds an address has a provider's way.
func (a Account) OnlyThePasswordReaches() bool { return a.SignInName != "" && a.Email == "" }

// NewManagedAccount produces the account a managed account is: a person, active from the moment it
// exists - there is no mailbox an invitation could prove, so nothing waits to be accepted - with a
// sign-in name instead of an address. The display name falls back to the name, an invitation's
// reasoning: something has to stand beside every action the account takes.
func NewManagedAccount(
	id, tenantID shared.ID, signInName, displayName string, form text.Normalizer,
) (Account, error) {
	if id.IsZero() || tenantID.IsZero() {
		return Account{}, shared.ErrInternal.WithDetail("accounts.identity_incomplete")
	}
	name, err := SignInName(signInName, form)
	if err != nil {
		return Account{}, err
	}
	shown, err := accountDisplayName(displayName, name, form)
	if err != nil {
		return Account{}, err
	}
	return Account{
		ID:          id,
		TenantID:    tenantID,
		Kind:        AccountUser,
		SignInName:  name,
		DisplayName: shown,
		Status:      AccountActive,
	}, nil
}

// StartPassword spells drawn entropy as a start password: four groups of four characters joined by
// a dash. The caller draws the bytes through its entropy port (rule 4); fewer than
// StartPasswordBytes is refused rather than spelt into a weaker password.
func StartPassword(drawn []byte) (string, error) {
	if len(drawn) < StartPasswordBytes {
		return "", shared.ErrInternal.WithDetail("accounts.start_password_unmintable")
	}
	var spelt strings.Builder
	for index, b := range drawn[:StartPasswordBytes] {
		if index > 0 && index%4 == 0 {
			spelt.WriteByte('-')
		}
		spelt.WriteByte(startPasswordAlphabet[int(b)%len(startPasswordAlphabet)])
	}
	return spelt.String(), nil
}
