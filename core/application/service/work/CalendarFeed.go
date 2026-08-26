// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"time"

	feedrepo "github.com/Jersyfi/hubtask/core/application/repository/integration"
	viewrepo "github.com/Jersyfi/hubtask/core/application/repository/view"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	CreateCalendarFeedName = "CreateCalendarFeed"
	ListCalendarFeedsName  = "ListCalendarFeeds"
	RevokeCalendarFeedName = "RevokeCalendarFeed"

	// The audit codes. A feed is a credential, so both its minting and its revocation are
	// auditable acts in their own right (audit.md §2).
	FeedCreatedAction audit.Action = "calendar.feed_created"
	FeedRevokedAction audit.Action = "calendar.feed_revoked"
	// FeedReadAction is the act the public route performs: somebody fetched a calendar. Declared
	// here so that the fetch and the two management acts share one target type.
	FeedReadAction audit.Action = "calendar.feed_read"

	feedTarget = "calendar_feed"
)

// CalendarFeedWriter is what the three feed use cases share.
//
// It lives beside the saved views rather than in a package of its own, because minting a feed is
// a read of a view: the rule that decides who may see which view is viewVisibleTo, in this
// package, and a second copy of a visibility rule is how two answers to one security question get
// into a codebase.
type CalendarFeedWriter struct {
	Feeds      feedrepo.CalendarFeeds
	Views      viewrepo.SavedViews
	Containers repository.Containers
	// Permits answers the view's visibility without writing a denial: a view the caller cannot
	// see is not found, and a DENIED entry per invisible bookmark makes the trail unreadable
	// (T-04, the reasoning GetSavedView already applies).
	Permits    Permitting
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// Entropy is where the token's secret half comes from. A port, so that production draws from
	// crypto/rand and a test can fix the credential it asserts on (rule 4).
	Entropy clock.Entropy
}

// CreateCalendarFeed mints a feed over one saved view and answers its token, once (D-08).
type CreateCalendarFeed struct{ Writer CalendarFeedWriter }

// MintedFeed is what a minting answers: the feed as it will be listed, and the credential that
// will never be readable again.
type MintedFeed struct {
	Feed  domain.CalendarFeed
	Token domain.FeedToken
}

// Execute mints the feed.
//
// The permission is the view's own read. A feed grants exactly what its owner may already read
// and re-asks that question at every fetch, so minting one is not an escalation and needs no
// permission beyond seeing the view - which is also why a view the caller cannot see answers
// "not found" here rather than "forbidden" (T-04).
func (h CreateCalendarFeed) Execute(
	ctx context.Context, actor appshared.ActorContext, viewID shared.ID,
) (MintedFeed, error) {
	w := h.Writer
	if err := actor.RequireScope(viewsRead); err != nil {
		return MintedFeed{}, err
	}
	if viewID.IsZero() {
		return MintedFeed{}, shared.ErrValidation.
			WithDetail("calendar.view_required").
			WithFields(shared.FieldError{Path: "/view_id", Code: "calendar.view_required"})
	}
	if actor.AccountID.IsZero() {
		// A feed reads as an account. A service account or an automation has none, and a feed
		// minted by one would have no authorisation to fetch with.
		return MintedFeed{}, shared.ErrForbidden.WithDetail("calendar.owner_required")
	}

	var saved view.SavedView
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Views.Find(ctx, viewID)
		saved = found
		return err
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return MintedFeed{}, viewNotFound(viewID)
		}
		return MintedFeed{}, err
	}

	visible, err := viewVisibleTo(ctx, w.UnitOfWork, w.Containers, w.Permits, actor, saved)
	if err != nil {
		return MintedFeed{}, err
	}
	if !visible {
		return MintedFeed{}, viewNotFound(viewID)
	}

	entropy, err := w.Entropy.Bytes(domain.FeedTokenSecretBytes)
	if err != nil {
		return MintedFeed{}, shared.ErrInternal.
			WithDetail("calendar.token_unmintable").
			WithCause(err)
	}
	token, err := domain.NewFeedToken(actor.TenantID, entropy)
	if err != nil {
		return MintedFeed{}, err
	}

	var minted domain.CalendarFeed
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		feed, err := domain.NewCalendarFeed(domain.NewCalendarFeedInput{
			ID: w.IDs.NewID(), TenantID: actor.TenantID, AccountID: actor.AccountID,
			ViewID: saved.ID, Now: now,
		})
		if err != nil {
			return err
		}
		if err := w.Feeds.Insert(ctx, feed, token); err != nil {
			return err
		}
		if err := recordFeedAudit(ctx, w.Audit, actor, feed, FeedCreatedAction, now); err != nil {
			return err
		}
		minted = feed
		return nil
	})
	if err != nil {
		return MintedFeed{}, err
	}
	return MintedFeed{Feed: minted, Token: token}, nil
}

