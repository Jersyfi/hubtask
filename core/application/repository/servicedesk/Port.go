// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package servicedesk is the persistence port of the service desk's building blocks: business
// calendars, SLA policies and each entry's SLA record (migration 0240).
package servicedesk

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PendingQuery selects the waiting approvals that may be one reader's: People are the reader and
// those whose approvals the reader decides now; the cursor is the last row of the page before.
type PendingQuery struct {
	People           []shared.ID
	AfterRequestedAt time.Time
	AfterID          shared.ID
	Limit            int
}

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
	// KeyHeld answers whether any hub keeps its own version of the key. The key itself is the
	// use case's to check: the installation's calendars are no rows (migration 0327).
	KeyHeld(ctx context.Context, key string) (bool, error)
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
	// Report counts the goals armed in [from, to) per policy, goal and standing, the OLAs' beside
	// the requester's.
	Report(ctx context.Context, from, to time.Time) ([]servicedesk.ReportRow, error)
}

// OLARecords stores the goals of the OLAs on an entry (UC-SVC-02 check 10): one per entry, policy
// and goal, apart from Records - an entry carries one requester's promise and any number of teams'.
type OLARecords interface {
	SaveOLA(ctx context.Context, record servicedesk.EntrySLA, now time.Time) error
	// OLAsOf answers the entry's OLA records, by policy and goal.
	OLAsOf(ctx context.Context, itemID shared.ID) ([]servicedesk.EntrySLA, error)
	// FindOLA answers one record, the subject of its timers; shared.ErrNotFound when it is gone.
	FindOLA(ctx context.Context, id shared.ID) (servicedesk.EntrySLA, error)
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
	// PendingFor answers, oldest first after the query's cursor and at most its limit, the waiting
	// approvals whose current level counts one of its people or names a role.
	PendingFor(ctx context.Context, query PendingQuery) ([]servicedesk.Approval, error)
	// Record writes one decision.
	Record(ctx context.Context, decision servicedesk.Decision) error
	// DecisionsOf answers the decisions of several approvals by approval, each in the order given.
	DecisionsOf(ctx context.Context, approvalIDs []shared.ID) (map[shared.ID][]servicedesk.Decision, error)
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
}

// PublicForms stores a collection's public form (UC-SVC-13). It alone knows how a token becomes
// its hash - the pepper is the adapter's secret (security.md §8) - so the port takes the token
// whole.
type PublicForms interface {
	// Put writes the collection's link under a new token, replacing the one it had.
	Put(ctx context.Context, form servicedesk.PublicForm, token integration.FormLinkToken) error
	// Find answers the collection's link; found false when it has none.
	Find(ctx context.Context, collectionID shared.ID) (servicedesk.PublicForm, bool, error)
	// FindByToken answers the link a token names inside the caller's transaction, or not found.
	FindByToken(ctx context.Context, token integration.FormLinkToken) (servicedesk.PublicForm, error)
	// Delete takes the collection's link away and says whether it had one.
	Delete(ctx context.Context, collectionID shared.ID) (bool, error)
}

// DeskLogos stores the logo a collection's portal draws (UC-SVC-05 check 9).
type DeskLogos interface {
	// Find answers the collection's logo, zero for none; not found for a hub or a trashed collection.
	Find(ctx context.Context, collectionID shared.ID) (shared.ID, error)
	// Set writes the collection's logo; zero takes it away.
	Set(ctx context.Context, collectionID, mediaID shared.ID) error
}

// SatisfactionLinks stores the satisfaction question mailed to a requester without an account
// (UC-SVC-07 check 6), hashing its token as PublicForms does.
type SatisfactionLinks interface {
	// Put writes the requester's link on the entry under a new token, replacing the one it had.
	Put(ctx context.Context, link servicedesk.SatisfactionLink, token integration.SatisfactionLinkToken) error
	// FindByToken answers the link a token names inside the caller's transaction, or not found.
	FindByToken(ctx context.Context, token integration.SatisfactionLinkToken) (servicedesk.SatisfactionLink, error)
	// Use marks the link answered at now and says whether it still was unanswered and alive: of
	// two answers at once, one wins.
	Use(ctx context.Context, token integration.SatisfactionLinkToken, now time.Time) (bool, error)
}
