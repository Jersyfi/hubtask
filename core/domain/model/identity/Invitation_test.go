// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

const accountID = shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")

// An invited account is a real account that cannot act. Both halves matter: the permissions can be
// arranged before the person ever signs in, and until they do, nothing they were given works.
func TestAnInvitedAccountExistsAndCannotAct(t *testing.T) {
	account, err := Invite(accountID, tenantID, "Anna@Example.ORG", "Anna", nil, nil)
	if err != nil {
		t.Fatalf("inviting: %v", err)
	}

	if account.Status != AccountInvited {
		t.Errorf("status %q, want INVITED", account.Status)
	}
	if account.Kind != AccountUser {
		t.Errorf("kind %q, want USER", account.Kind)
	}
	if err := account.Verify(); err == nil {
		t.Error("an invited account was allowed to act before accepting")
	}
}

// Stored lower case, because the uniqueness index compares that way - two spellings of one address
// would otherwise be two accounts for one person.
func TestTheAddressIsNormalisedTheWayTheIndexCompares(t *testing.T) {
	account, err := Invite(accountID, tenantID, "  Anna@Example.ORG  ", "", nil, nil)
	if err != nil {
		t.Fatalf("inviting: %v", err)
	}
	if account.Email != "anna@example.org" {
		t.Errorf("email %q, want it lower case and trimmed", account.Email)
	}
}

// The check is structural, not a proof: only a message arriving proves an address. What it catches
// is the mistake before it becomes a row and a send attempt.
func TestAnAddressThatCannotBeOneIsRefused(t *testing.T) {
	cases := []struct {
		name  string
		given string
		code  string
	}{
		{"nothing at all", "", "accounts.email_empty"},
		{"whitespace alone", "   ", "accounts.email_empty"},
		{"a name pasted into the wrong field", "Anna Winkel", "accounts.email_malformed"},
		{"no domain", "anna@", "accounts.email_malformed"},
		{"no local part", "@example.org", "accounts.email_malformed"},
		{"a domain without a dot", "anna@localhost", "accounts.email_malformed"},
		{"a trailing dot", "anna@example.", "accounts.email_malformed"},
		{"a list rather than an address", "anna@example.org, bert@example.org", "accounts.email_malformed"},
		{"a display form", "Anna <anna@example.org>", "accounts.email_malformed"},
		{"longer than any real address", strings.Repeat("a", 250) + "@example.org", "accounts.email_too_long"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Invite(accountID, tenantID, c.given, "Anna", nil, nil)
			if err == nil || shared.AsError(err).DetailCode != c.code {
				t.Fatalf("error %v, want %s", err, c.code)
			}
		})
	}
}

// An invitation that names nobody still has to show something beside every action the account
// takes; the local part is a better answer than an empty cell.
// The display name is stored in normal form C (i18n-l10n.md §5, M-07): it stands beside
// everything a person does, and stands there in one spelling. The three doors an account comes
// in through - an invitation, a service account, an identity provider - all bring it there, and
// without the port a name that is not ASCII is refused rather than stored as it came.
func TestADisplayNameIsStoredInNormalFormC(t *testing.T) {
	invited, err := Invite(accountID, tenantID, "anna@example.org", " Anna Mu\u0308ller ", nil, text.Composing{})
	if err != nil {
		t.Fatalf("inviting: %v", err)
	}
	if invited.DisplayName != "Anna M\u00fcller" {
		t.Errorf("invited as %q, want the composed form", invited.DisplayName)
	}

	machine, err := NewServiceAccount(accountID, tenantID, "Na\u0308chtlicher Import", text.Composing{})
	if err != nil {
		t.Fatalf("a service account: %v", err)
	}
	if machine.DisplayName != "N\u00e4chtlicher Import" {
		t.Errorf("the service account is %q, want the composed form", machine.DisplayName)
	}

	external, err := ProvisionExternal(accountID, tenantID, "", "Jose\u0301", nil, text.Composing{})
	if err != nil {
		t.Fatalf("provisioning from a provider: %v", err)
	}
	if external.DisplayName != "Jos\u00e9" {
		t.Errorf("provisioned as %q, want the composed form", external.DisplayName)
	}

	_, err = Invite(accountID, tenantID, "anna@example.org", "Anna Mu\u0308ller", nil, nil)
	if shared.AsError(err).DetailCode != "text.normalizer_missing" {
		t.Errorf("without a port the decomposed name was accepted: %v", err)
	}
}

func TestADisplayNameFallsBackToTheLocalPart(t *testing.T) {
	account, err := Invite(accountID, tenantID, "j.winkel@example.org", "   ", nil, nil)
	if err != nil {
		t.Fatalf("inviting: %v", err)
	}
	if account.DisplayName != "j.winkel" {
		t.Errorf("display name %q, want the local part", account.DisplayName)
	}
}

