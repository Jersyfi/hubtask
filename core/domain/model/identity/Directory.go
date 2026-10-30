// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A directory - Entra ID, Okta, Keycloak, Google - provisions the workspace's people and groups
// over SCIM 2.0 (UC-ID-28, identity.md §26). What it may do is what an administrator may: invite a
// person, switch one off and on, keep groups. What it may not is decided here, because it holds
// for every door a directory could come through.

// MaxExternalID bounds the identifier a directory knows a person or a group by (migration 0256).
const MaxExternalID = 255

// ExternalID checks the identifier a directory sends: trimmed, at most MaxExternalID characters,
// no control character. Empty is no identifier - the directory cleared it.
func ExternalID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if utf8.RuneCountInString(id) > MaxExternalID {
		return "", externalIDRefused("directory.external_id_too_long")
	}
	for _, r := range id {
		if unicode.IsControl(r) {
			return "", externalIDRefused("directory.external_id_malformed")
		}
	}
	return id, nil
}

func externalIDRefused(code string) error {
	return shared.ErrValidation.WithDetail(code).
		WithFields(shared.FieldError{Path: "/externalId", Code: code})
}

// DirectoryActive is what a directory reads as `active`: whether the person is provisioned and
// not switched off. An invited person is active - provisioned, waiting only for their first
// sign-in through the workspace's provider.
func (a Account) DirectoryActive() bool {
	return a.Status != AccountDisabled && a.Status != AccountAnonymized
}

// DirectoryStatus is the status a directory's `active` moves an account to, and whether it moves
// at all.
//
// Switching off takes an active or invited person to DISABLED; switching on brings a disabled one
// back as ACTIVE, a person who had been here. A restricted account (Art. 18) is refused rather than
// switched off: switching it on again would lift a restriction nobody asked to lift
// (data-protection.md §4), and the restriction is the workspace's to end. An anonymised account has
// nobody left to switch.
func DirectoryStatus(current AccountStatus, active bool) (AccountStatus, bool, error) {
	switch current {
	case AccountAnonymized:
		return current, false, shared.ErrNotFound.WithDetail("accounts.not_found")
	case AccountRestricted:
		if active {
			return current, false, nil
		}
		return current, false, shared.ErrConflict.WithDetail("directory.restricted_kept")
	case AccountDisabled:
		if active {
			return AccountActive, true, nil
		}
		return current, false, nil
	case AccountActive, AccountInvited:
		if active {
			return current, false, nil
		}
		return AccountDisabled, true, nil
	}
	return current, false, shared.ErrInternal.WithDetail("accounts.status_unknown")
}

// SameAddress reports whether a directory's `userName` names the address the account signs in
// with. The address is the person's way in, so a directory does not change it (identity.md §26):
// a different one is refused rather than written.
func (a Account) SameAddress(lookup string) bool {
	return a.Email != "" && strings.EqualFold(a.Email, lookup)
}
