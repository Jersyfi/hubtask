// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	feedrepo "github.com/Jersyfi/hubtask/core/application/repository/integration"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// ReadCalendarFeed answers what one subscription contains (D-08).
//
// It is not a use case and is deliberately not in the catalogue, for MediaContent's reason: a
// catalogue entry is something a person, an agent or a rule may ask for, and this is a route
// answering a credential nobody in the system holds. What it shares with a use case is the rule
// that matters - the tenant comes from the credential and from nowhere else (multi-tenancy.md
// §2.2) - and the rule that matters more: every permission question is asked here, in the
// application layer, and the adapter only ever parses a string (ADR-0005).
//
// The feed reads as its **owner**, evaluated now rather than when the token was minted. An owner
// who has lost access to half the view sees the feed narrow with them, a disabled account's feed
// stops answering, and a revoked membership is not survived by a token that remembers better days.
type ReadCalendarFeed struct {
	Feeds    feedrepo.CalendarFeeds
	Accounts identityrepo.Accounts
	// Export is the selection the view export performs, without its audit entry: the same rows,
	// the same cap, the same visibility question. Shared rather than reimplemented, so a feed and
	// an export of one view can never disagree about what is in it.
	Export     ExportView
	UnitOfWork persistence.UnitOfWork
}

// Execute answers the feed's rows, or that there is no such feed.
//
// Every refusal is the same one. An unknown token, a revoked feed, a feed whose view was deleted,
// an owner who no longer exists and an owner who may no longer see the view all answer not found,
// with the same body: distinguishing them would answer questions for whoever is trying tokens
// (T-04's shape, and T-21's requirement).
func (h ReadCalendarFeed) Execute(
	ctx context.Context, token domain.FeedToken,
) (ExportedView, error) {
	if token.IsZero() {
		return ExportedView{}, feedGone()
	}
	// The tenant the token names, and the only source of one on a route with no authentication.
	// A token quoting a tenant it does not belong to finds nothing: the hash is unique across the
	// installation and the lookup runs inside this scope (ADR-0010).
	scope := persistence.Scope{TenantID: token.TenantID()}

	var (
		feed  domain.CalendarFeed
		owner appshared.ActorContext
	)
	err := h.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		found, err := h.Feeds.FindByToken(ctx, token)
		if err != nil {
			return err
		}
		if found.IsRevoked() || !found.ServesAView() {
			return feedGone()
		}

		account, err := h.Accounts.Find(ctx, found.AccountID)
		if err != nil {
			return err
		}
		if err := account.Verify(); err != nil {
			// A disabled or still-invited account has no authorisation to read with. The feed
			// stops answering with it, rather than serving what the account could see last year.
			return feedGone()
		}

		feed = found
		owner = appshared.ActorContext{
			Kind: appshared.ActorUser, TenantID: found.TenantID, AccountID: account.ID,
			AccountName: account.DisplayName,
			Locale:      account.Locale, TimeZone: account.TimeZone,
			// Exactly the two reads a fetch performs, and nothing else. A feed is not a token
			// with scopes somebody chose: it is a subscription, and giving the synthetic actor
			// only what the fetch needs is what keeps it from ever being able to do more.
			Scopes: []string{viewsRead, itemsRead},
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return ExportedView{}, feedGone()
		}
		return ExportedView{}, err
	}

	saved, err := h.view(ctx, owner, feed.ViewID)
	if err != nil {
		return ExportedView{}, err
	}

	exported, err := h.Export.rows(ctx, owner, saved)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) || errors.Is(err, shared.ErrForbidden) {
			// The owner may no longer see their own view - shared into a scope they have since
			// been removed from. The subscription goes quiet rather than explaining why.
			return ExportedView{}, feedGone()
		}
		return ExportedView{}, err
	}
	return exported, nil
}

// view reads the saved view the feed serves.
func (h ReadCalendarFeed) view(
	ctx context.Context, owner appshared.ActorContext, viewID shared.ID,
) (view.SavedView, error) {
	var saved view.SavedView
	err := h.UnitOfWork.WithinReadOnly(ctx, owner.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Export.Views.Find(ctx, viewID)
		saved = found
		return err
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return view.SavedView{}, feedGone()
		}
		return view.SavedView{}, err
	}
	return saved, nil
}

// feedGone is the one answer every refusal on this route produces. A 404 with no detail beyond
// the code: a revoked token and one that never existed have to be indistinguishable, or the route
// becomes an oracle for whoever is trying them (security.md §4 T-21).
func feedGone() error {
	return shared.ErrNotFound.WithDetail("calendar.feed_not_found")
}