// ListCalendarFeeds answers the caller's own feeds.
type ListCalendarFeeds struct{ Writer CalendarFeedWriter }

// Execute reads them. Never anybody else's, whatever the role: a feed is a credential its owner
// holds, and an administrator who could list one could subscribe to it.
func (h ListCalendarFeeds) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.CalendarFeed, error) {
	w := h.Writer
	if err := actor.RequireScope(viewsRead); err != nil {
		return nil, err
	}
	if actor.AccountID.IsZero() {
		return nil, shared.ErrForbidden.WithDetail("calendar.owner_required")
	}

	var feeds []domain.CalendarFeed
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Feeds.ListForAccount(ctx, actor.AccountID)
		feeds = found
		return err
	})
	if err != nil {
		return nil, err
	}
	return feeds, nil
}

// RevokeCalendarFeed stops a token.
type RevokeCalendarFeed struct{ Writer CalendarFeedWriter }

// Execute revokes the feed, and is idempotent: revoking twice is somebody making sure.
//
// The permission is the same read the minting asked for, and deliberately no more. Revocation
// must never be harder than minting - the moment somebody needs it is the moment a URL has gone
// somewhere it should not have, and a person who cannot stop their own leak because their role
// changed is a security problem rather than a policy.
func (h RevokeCalendarFeed) Execute(
	ctx context.Context, actor appshared.ActorContext, feedID shared.ID,
) error {
	w := h.Writer
	if err := actor.RequireScope(viewsRead); err != nil {
		return err
	}
	if feedID.IsZero() {
		return shared.ErrValidation.
			WithDetail("calendar.feed_id_required").
			WithFields(shared.FieldError{Path: "/feed_id", Code: "calendar.feed_id_required"})
	}

	return w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		feed, err := w.Feeds.Find(ctx, feedID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return feedNotFound(feedID)
			}
			return err
		}
		if feed.AccountID != actor.AccountID {
			// Somebody else's feed is not found rather than forbidden, for the reason every other
			// read of somebody else's thing is (T-04).
			return feedNotFound(feedID)
		}

		now := w.Clock.Now()
		changed, err := w.Feeds.Revoke(ctx, feed.ID, now)
		if err != nil {
			return err
		}
		if !changed {
			// Already revoked. No second audit entry: nothing happened, and an entry saying it
			// did would be a false one.
			return nil
		}
		revoked, _ := feed.Revoked(now)
		return recordFeedAudit(ctx, w.Audit, actor, revoked, FeedRevokedAction, now)
	})
}

// feedNotFound is the one answer an absent feed and somebody else's produce.
func feedNotFound(id shared.ID) error {
	return shared.ErrNotFound.
		WithDetail("calendar.feed_not_found").
		WithParams(map[string]string{"feed_id": id.String()})
}

// recordFeedAudit writes the evidence. Every field on it is an identifier: a feed carries no free
// text at all, and the one value that would matter - the token - is never written anywhere, here
// least of all (rule 10, T-21).
func recordFeedAudit(
	ctx context.Context, sink audit.Sink, actor appshared.ActorContext,
	feed domain.CalendarFeed, action audit.Action, at time.Time,
) error {
	changes := []audit.Change{
		{Field: "view_id", Classification: audit.Open, To: idOrEmptyText(feed.ViewID)},
	}
	if action == FeedRevokedAction {
		changes = append(changes,
			audit.Change{Field: "revoked_at", Classification: audit.Open,
				To: feed.RevokedAt.UTC().Format(time.RFC3339)})
	}

	return sink.Append(ctx, audit.Entry{
		TenantID:   feed.TenantID,
		OccurredAt: at,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: feedTarget,
		TargetID:   feed.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	})
}

