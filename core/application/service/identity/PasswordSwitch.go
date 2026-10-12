// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// Whether the password is a way into a workspace at all (UC-ID-12 check 6).
//
// A workspace that switches the password off - "only through our directory" - means it: the sign-in
// card hides the form, and every door the server has refuses a password there as well. A switch that
// were only the screen's would let an account that still holds a password sign in with it through
// the API, hubctl or any client that did not read the card - a second door the workspace had closed,
// for a person its directory had already let go (P-02).
//
// **Nothing is taken from an account.** The stored password stays; switching the password back on
// makes it work again. **Two exceptions.** ADR-0076 §4's fallback: a workspace left with no way in
// that works - whatever the cause (E2) - signs in by password again. And an account no provider can
// ever recognise - a sign-in name, no address (ADR-0074) - keeps its password, whatever the methods
// say (identity.md §4.1): otherwise switching the password off for a household that signs in through
// a provider would lock out its child, while the fallback stays shut because the provider works for
// everybody else (P-16). **The refusal is the same for every address**, asked before any account is
// looked up, so it says nothing about who has one - only what the public sign-in rules already say
// about the workspace. A sign-in name goes on to the lookup, and an account it finds that also holds
// an address is refused once the password has proved right.

// PasswordDoor answers whether the password is open in a workspace, and why where it is open only as
// the fallback - empty otherwise. The password writer is one; the session writer asks it through its
// rule, which is that writer (teachTheRule).
type PasswordDoor interface {
	PasswordOpen(ctx context.Context, tenantID shared.ID) (open bool, fallback FallbackCause, err error)
}

var _ PasswordDoor = (*PasswordWriter)(nil)

// PasswordOpen resolves the workspace's methods: open where the password is among them, and where it
// is not, open only as the fallback - which an operator's opening is too, overriding the workspace's
// switch and any installation lock alike (ADR-0078 §3), because both are read through the methods.
func (w PasswordWriter) PasswordOpen(
	ctx context.Context, tenantID shared.ID,
) (bool, FallbackCause, error) {
	rules, err := w.ResolveFor(ctx, tenantID)
	if err != nil {
		return false, "", err
	}
	methods := rules.Effective.Policy.Methods
	if slices.Contains(methods, domain.MethodDirect) {
		return true, "", nil
	}
	fallback, err := w.WaysIn.PasswordFallback(ctx, tenantID, methods)
	if err != nil {
		return false, "", err
	}
	return fallback.Opens(), fallback, nil
}

// passwordOpenHere asks the rule once whether the password is open here, and whether only as the
// fallback. A door that lets the password through and records the fallback records it from this
// answer - a second read could disagree with the one that opened the door, and costs the same rows
// twice (E2). A writer whose rule cannot answer - built without one, as the tests of other doors
// are - lets the password through.
func (w SessionWriter) passwordOpenHere(
	ctx context.Context, tenantID shared.ID,
) (open bool, fallback FallbackCause, err error) {
	door, ok := w.Rule.(PasswordDoor)
	if !ok {
		return true, "", nil
	}
	return door.PasswordOpen(ctx, tenantID)
}

// passwordShutFor refuses a password the workspace switched off, at a door that knows whose password
// it is asked: open, where the methods leave the password out, for an account only the password
// reaches.
func (w SessionWriter) passwordShutFor(ctx context.Context, tenantID, accountID shared.ID) error {
	open, fallback, err := w.passwordOpenHere(ctx, tenantID)
	if err != nil || open {
		return err
	}
	if w.People == nil {
		return refuseShut(open, fallback, nil)
	}
	var held domain.Account
	err = w.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID, ActorID: accountID},
		func(ctx context.Context) error {
			read, err := w.People.Find(ctx, accountID)
			held = read
			return err
		})
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
		return err
	}
	if err != nil || !held.OnlyThePasswordReaches() {
		return passwordNotOffered()
	}
	return nil
}

// refuseShut turns PasswordOpen's answer into the refusal, or into nothing.
func refuseShut(open bool, _ FallbackCause, err error) error {
	if err != nil {
		return err
	}
	if !open {
		return passwordNotOffered()
	}
	return nil
}

// passwordNotOffered is the one refusal every password door gives in a workspace without one.
func passwordNotOffered() error {
	return shared.ErrForbidden.WithDetail("auth.password_not_offered")
}
