// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// RestrictedPerson is somebody whose processing is restricted (Art. 18, data-protection.md §4): an
// account in the RESTRICTED state, or an anonymised one a legal hold still keeps contributions of
// (§4.1). The names and the address are what a text might carry of them; an anonymised account
// has none left.
type RestrictedPerson struct {
	AccountID   shared.ID
	DisplayName string
	SignInName  string
	Email       string
}

// Names answers every spelling of the person a text might carry.
func (p RestrictedPerson) Names() []string {
	return []string{p.DisplayName, p.SignInName, p.Email}
}

// Restricted is the set of people nothing automatic may act on, as one read answered it: no rule
// runs as or about them, no rule puts them on an entry, and no model is shown their content.
// The zero value restricts nobody.
type Restricted struct {
	accounts  map[shared.ID]bool
	addresses map[string]bool
}

// NewRestricted builds the set.
func NewRestricted(people []RestrictedPerson) Restricted {
	set := Restricted{accounts: map[shared.ID]bool{}, addresses: map[string]bool{}}
	for _, person := range people {
		if person.AccountID.IsZero() {
			continue
		}
		set.accounts[person.AccountID] = true
		if address := strings.ToLower(strings.TrimSpace(person.Email)); address != "" {
			set.addresses[address] = true
		}
	}
	return set
}

// Has reports whether an account's processing is restricted. The zero account - the system, a
// row nobody is named on - never is.
func (r Restricted) Has(accountID shared.ID) bool {
	return !accountID.IsZero() && r.accounts[accountID]
}

// HasAddress reports whether a sender's address is a restricted person's: what they mailed in is
// theirs.
func (r Restricted) HasAddress(address string) bool {
	address = strings.ToLower(strings.TrimSpace(address))
	return address != "" && r.addresses[address]
}

// Empty reports that nobody is restricted.
func (r Restricted) Empty() bool { return len(r.accounts) == 0 }
