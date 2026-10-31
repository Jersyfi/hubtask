// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package servicedesk is the persistence port of the service desk's building blocks: business
// calendars, SLA policies and each entry's SLA record (migration 0240).
package servicedesk

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Calendars stores the workspace's business calendars.
type Calendars interface {
	// Insert writes a new calendar; a key the workspace holds is `calendars.key_taken`.
	Insert(ctx context.Context, cal servicedesk.Calendar) error
	// Update writes the calendar where its version is still expectedVersion.
	Update(ctx context.Context, cal servicedesk.Calendar, expectedVersion int) error
	// Find answers the calendar of a key; shared.ErrNotFound when there is none.
	Find(ctx context.Context, key string) (servicedesk.Calendar, error)
	List(ctx context.Context) ([]servicedesk.Calendar, error)
	// Delete removes the calendar; one a policy names is `calendars.in_use`.
	Delete(ctx context.Context, key string) error
}

// HubCalendars stores the hubs' own versions of the workspace's calendars (migration 0270): one
// per hub and key, under a key the workspace holds.
type HubCalendars interface {
	// Insert writes a hub's calendar; one the hub holds under the key is `calendars.key_taken`.
	Insert(ctx context.Context, cal servicedesk.Calendar) error
	Update(ctx context.Context, cal servicedesk.Calendar, expectedVersion int) error
	// Find answers the hub's calendar of a key; shared.ErrNotFound when it keeps none.
	Find(ctx context.Context, hubID shared.ID, key string) (servicedesk.Calendar, error)
	// OfHub answers the hub's calendars by key.
	OfHub(ctx context.Context, hubID shared.ID) ([]servicedesk.Calendar, error)
	Delete(ctx context.Context, hubID shared.ID, key string) error
}

// Policies stores the workspace's SLA policies.
type Policies interface {
	// Insert writes a new policy; a key the workspace holds is `sla_policies.key_taken`. The
	// calendar's key is the use case's to check: the installation's calendars are no rows.
	Insert(ctx context.Context, policy servicedesk.SLAPolicy) error
	Update(ctx context.Context, policy servicedesk.SLAPolicy, expectedVersion int) error
	Find(ctx context.Context, key string) (servicedesk.SLAPolicy, error)
	// List answers every policy by position and key.
	List(ctx context.Context) ([]servicedesk.SLAPolicy, error)
	Delete(ctx context.Context, key string) error
}

// Records stores what became of each goal on an entry.
type Records interface {
	// Save writes the goal's record whole, one per entry and goal.
	Save(ctx context.Context, record servicedesk.EntrySLA, now time.Time) error
	// OfItem answers the entry's records, by goal.
	OfItem(ctx context.Context, itemID shared.ID) ([]servicedesk.EntrySLA, error)
	// Report counts the goals armed in [from, to) per policy, goal and standing.
	Report(ctx context.Context, from, to time.Time) ([]servicedesk.ReportRow, error)
}

// ReplyTemplates stores the workspace's canned replies.
type ReplyTemplates interface {
	// Insert writes a new template; a key the workspace holds is `reply_templates.key_taken`.
	Insert(ctx context.Context, template servicedesk.ReplyTemplate) error
	Update(ctx context.Context, template servicedesk.ReplyTemplate, expectedVersion int) error
	// Find answers the template of a key; shared.ErrNotFound when there is none.
	Find(ctx context.Context, key string) (servicedesk.ReplyTemplate, error)
	// List answers every template by key.
	List(ctx context.Context) ([]servicedesk.ReplyTemplate, error)
	Delete(ctx context.Context, key string) error
}

// Ratings stores how satisfied requesters were, one per request (migration 0243).
type Ratings interface {
	// Save writes the request's rating, replacing an earlier one, and answers it as stored.
	Save(ctx context.Context, rating servicedesk.Rating) (servicedesk.Rating, error)
	// OfItem answers the request's rating; shared.ErrNotFound when it has none.
	OfItem(ctx context.Context, itemID shared.ID) (servicedesk.Rating, error)
}

// Approvals stores the approvals a step opens, their decisions and people's delegations
// (migration 0268).
type Approvals interface {
	// Insert writes a new approval; an entry already waiting on one is `approvals.pending`.
	Insert(ctx context.Context, approval servicedesk.Approval) error
	// Update writes where the approval stands now, where its version is still expectedVersion.
	Update(ctx context.Context, approval servicedesk.Approval, expectedVersion int) error
	// Find answers one approval; shared.ErrNotFound when the workspace has none by the identifier.
	Find(ctx context.Context, id shared.ID) (servicedesk.Approval, error)
	// PendingOf answers the approval an entry waits on; shared.ErrNotFound when none.
	PendingOf(ctx context.Context, itemID shared.ID) (servicedesk.Approval, error)
	// OfItem answers an entry's approvals, newest first, at most fifty.
	OfItem(ctx context.Context, itemID shared.ID) ([]servicedesk.Approval, error)
	// Pending answers every approval waiting in the workspace, oldest first, at most limit.
	Pending(ctx context.Context, limit int) ([]servicedesk.Approval, error)
	// Record writes one decision.
	Record(ctx context.Context, decision servicedesk.Decision) error
	// Decisions answers an approval's decisions in the order they were given.
	Decisions(ctx context.Context, approvalID shared.ID) ([]servicedesk.Decision, error)
	// Delegate writes a person's delegation, replacing an earlier one.
	Delegate(ctx context.Context, delegation servicedesk.Delegation) error
	// DelegationOf answers a person's delegation; shared.ErrNotFound when none.
	DelegationOf(ctx context.Context, accountID shared.ID) (servicedesk.Delegation, error)
	// DelegationsTo answers the delegations naming a delegate, in force or not.
	DelegationsTo(ctx context.Context, delegateID shared.ID) ([]servicedesk.Delegation, error)
	// EndDelegation removes a person's delegation; false when there was none.
	EndDelegation(ctx context.Context, accountID shared.ID) (bool, error)
}

// Organisations stores a workspace's portal organisations (migration 0287): requesters grouped by
// mail domain, a domain in at most one of them.
type Organisations interface {
	// Insert writes a new organisation; a key the workspace holds is `organisations.key_taken`, a
	// domain another organisation holds `organisations.domain_taken`.
	Insert(ctx context.Context, organisation servicedesk.Organisation) error
	Update(ctx context.Context, organisation servicedesk.Organisation, expectedVersion int) error
	// Find answers the organisation of a key; shared.ErrNotFound when there is none.
	Find(ctx context.Context, key string) (servicedesk.Organisation, error)
	// List answers the organisations by key, at most servicedesk.MaxOrganisations.
	List(ctx context.Context) ([]servicedesk.Organisation, error)
	Count(ctx context.Context) (int, error)
	Delete(ctx context.Context, key string) error
	// OfDomain answers the organisation a mail domain belongs to; shared.ErrNotFound for none.
	OfDomain(ctx context.Context, domain string) (servicedesk.Organisation, error)
	// Members answers the active people whose address is at one of the domains, by account, at
	// most limit of them.
	Members(ctx context.Context, domains []string, limit int) ([]shared.ID, error)
}
