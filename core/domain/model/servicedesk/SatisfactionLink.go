// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// SatisfactionLinkLife is how long the mail's links answer: long enough for a requester back from
// a holiday, short enough that an old mail forwarded on rates nothing.
const SatisfactionLinkLife = 30 * 24 * time.Hour

// SatisfactionLink is the satisfaction question mailed to a requester without an account
// (UC-SVC-07 check 6): one per requester and entry, answered once. A resolution mailed again
// replaces it.
type SatisfactionLink struct {
	TenantID    shared.ID
	RequesterID shared.ID
	ItemID      shared.ID
	CreatedAt   time.Time
	ExpiresAt   time.Time
	UsedAt      *time.Time
}

// NewSatisfactionLink opens the question at now.
func NewSatisfactionLink(tenantID, requesterID, itemID shared.ID, now time.Time) (SatisfactionLink, error) {
	if tenantID.IsZero() || requesterID.IsZero() || itemID.IsZero() {
		return SatisfactionLink{}, shared.ErrInternal.WithDetail("satisfaction.identity_incomplete")
	}
	return SatisfactionLink{
		TenantID: tenantID, RequesterID: requesterID, ItemID: itemID,
		CreatedAt: now, ExpiresAt: now.Add(SatisfactionLinkLife),
	}, nil
}

// Use answers the question at now: a link answered once or past its life is refused, each with
// its own code, so the page can say which.
func (l SatisfactionLink) Use(now time.Time) (SatisfactionLink, error) {
	if l.UsedAt != nil {
		return l, shared.ErrConflict.WithDetail("satisfaction.link_used")
	}
	if !now.Before(l.ExpiresAt) {
		return l, shared.ErrGone.WithDetail("satisfaction.link_expired")
	}
	used := now
	l.UsedAt = &used
	return l, nil
}
