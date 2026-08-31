// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// maxEmail is the practical bound. The standard allows 254 octets; anything at that length is a
// machine's address, and the column is text either way - the check exists so that a paste of a
// whole message body fails as a validation error rather than as a database error.
const maxEmail = 254

// maxDisplayName bounds what appears beside every action in the audit trail and the activity feed.
const maxDisplayName = 200

// Invite produces the account an invitation creates.
//
// An invited account is a real account in `INVITED` status, not a row in a waiting room: the
// permissions can be granted before the person ever signs in, which is what an administrator
// setting a workspace up actually does. What it cannot do is act - Account.Verify refuses any
// status but ACTIVE, so an invitation that is never accepted grants nothing.
//
// There is deliberately no token here. Accepting an invitation means proving control of the
// mailbox and choosing a credential, and neither exists before the sign-in flow arrives in 0.6.0
// (security.md §5). Issuing a token nobody can redeem would be a credential lying around for
// months.
func Invite(id shared.ID, tenantID shared.ID, email string, displayName string) (Account, error) {
	if id.IsZero() || tenantID.IsZero() {
		return Account{}, shared.ErrInternal.WithDetail("accounts.identity_incomplete")
	}

	address, err := emailAddress(email)
	if err != nil {
		return Account{}, err
	}
	name, err := accountDisplayName(displayName, address)
	if err != nil {
		return Account{}, err
	}

	return Account{
		ID:          id,
		TenantID:    tenantID,
		Kind:        AccountUser,
		Email:       address,
		DisplayName: name,
		Status:      AccountInvited,
	}, nil
}

// NewServiceAccount produces an account that exists only to be acted through: an integration, a
// script, a rule that has to keep running after the person who wrote it has left.
//
// Three things are deliberately different from an invitation. There is no address, because there
// is nobody to write to and no mailbox to prove control of. There is no INVITED step, for the
// same reason - nothing is waiting to be accepted, so the account is active from the moment it
// exists. And the display name is required rather than derived: an invitation can fall back on
// the local part of an address, and this has none, so a service account with no name would appear
// nameless beside every action it takes in the audit trail.
func NewServiceAccount(id shared.ID, tenantID shared.ID, displayName string) (Account, error) {
	if id.IsZero() || tenantID.IsZero() {
		return Account{}, shared.ErrInternal.WithDetail("accounts.identity_incomplete")
	}

	name, err := accountDisplayName(displayName, "")
	if err != nil {
		return Account{}, err
	}

	return Account{
		ID:          id,
		TenantID:    tenantID,
		Kind:        AccountServiceAccount,
		DisplayName: name,
		Status:      AccountActive,
	}, nil
}

// emailAddress normalises and checks an address.
//
// The check is deliberately structural rather than a full RFC 5322 parse: the only proof that an
// address is real is a message arriving at it, which is what the invitation is for. What this
// catches is the mistake - a name pasted into the wrong field, a missing domain - before it
// becomes a row and a send attempt.
//
// Lower-cased, because the uniqueness index compares that way (db/schema.sql, account_email_uq)
// and because two spellings of one address are two accounts for one person.
func emailAddress(raw string) (string, error) {
	address := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case address == "":
		return "", shared.ErrValidation.WithDetail("accounts.email_empty")
	case utf8.RuneCountInString(address) > maxEmail:
		return "", shared.ErrValidation.
			WithDetail("accounts.email_too_long").
			WithParams(map[string]string{"maximum": itoa(maxEmail)})
	}

	local, domain, found := strings.Cut(address, "@")
	if !found || local == "" || domain == "" ||
		strings.ContainsAny(address, " \t\n\r,;<>\"") ||
		!strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", shared.ErrValidation.WithDetail("accounts.email_malformed")
	}
	return address, nil
}

// accountDisplayName falls back to the local part of the address. An invitation that names nobody
// still has to show something beside every action the account takes, and "j.winkel" is a better
// answer than an empty cell (audit.md §2).
func accountDisplayName(raw string, address string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		local, _, _ := strings.Cut(address, "@")
		name = local
	}
	switch {
	case name == "":
		return "", shared.ErrValidation.WithDetail("accounts.display_name_empty")
	case utf8.RuneCountInString(name) > maxDisplayName:
		return "", shared.ErrValidation.
			WithDetail("accounts.display_name_too_long").
			WithParams(map[string]string{"maximum": itoa(maxDisplayName)})
	case strings.ContainsAny(name, "\n\r"):
		return "", shared.ErrValidation.WithDetail("accounts.display_name_malformed")
	}
	return name, nil
}

// Preferences is what an account may set about how the product speaks to it: the second link of
// the resolution chain request -> account -> tenant -> installation (i18n-l10n.md §2).
//
// Every field is optional in the sense that empty means "inherit". That is not the same as unset:
// clearing a preference is a legitimate action, and it means the tenant's default applies again.
type Preferences struct {
	Locale   string
	TimeZone string
	// WeekStart is a day name in English, because it is a code and not display text (rule 8):
	// MONDAY, SUNDAY, SATURDAY - what CLDR distinguishes for the calendars this product draws.
	WeekStart string
}

// weekStarts is the closed set. Three rather than seven: those are the ones a calendar in real use
// starts on, and an unbounded value here would reach a date library that has to guess.
var weekStarts = [...]string{"MONDAY", "SUNDAY", "SATURDAY"}

// WithPreferences returns the account with its preferences applied, each checked.
//
// A value receiver returning a copy, like every other change in this package: a caller that
// ignores the result has changed nothing.
func (a Account) WithPreferences(p Preferences) (Account, error) {
	locale, err := localeTag(p.Locale)
	if err != nil {
		return Account{}, err
	}
	zone, err := timeZone(p.TimeZone)
	if err != nil {
		return Account{}, err
	}
	weekStart, err := weekStart(p.WeekStart)
	if err != nil {
		return Account{}, err
	}

	a.Locale, a.TimeZone, a.WeekStart = locale, zone, weekStart
	return a, nil
}

// localeTag checks the shape of a BCP 47 tag: de, de-AT, pt-BR, zh-Hans. Structural on purpose -
// the catalogue decides which locales exist, and a tag this product has no translation for still
// resolves to its fallback rather than failing a request (i18n-l10n.md §2).
//
// The grammar itself is shared.LanguageTag, because an entry's content language is checked against
// exactly the same one (C-08). What is not shared is the message code: this is an account's
// preference and says so.
func localeTag(raw string) (string, error) {
	tag, ok := shared.LanguageTag(raw)
	if !ok {
		return "", shared.ErrValidation.
			WithDetail("accounts.locale_invalid").
			WithParams(map[string]string{"value": strings.TrimSpace(raw)})
	}
	return tag, nil
}

// timeZone checks an IANA name by loading it, which is the only check worth making: a name the
// time package cannot load is a name no calculation can use. Never a fixed offset - an offset
// cannot represent daylight saving, and this product schedules across it.
func timeZone(raw string) (string, error) {
	zone := strings.TrimSpace(raw)
	if zone == "" {
		return "", nil
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return "", shared.ErrValidation.
			WithDetail("accounts.time_zone_invalid").
			WithParams(map[string]string{"value": zone})
	}
	return zone, nil
}

func weekStart(raw string) (string, error) {
	day := strings.ToUpper(strings.TrimSpace(raw))
	if day == "" {
		return "", nil
	}
	for _, known := range weekStarts {
		if day == known {
			return day, nil
		}
	}
	return "", shared.ErrValidation.
		WithDetail("accounts.week_start_invalid").
		WithParams(map[string]string{"value": day})
}