func TestPreferencesAreCheckedAndEmptyMeansInherit(t *testing.T) {
	account, err := Invite(accountID, tenantID, "anna@example.org", "Anna", nil, nil)
	if err != nil {
		t.Fatalf("inviting: %v", err)
	}

	cases := []struct {
		name  string
		given Preferences
		code  string
	}{
		{"a language", Preferences{Locale: "de"}, ""},
		{"a language and a region", Preferences{Locale: "de-AT"}, ""},
		{"a script", Preferences{Locale: "zh-Hans"}, ""},
		{"a numeric region", Preferences{Locale: "es-419"}, ""},
		{"a zone that exists", Preferences{TimeZone: "Europe/Berlin"}, ""},
		{"a week start", Preferences{WeekStart: "monday"}, ""},
		{"everything empty means inherit", Preferences{}, ""},
		{"a sentence is not a locale", Preferences{Locale: "German please"}, "accounts.locale_invalid"},
		{"an empty subtag", Preferences{Locale: "de-"}, "accounts.locale_invalid"},
		{"a zone nobody has", Preferences{TimeZone: "Europe/Atlantis"}, "accounts.time_zone_invalid"},
		{"an offset cannot represent daylight saving", Preferences{TimeZone: "+02:00"}, "accounts.time_zone_invalid"},
		{"a day the calendar does not start on", Preferences{WeekStart: "WEDNESDAY"}, "accounts.week_start_invalid"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			updated, err := account.WithPreferences(c.given)

			if c.code == "" {
				if err != nil {
					t.Fatalf("error %v, want the preference accepted", err)
				}
				if c.given.WeekStart != "" && updated.WeekStart != strings.ToUpper(c.given.WeekStart) {
					t.Errorf("week start %q, want it upper case", updated.WeekStart)
				}
				return
			}
			if err == nil || shared.AsError(err).DetailCode != c.code {
				t.Fatalf("error %v, want %s", err, c.code)
			}
		})
	}
}

// The preferences are a copy, like every other change here: an unchecked value never reaches the
// account the caller is holding.
func TestApplyingPreferencesDoesNotMutate(t *testing.T) {
	account, _ := Invite(accountID, tenantID, "anna@example.org", "Anna", nil, nil)

	if _, err := account.WithPreferences(Preferences{TimeZone: "Europe/Atlantis"}); err == nil {
		t.Fatal("an unknown zone was accepted")
	}
	if account.TimeZone != "" {
		t.Errorf("the account changed to %q despite the error", account.TimeZone)
	}
}

// encoder stands in for the idna adapter (M-10): one Unicode domain, and one the DNS refuses.
type encoder struct{}

func (encoder) ToASCII(domain string) (string, error) {
	switch domain {
	case "müller.de":
		return "xn--mller-kva.de", nil
	case "-bad.example":
		return "", errors.New("idna: invalid label")
	}
	return domain, nil
}

// An address with a Unicode domain is stored in the form a mail server sees (i18n-l10n.md §7),
// so that anna@müller.de and anna@xn--mller-kva.de are one row; without an encoder the domain is
// refused rather than stored as typed - fail closed, so that a caller that forgot the port
// cannot create the second row this exists to prevent.
func TestAUnicodeDomainIsStoredAsPunycode(t *testing.T) {
	first, err := Invite(accountID, tenantID, "Anna@Müller.de", "Anna", encoder{}, nil)
	if err != nil {
		t.Fatalf("inviting: %v", err)
	}
	second, err := Invite(accountID, tenantID, "anna@xn--MLLER-kva.de", "Anna", encoder{}, nil)
	if err != nil {
		t.Fatalf("inviting the encoded spelling: %v", err)
	}
	if first.Email != "anna@xn--mller-kva.de" || second.Email != first.Email {
		t.Errorf("the two spellings became %q and %q", first.Email, second.Email)
	}

	for raw, want := range map[string]string{
		"anna@-bad.example": "accounts.email_malformed",
	} {
		_, err := Invite(accountID, tenantID, raw, "Anna", encoder{}, nil)
		var domainErr *shared.Error
		if !errors.As(err, &domainErr) || domainErr.DetailCode != want {
			t.Errorf("%q: %v, want %s", raw, err, want)
		}
	}

	_, err = Invite(accountID, tenantID, "anna@müller.de", "Anna", nil, nil)
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "accounts.email_malformed" {
		t.Errorf("without an encoder the Unicode domain was not refused: %v", err)
	}
	if plain, err := Invite(accountID, tenantID, "anna@example.org", "Anna", nil, nil); err != nil || plain.Email != "anna@example.org" {
		t.Errorf("without an encoder an ASCII address is refused: %v", err)
	}
}

func TestLookupAddressBringsATypedAddressToTheStoredForm(t *testing.T) {
	for raw, want := range map[string]string{
		"Anna@Müller.de":    "anna@xn--mller-kva.de",
		"anna@example.org":  "anna@example.org",
		"not an address":    "not an address",
		"anna@-bad.example": "anna@-bad.example",
	} {
		if got := LookupAddress(raw, encoder{}); got != want {
			t.Errorf("%q → %q, want %q", raw, got, want)
		}
	}
	if got := LookupAddress("anna@müller.de", nil); got != "anna@müller.de" {
		t.Errorf("without an encoder the address changed: %q", got)
	}
}
