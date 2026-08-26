// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// CalendarFeed is one subscription: a token somebody holds, over one saved view, revocable.
//
// It carries no token - only the hash of one, and not even that here: what is stored is computed
// by the adapter that holds the pepper (security.md §8), the same division the personal access
// token uses. This aggregate knows who owns the feed, what it serves, and whether it still does.
type CalendarFeed struct {
	ID       shared.ID
	TenantID shared.ID
	// AccountID is the owner. The feed reads as this account reads, evaluated at every fetch -
	// which is what makes a revoked membership narrow the calendar instead of being survived by
	// a token that remembers better days.
	AccountID shared.ID
	// ViewID is the saved view the feed serves, and the zero identifier once that view has been
	// deleted. Not a reason to delete the feed: a view is the workspace's and a feed is the
	// subscriber's, so the reference nulls and the feed serves nothing while saying why.
	ViewID    shared.ID
	CreatedAt time.Time
	// RevokedAt is when the token stopped working, and the zero time while it works. Kept rather
	// than deleted, because "that token was revoked on Tuesday" is a question somebody asks after
	// a laptop goes missing.
	RevokedAt time.Time
}

// NewCalendarFeedInput is what minting one needs.
type NewCalendarFeedInput struct {
	ID        shared.ID
	TenantID  shared.ID
	AccountID shared.ID
	ViewID    shared.ID
	Now       time.Time
}

// NewCalendarFeed mints a feed. The token is minted beside it and never stored on it.
func NewCalendarFeed(in NewCalendarFeedInput) (CalendarFeed, error) {
	switch {
	case in.ID.IsZero(), in.TenantID.IsZero():
		return CalendarFeed{}, shared.ErrInternal.WithDetail("calendar.feed_incomplete")
	case in.AccountID.IsZero():
		// A feed with no owner has no authorisation to read with, and this one reads at every
		// fetch. That is a defect in the caller rather than something a client sent.
		return CalendarFeed{}, shared.ErrInternal.WithDetail("calendar.feed_owner_missing")
	case in.ViewID.IsZero():
		return CalendarFeed{}, shared.ErrValidation.
			WithDetail("calendar.view_required").
			WithFields(shared.FieldError{Path: "/view_id", Code: "calendar.view_required"})
	}

	return CalendarFeed{
		ID: in.ID, TenantID: in.TenantID, AccountID: in.AccountID, ViewID: in.ViewID,
		CreatedAt: in.Now,
	}, nil
}

// Revoked stops the token, and says whether anything changed. Revoking twice is not an error -
// the second call is somebody making sure - but it does not move the moment, because the first
// revocation is when the token stopped working.
func (f CalendarFeed) Revoked(at time.Time) (CalendarFeed, bool) {
	if !f.RevokedAt.IsZero() {
		return f, false
	}
	f.RevokedAt = at
	return f, true
}

// IsRevoked reports whether the token still works. The moment is not compared against a clock:
// revocation is immediate, and a row with a revoked_at in it is a row whose token is finished.
func (f CalendarFeed) IsRevoked() bool { return !f.RevokedAt.IsZero() }

// ServesAView reports whether there is still a view behind the feed. A feed whose view was
// deleted answers nothing, and answers it as a 404 rather than as an empty calendar: an empty
// calendar reads as "you have nothing on", which is a different and wrong statement.
func (f CalendarFeed) ServesAView() bool { return !f.ViewID.IsZero() }
