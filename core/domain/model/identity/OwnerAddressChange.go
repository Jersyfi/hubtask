// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A workspace's owner changes its address where the installation allows it, once its cooldown has
// passed since the last change, whoever made it (multi-tenancy.md §3.1, milestone CD Decision 4).
// The operator is never held by either.

const (
	// DefaultAddressCooldownDays is the cooldown where the installation decides none.
	DefaultAddressCooldownDays = 30
	// MaxAddressCooldownDays bounds what an operator may set: a year.
	MaxAddressCooldownDays = 365
)

// OwnerAddressRule is what the installation decided about an owner's change.
type OwnerAddressRule struct {
	// Allowed is the installation's switch, off by default and always off in single mode.
	Allowed bool
	// CooldownDays is the installation's cooldown; zero is the default.
	CooldownDays int
}

// Cooldown is the rule's cooldown in force.
func (r OwnerAddressRule) Cooldown() time.Duration {
	days := r.CooldownDays
	if days <= 0 {
		days = DefaultAddressCooldownDays
	}
	return time.Duration(days) * 24 * time.Hour
}

// ChangeableFrom is the moment from which an owner may change the address again: the last change
// plus the cooldown. Zero where the address never changed or the cooldown has passed by now.
func (r OwnerAddressRule) ChangeableFrom(lastChange, now time.Time) time.Time {
	if lastChange.IsZero() {
		return time.Time{}
	}
	from := lastChange.Add(r.Cooldown())
	if !now.Before(from) {
		return time.Time{}
	}
	return from
}

// RefuseOwnerChange refuses an owner's change the installation does not allow
// (`access.address_change_not_allowed`), or one inside the cooldown
// (`access.address_change_cooldown`, naming `possible_at`).
func (r OwnerAddressRule) RefuseOwnerChange(lastChange, now time.Time) error {
	if !r.Allowed {
		return shared.ErrForbidden.WithDetail("access.address_change_not_allowed")
	}
	if from := r.ChangeableFrom(lastChange, now); !from.IsZero() {
		return shared.ErrConflict.WithDetail("access.address_change_cooldown").
			WithParams(map[string]string{"possible_at": from.UTC().Format(time.RFC3339)})
	}
	return nil
}

// ValidAddressCooldownDays checks the operator's cooldown: zero for none decided, else 1 to 365 days.
func ValidAddressCooldownDays(days int) (int, error) {
	if days == 0 || (days >= 1 && days <= MaxAddressCooldownDays) {
		return days, nil
	}
	params := map[string]string{"value": strconv.Itoa(days), "max": strconv.Itoa(MaxAddressCooldownDays)}
	return 0, shared.ErrValidation.WithDetail("admin.address_cooldown_invalid").WithParams(params).
		WithFields(shared.FieldError{
			Path: "/address_change_cooldown_days", Code: "admin.address_cooldown_invalid", Params: params,
		})
}
