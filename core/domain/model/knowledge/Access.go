// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The bounds of who a page names.
const (
	// MaxRestrictedTo bounds the people one restriction names: a restriction is a small circle,
	// the hub's memberships are what reach many.
	MaxRestrictedTo = 100
	// MaxSharesPerPage bounds the people a page is shared with outside its hub.
	MaxSharesPerPage = 100
)

// Readers is who of the people reaching a page's hub may read it. A page is restricted when it or
// a page above it names the people who may read it (products.md, M3: a page may be restricted to
// fewer, never widened); a page under several restricted ones is read only by the people every one
// of them names.
type Readers struct {
	Restricted bool
	// Accounts are the people every restriction on the way up names, when Restricted.
	Accounts []shared.ID
}

// Admits reports whether an account may read the page beyond what its hub allows. An unrestricted
// page admits everybody the hub admits.
func (r Readers) Admits(account shared.ID) bool {
	if !r.Restricted {
		return true
	}
	for _, id := range r.Accounts {
		if id == account {
			return true
		}
	}
	return false
}

// CombineRestrictions answers the readers of a page from the restrictions on its way up, one set
// per restricted page: the people in every set. No set is an unrestricted page; an empty set
// admits nobody - a restriction whose people have all gone keeps the page closed rather than
// opening it to the hub.
func CombineRestrictions(sets [][]shared.ID) Readers {
	if len(sets) == 0 {
		return Readers{}
	}
	in := map[shared.ID]int{}
	for _, set := range sets {
		seen := map[shared.ID]bool{}
		for _, id := range set {
			if !seen[id] {
				seen[id] = true
				in[id]++
			}
		}
	}
	out := []shared.ID{}
	for _, id := range sets[0] {
		if in[id] == len(sets) {
			out = append(out, id)
			in[id] = 0
		}
	}
	return Readers{Restricted: true, Accounts: out}
}

// CheckRestriction judges the people a page is restricted to: at least the one restricting, so
// that nobody locks themselves out by mistake, without repeats and within the bound.
func CheckRestriction(accounts []shared.ID, by shared.ID) ([]shared.ID, error) {
	out := make([]shared.ID, 0, len(accounts)+1)
	seen := map[shared.ID]bool{}
	for _, id := range accounts {
		if id.IsZero() || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if !seen[by] {
		return nil, invalid("/account_ids", "pages.restriction_excludes_you")
	}
	if len(out) > MaxRestrictedTo {
		return nil, invalid("/account_ids", "pages.restriction_too_large").
			WithParams(map[string]string{"maximum": "100"})
	}
	return out, nil
}

// Share is a page shared with one person, read-only, beside what its hub's memberships give
// (UC-DOC-08). A share opens the page alone, not the pages under it, and never passes a
// restriction: a restricted page is shared only with somebody it already admits.
type Share struct {
	TenantID  shared.ID
	PageID    shared.ID
	AccountID shared.ID
	GrantedBy shared.ID
	GrantedAt time.Time
}

// NewShare checks a share.
func NewShare(tenant, page, account, by shared.ID, readers Readers, now time.Time) (Share, error) {
	if tenant.IsZero() || page.IsZero() || by.IsZero() {
		return Share{}, shared.ErrInternal.WithDetail("pages.identity_incomplete")
	}
	if account.IsZero() {
		return Share{}, invalid("/account_id", "pages.share_account_missing")
	}
	if !readers.Admits(account) {
		return Share{}, invalid("/account_id", "pages.share_restricted")
	}
	return Share{TenantID: tenant, PageID: page, AccountID: account, GrantedBy: by, GrantedAt: now}, nil
}

// DailyNoteDay reads the day a daily note is for: a calendar date, the reader's today when empty.
// Only a day near today is taken, so that a typing mistake does not open a note a century away.
func DailyNoteDay(raw string, now time.Time, zone *time.Location) (time.Time, error) {
	if zone == nil {
		zone = time.UTC
	}
	local := now.In(zone)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	if raw == "" {
		return today, nil
	}
	day, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, invalid("/day", "pages.day_malformed")
	}
	if day.Before(today.AddDate(-10, 0, 0)) || day.After(today.AddDate(1, 0, 0)) {
		return time.Time{}, invalid("/day", "pages.day_out_of_range")
	}
	return day, nil
}

// DailyNoteTitle is a daily note's title: its date, ISO 8601, which every language reads and the
// client shows in the reader's own format.
func DailyNoteTitle(day time.Time) string { return day.Format(time.DateOnly) }
