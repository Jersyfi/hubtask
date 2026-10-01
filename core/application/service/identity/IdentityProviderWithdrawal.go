// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strconv"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// Ending an installation's offer (ADR-0076 §2-3).
//
// **Announced, by default two weeks ahead.** Until the date the provider keeps working everywhere
// and the workspaces that use it say when it ends; on the date every reader of the offer stops
// reading it as one (offeredHere), so nothing has to run on that day. **Withdraw now** is the same
// write with a date that has come, and it repeats the count the operator just read - the answer to a
// compromised provider, which is why the withdrawal is never blocked by who uses it.
//
// Here rather than in the control plane's package for the reason ConfigureAt is: the proof a change
// to a way in demands is this writer's, and a second implementation of it would be a second rule.

// WithdrawOfferAt announces the end of an installation's offer, or ends it now. A zero `at` is the
// default notice; a moment that has come is now, and needs `confirmCount` to equal the number of
// workspaces that have the provider switched on.
func (w IdentityProviderWriter) WithdrawOfferAt(
	ctx context.Context, scope persistence.Scope, actor appshared.ActorContext,
	id shared.ID, at time.Time, confirmCount *int, stepUpToken string,
) (domain.IdentityProvider, error) {
	if err := w.proveChange(ctx, actor, stepUpToken); err != nil {
		return domain.IdentityProvider{}, err
	}
	var stored domain.IdentityProvider
	err := w.Session.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		now := w.Session.Clock.Now()
		found, err := w.installationRow(ctx, id)
		if err != nil {
			return err
		}
		when := at
		if when.IsZero() {
			when = now.Add(domain.WithdrawalNotice)
		}
		if !now.Before(when) {
			// Withdraw now. The count is read inside this transaction, so a workspace that
			// switched the provider on since the operator looked makes the confirmation wrong
			// rather than unseen.
			if confirmCount == nil || *confirmCount != found.OfferedWorkspaces {
				return withdrawCountMismatch(found.OfferedWorkspaces)
			}
			// The moment it actually ended, not one in the past it never ended at.
			when = now
		}
		written, ok, err := w.Providers.SetWithdrawal(ctx, id, when, now)
		if err != nil {
			return err
		}
		if !ok {
			return shared.ErrNotFound.WithDetail("identity_provider.not_found")
		}
		stored = written
		return nil
	})
	if err != nil {
		return domain.IdentityProvider{}, err
	}
	return stored, nil
}

// CancelWithdrawalAt keeps offering an installation's provider without an end. Before the date
// nothing changes for anybody; after it, the workspaces that had it switched on sign in through it
// again - their switches and the connected identities were never touched (UC-INS-11 check 5).
func (w IdentityProviderWriter) CancelWithdrawalAt(
	ctx context.Context, scope persistence.Scope, actor appshared.ActorContext,
	id shared.ID, stepUpToken string,
) (domain.IdentityProvider, error) {
	if err := w.proveChange(ctx, actor, stepUpToken); err != nil {
		return domain.IdentityProvider{}, err
	}
	var stored domain.IdentityProvider
	err := w.Session.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if _, err := w.installationRow(ctx, id); err != nil {
			return err
		}
		written, ok, err := w.Providers.SetWithdrawal(ctx, id, time.Time{}, w.Session.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			return shared.ErrNotFound.WithDetail("identity_provider.not_found")
		}
		stored = written
		return nil
	})
	if err != nil {
		return domain.IdentityProvider{}, err
	}
	return stored, nil
}

// installationRow reads the row a withdrawal is about. A workspace's own row is not offered by
// anybody and has nothing to withdraw: its switch is in the workspace's list.
func (w IdentityProviderWriter) installationRow(
	ctx context.Context, id shared.ID,
) (domain.IdentityProvider, error) {
	if id.IsZero() {
		return domain.IdentityProvider{}, shared.ErrNotFound.WithDetail("identity_provider.not_found")
	}
	found, err := w.Providers.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.IdentityProvider{}, shared.ErrNotFound.WithDetail("identity_provider.not_found")
		}
		return domain.IdentityProvider{}, err
	}
	if !found.Installation() {
		return domain.IdentityProvider{}, shared.ErrNotFound.WithDetail("identity_provider.not_found")
	}
	return found, nil
}

// withdrawCountMismatch is the refusal of a *Withdraw now* whose confirmation does not repeat the
// count. The number travels: it is the operator's to know, and the sentence asks them to read it.
func withdrawCountMismatch(count int) error {
	return shared.ErrValidation.
		WithDetail("identity_provider.withdraw_count_mismatch").
		WithParams(map[string]string{"count": strconv.Itoa(count)}).
		WithFields(shared.FieldError{
			Path: "/confirm_count", Code: "identity_provider.withdraw_count_mismatch",
		})
}

// withdrawInstead is the refusal of the installation's form when it would switch the offer: an
// offer ends through the withdrawal, announced or confirmed, and is kept by cancelling it.
func withdrawInstead() error {
	return shared.ErrValidation.
		WithDetail("identity_provider.withdraw_instead").
		WithFields(shared.FieldError{Path: "/enabled", Code: "identity_provider.withdraw_instead"})
}
