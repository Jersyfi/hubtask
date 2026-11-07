// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// CalendarPush writes a person's calendar feed into their own calendar at a provider - Microsoft
// 365 through Graph, Google through its Calendar API - instead of the provider fetching the feed
// (UC-INT-16). It reads as the feed does, as its owner evaluated now; what it writes is what the
// feed serves, a title, a moment and a link back.
type CalendarPush struct {
	ID        shared.ID
	TenantID  shared.ID
	AccountID shared.ID
	FeedID    shared.ID
	Provider  PushProvider
	// Address is the provider account the owner signed in with, as its ID token named it.
	Address string
	State   PushState
	// Failure is the message code of the last push that did not go through; empty while it does.
	Failure   string
	PushedAt  time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

// PushProvider is the calendar a push writes into.
type PushProvider string

const (
	PushMicrosoft PushProvider = "MICROSOFT"
	PushGoogle    PushProvider = "GOOGLE"
)

// OAuthKey is the provider's key among the installation's OAuth apps, shared with mailboxes.
func (p PushProvider) OAuthKey() string { return strings.ToLower(string(p)) }

// ParsePushProvider reads a provider; anything else is refused.
func ParsePushProvider(value string) (PushProvider, error) {
	switch provider := PushProvider(strings.ToUpper(strings.TrimSpace(value))); provider {
	case PushMicrosoft, PushGoogle:
		return provider, nil
	}
	return "", shared.ErrValidation.WithDetail("calendar_push.provider_unknown").
		WithFields(shared.FieldError{Path: "/provider", Code: "calendar_push.provider_unknown"})
}

// PushScopes are what a push asks the provider for: its calendar's events, an offline grant and
// who signed in - never mail, never files.
func (p PushProvider) PushScopes() []string {
	if p == PushGoogle {
		return []string{"https://www.googleapis.com/auth/calendar.events", "openid", "email"}
	}
	return []string{"https://graph.microsoft.com/Calendars.ReadWrite", "offline_access", "openid", "email"}
}

// PushState is whether a push goes through.
type PushState string

const (
	PushActive PushState = "ACTIVE"
	// PushFailed is a push the provider refused for good - the grant revoked, the feed gone: it
	// stops until its owner connects again.
	PushFailed PushState = "FAILED"
)

// MaxCalendarPushes bounds one person's pushes; MaxPushedEvents what one push writes per run, the
// feed's own cap.
const (
	MaxCalendarPushes = 10
	MaxPushedEvents   = 1000
	// PushInterval is how often a push compares the feed with what it wrote.
	PushInterval = 15 * time.Minute
)

// NewCalendarPush connects a feed to a provider.
func NewCalendarPush(id, tenant, account shared.ID, feed CalendarFeed, provider PushProvider, address string, now time.Time) (CalendarPush, error) {
	if feed.AccountID != account || feed.IsRevoked() || !feed.ServesAView() || !feed.Opens(CalendarKind) {
		return CalendarPush{}, shared.ErrNotFound.WithDetail("calendar_push.feed_unknown")
	}
	address = strings.TrimSpace(address)
	if len(address) > 320 {
		address = address[:320]
	}
	return CalendarPush{
		ID: id, TenantID: tenant, AccountID: account, FeedID: feed.ID, Provider: provider, Address: address,
		State: PushActive, CreatedAt: now, UpdatedAt: now, Version: 1,
	}, nil
}

// Pushed records a run: through, or refused with a code - for good when permanent.
func (p *CalendarPush) Pushed(now time.Time, failure string, permanent bool) {
	p.Failure, p.UpdatedAt = failure, now
	if failure == "" {
		p.PushedAt = now
		return
	}
	if permanent {
		p.State = PushFailed
	}
}

// PushedEvent is what a push wrote for one entry: the provider's identifier and a digest of what
// it wrote, so a run that finds the entry unchanged writes nothing.
type PushedEvent struct {
	EntryID  shared.ID
	RemoteID string
	Digest   string
}

// PushEvent is one calendar entry a push writes: the feed's own shape (data-protection.md §9).
type PushEvent struct {
	EntryID shared.ID
	Summary string
	// Start is the instant, or for an all-day entry that day's wall clock in UTC.
	Start  time.Time
	AllDay bool
	URL    string
}

// Digest is what tells a changed entry from an unchanged one.
func (e PushEvent) Digest() string {
	all := "0"
	if e.AllDay {
		all = "1"
	}
	sum := sha256.Sum256([]byte(e.Summary + "\x00" + e.Start.UTC().Format(time.RFC3339) + "\x00" + all + "\x00" + e.URL))
	return hex.EncodeToString(sum[:16])
}

// PushPlan is what one run does: the events to write anew or again, and those to take away.
type PushPlan struct {
	Create []PushEvent
	Update []PushUpdate
	Delete []PushedEvent
}

// PushUpdate is an event written before whose entry changed.
type PushUpdate struct {
	Event    PushEvent
	RemoteID string
}

// PlanPush compares what the feed serves now with what was written: a new entry is created, a
// changed one updated, one the feed no longer serves removed, an unchanged one left alone.
func PlanPush(events []PushEvent, written []PushedEvent) PushPlan {
	known := make(map[shared.ID]PushedEvent, len(written))
	for _, w := range written {
		known[w.EntryID] = w
	}
	var plan PushPlan
	seen := make(map[shared.ID]bool, len(events))
	for _, event := range events {
		if seen[event.EntryID] || len(seen) >= MaxPushedEvents {
			continue
		}
		seen[event.EntryID] = true
		before, ok := known[event.EntryID]
		switch {
		case !ok:
			plan.Create = append(plan.Create, event)
		case before.Digest != event.Digest():
			plan.Update = append(plan.Update, PushUpdate{Event: event, RemoteID: before.RemoteID})
		}
	}
	for _, w := range written {
		if !seen[w.EntryID] {
			plan.Delete = append(plan.Delete, w)
		}
	}
	return plan
}