// feedOutput is the projection every channel gets. The token is not in it: it exists in the
// answer to the minting and nowhere else, which is what "shown once" means.
func feedOutput(feed domain.CalendarFeed) usecase.Output {
	out := usecase.Output{
		"id":         feed.ID.String(),
		"account_id": feed.AccountID.String(),
		"created_at": feed.CreatedAt.UTC().Format(time.RFC3339),
		"view_id":    nil,
		"revoked_at": nil,
	}
	if feed.ServesAView() {
		out["view_id"] = feed.ViewID.String()
	}
	if feed.IsRevoked() {
		out["revoked_at"] = feed.RevokedAt.UTC().Format(time.RFC3339)
	}
	return out
}

// Descriptor is the catalogue entry.
func (h CreateCalendarFeed) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateCalendarFeedName,
		Summary: "Mints a calendar feed over one saved view and answers its token and URL - " +
			"once. The token is stored only as a hash keyed on the installation secret under " +
			"its own purpose label; nothing can turn that back into the token, so a feed whose " +
			"URL was lost is revoked and made again. The feed reads as its owner reads, " +
			"evaluated at every fetch, so an owner who loses access to half the view sees the " +
			"feed narrow with them.",
		SideEffects: "Writes the feed and an audit entry, and answers a credential. Announces " +
			"nothing: a subscription is one person's, and no event in the catalogue is about one.",
		TokenScope: viewsRead,
		Input: []usecase.Field{
			{
				Name: "view_id", Kind: usecase.KindID, Required: true,
				Description: "The saved view to serve. One the caller may read - a view they " +
					"cannot see answers not found, exactly as reading it would.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: FeedCreatedAction, TargetType: feedTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A feed is a credential over a view rather than a change to an entry, and " +
				"the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateCalendarFeed) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	viewID, err := in.ID("view_id")
	if err != nil {
		return nil, err
	}

	minted, err := h.Execute(ctx, actor, viewID)
	if err != nil {
		return nil, err
	}

	// The one place the credential is ever answered. Every channel gets it here and no channel
	// can get it again - the projection every other call uses does not carry it.
	out := feedOutput(minted.Feed)
	out["token"] = minted.Token.Secret()
	return out, nil
}

// Descriptor is the catalogue entry.
func (h ListCalendarFeeds) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListCalendarFeedsName,
		Summary: "The caller's own calendar feeds, newest first - never anybody else's, " +
			"whatever the role. The token is not among the fields: it is shown once, when the " +
			"feed is minted, and stored only as its hash.",
		SideEffects: "None. Reads only.",
		TokenScope:  viewsRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: FeedReadAction, TargetType: feedTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListCalendarFeeds) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	feeds, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(feeds))
	for _, feed := range feeds {
		rows = append(rows, feedOutput(feed))
	}
	return usecase.Output{"data": rows}, nil
}

// Descriptor is the catalogue entry.
func (h RevokeCalendarFeed) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RevokeCalendarFeedName,
		Summary: "Revokes a feed. Every fetch from that moment on answers 404 in exactly the " +
			"words an unknown token produces. The row stays, with the moment it was revoked - " +
			"which is what makes \"that token was revoked on Tuesday\" answerable. Revoking " +
			"twice is not an error; somebody else's feed is not found rather than forbidden.",
		SideEffects: "Stamps the feed and writes an audit entry. A repeat writes neither.",
		TokenScope:  viewsRead,
		Input: []usecase.Field{
			{
				Name: "feed_id", Kind: usecase.KindID, Required: true,
				Description: "The feed to revoke, from the caller's own list.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: FeedRevokedAction, TargetType: feedTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The same reason minting one is exempt: a credential is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RevokeCalendarFeed) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	feedID, err := in.ID("feed_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, feedID); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
