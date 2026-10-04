// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"slices"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Whether the password is a way into a workspace at all (SC-24, #1119, UC-ID-12 check 6).
//
// A workspace that switches the password off - "only through our directory" - means it: the sign-in
// card hides the form, and every door the server has refuses a password there as well. Before this,
// the switch was the screen's only, and an account that still held a password signed in with it
// through the API, hubctl or any client that did not read the card - a second door the workspace had
// closed, for a person its directory had already let go (P-02).
//
// **Nothing is taken from an account.** The stored password stays; switching the password back on
// makes it work again. **The one exception** is ADR-0076 §4's fallback: a workspace left with no way in
// that works - whatever the cause (E2, #1138) - signs in by password again. **The refusal is the same
// for every address**, asked before any account is looked up, so it says nothing about who has one -
// only what the public sign-in rules already say about the workspace.

// PasswordDoor answers whether the password is open in a workspace. The password writer is one; the
// session writer asks it through its rule, which is that writer (teachTheRule).
type PasswordDoor interface {
	PasswordOpen(ctx context.Context, tenantID shared.ID) (open, fallback bool, err error)
}

var _ PasswordDoor = (*PasswordWriter)(nil)

// PasswordOpen resolves the workspace's methods: open where the password is among them, and where it
// is not, open only as the fallback.
func (w PasswordWriter) PasswordOpen(ctx context.Context, tenantID shared.ID) (bool, bool, error) {
	rules, err := w.ResolveFor(ctx, tenantID)
	if err != nil {
		return false, false, err
	}
	methods := rules.Effective.Policy.Methods
	if slices.Contains(methods, domain.MethodDirect) {
		return true, false, nil
	}
	fallback, err := w.WaysIn.PasswordFallback(ctx, tenantID, methods)
	if err != nil {
		return false, false, err
	}
	return fallback, fallback, nil
}

// passwordDoor asks the rule once whether the password is open here: the refusal where it is not,
// and whether it is open only as the fallback. A door that lets the password through and records the
// fallback records it from this answer - a second read could disagree with the one that opened the
// door, and costs the same rows twice (E2, #1138). A writer whose rule cannot answer - built without
// one, as the tests of other doors are - lets the password through, which is the shape before SC-24.
func (w SessionWriter) passwordDoor(ctx context.Context, tenantID shared.ID) (fallback bool, err error) {
	door, ok := w.Rule.(PasswordDoor)
	if !ok {
		return false, nil
	}
	open, fallback, err := door.PasswordOpen(ctx, tenantID)
	if err := refuseShut(open, fallback, err); err != nil {
		return false, err
	}
	return fallback, nil
}

// passwordShut is passwordDoor for a door that records nothing of the fallback.
func (w SessionWriter) passwordShut(ctx context.Context, tenantID shared.ID) error {
	_, err := w.passwordDoor(ctx, tenantID)
	return err
}

// refuseShut turns PasswordOpen's answer into the refusal, or into nothing.
func refuseShut(open, _ bool, err error) error {
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
