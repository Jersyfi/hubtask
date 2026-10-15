// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"

	privacyrepo "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	privacy "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A restriction of processing (GDPR Art. 18) as the automatic removals honour it
// (data-retention.md §4 item 2, UC-PRV-04 check 8).
//
// The same read automation and the AI exclusion use, so that the three cannot disagree about a
// person. It reaches what an account hold reaches (§4 item 1): the entries a person created,
// commented on or attached a file to. Only the automatic paths ask - a person emptying a trash or
// purging an entry acts on their own decision, and a restriction is not a lock.

// restrictionsOf reads whose processing is restricted, once per pass. Nothing wired is nobody: a
// build without the read removes as it did before, which is what a workspace without a restriction
// looks like anyway.
func restrictionsOf(ctx context.Context, port privacyrepo.Restrictions) (privacy.Restricted, error) {
	if port == nil {
		return privacy.Restricted{}, nil
	}
	people, err := port.RestrictedPeople(ctx)
	if err != nil {
		return privacy.Restricted{}, err
	}
	return privacy.NewRestricted(people), nil
}

// restrictedAmong reports whether any of an object's contributors is restricted: one is enough,
// as one held account is for an account hold.
func restrictedAmong(restricted privacy.Restricted, contributors []shared.ID) bool {
	for _, account := range contributors {
		if restricted.Has(account) {
			return true
		}
	}
	return false
}
