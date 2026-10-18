// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package admin holds the control plane's outbound ports (multi-tenancy.md §5).
//
// The methods on the tenant row itself deliberately take no tenant parameter, the same shape as
// every other repository: the control plane opens an ordinary bounded transaction per tenant it
// touches, and row level security holds even here. The two exceptions say so in their comments -
// the enumerator, which is the one legitimate listing of tenants (multi-tenancy.md §2.1), and the instance journal, which
// belongs to no tenant at all.
package admin

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// TenantRecord is one workspace as the control plane sees it: the row, no settings document -
// what the lifecycle needs and the listing answers, nothing a tenant configures.
type TenantRecord struct {
	ID              shared.ID
	Slug            string
	DisplayName     string
	Status          identity.TenantStatus
	DefaultLocale   string
	DefaultTimeZone string
	CreatedAt       time.Time
	// PurgeAfter is set while a deletion request stands: when the grace runs out. Zero otherwise.
	PurgeAfter time.Time
	// PasswordOpening is an operator's opening of the password, as the row holds it (ADR-0078 §3).
	// Whether it still stands is the reader's to judge against its clock.
	PasswordOpening identity.PasswordOpening
	// LegalHold says a legal hold is in force in the workspace, so it is not deleted
	// (data-protection.md §5). A state only: which hold, on what and why stay the workspace's.
	LegalHold bool
	// StatusBeforeDeletion is where the workspace stood when its deletion was requested, and where
	// a cancel returns it. Empty on a row requested before the column existed.
	StatusBeforeDeletion identity.TenantStatus
	// PurgeStartedAt is when the grace job began removing the workspace. Zero until then; from then
	// on a cancel is refused, because the bytes are already going.
	PurgeStartedAt time.Time
	Version        int
	// PlanName is the short name of the workspace's plan, empty for none (multi-tenancy.md §4.2).
	PlanName string
	// ExternalReference is a purchase platform's own reference for the workspace, empty for none
	// (multi-tenancy.md §4.3). It may name a customer: it goes into no log, metric or journal.
	ExternalReference string
}

// TenantQuery is the control plane's filters over the workspaces; an empty field filters nothing.
type TenantQuery struct {
	ID          shared.ID
	Slug        string
	Reference   string
	Plan        string
	WithoutPlan bool
	Status      identity.TenantStatus
	Cursor      string
	Size        int
}

// TenantFinder finds workspaces across the installation (multi-tenancy.md §4.3), through the one
// narrow door that filters and pages across tenant_self.
type TenantFinder interface {
	// Query answers one page of the matching workspaces, oldest first. A cursor this server did not
	// produce is a validation error with code `shared.cursor_invalid`.
	Query(ctx context.Context, query TenantQuery) ([]TenantRecord, PageInfo, error)
}

// Tenants is the lifecycle of the tenant row.
type Tenants interface {
	// List answers every workspace, oldest first. It reads through the installation-scoped
	// SECURITY DEFINER enumerator migration 0067 pins down - the one legitimate place tenants
	// are enumerated (multi-tenancy.md §2.1).
	List(ctx context.Context) ([]TenantRecord, error)

	// Insert writes the row, inside the new tenant's own scope: the first write a tenant ever
	// sees is already bounded to it. A taken slug is a conflict error with code
	// `admin.slug_taken`.
	Insert(ctx context.Context, record TenantRecord) error

	// Find answers the row of the transaction's own tenant, or an error wrapping
	// shared.ErrNotFound.
	Find(ctx context.Context) (TenantRecord, error)

	// SetStatus moves one lifecycle edge on the transaction's own tenant. False means the
	// origin status did not stand - the state moved under the caller, who reports the conflict
	// rather than overwriting it.
	SetStatus(ctx context.Context, from, to identity.TenantStatus, now time.Time) (bool, error)

	// RequestDeletion moves either living status to PENDING_DELETION and stamps the grace
	// deadline - the one edge with two origins (§5). False means the workspace was already
	// leaving, or gone.
	RequestDeletion(ctx context.Context, purgeAfter, now time.Time) (bool, error)

	// CancelDeletion moves a pending workspace back to the status it was requested from, or to
	// SUSPENDED where none was kept, and clears the deadline. It answers the status written; false
	// means the workspace was not pending, or its purge had begun - the statement guards both.
	CancelDeletion(ctx context.Context, now time.Time) (identity.TenantStatus, bool, error)

	// OpenPassword writes an operator's opening of the password on the transaction's own tenant,
	// replacing one that stands (ADR-0078 §3). False means the workspace is not active - suspended,
	// leaving, or gone.
	OpenPassword(ctx context.Context, opening identity.PasswordOpening, now time.Time) (bool, error)

	// SetReference sets a platform's reference on the transaction's own tenant, or clears it where
	// reference is empty, raising its version. False means the version moved since `version` was
	// read, or the purge has begun. A reference another workspace holds is a conflict error with code
	// `admin.reference_taken`, carrying neither the value nor the database's detail.
	SetReference(ctx context.Context, reference string, version int, now time.Time) (bool, error)

	// SetPlan puts the transaction's own tenant on a plan, or none where planID is zero, raising its
	// version. False means the version moved since `version` was read, or the purge has begun.
	SetPlan(ctx context.Context, planID shared.ID, version int, now time.Time) (bool, error)

	// ClosePassword ends the opening on the transaction's own tenant: whatever stands where `due` is
	// zero, and only one whose end has come by `due` otherwise - so a close at the old end leaves a
	// later opening alone. False means there was none to end.
	ClosePassword(ctx context.Context, due, now time.Time) (bool, error)
}

// Automations is the one switch the deletion request throws (§5, "automations disabled"): every
// enabled rule of the transaction's tenant, off in one stroke, visibly - the rows keep existing
// with enabled = false.
type Automations interface {
	// DisableAll switches the tenant's enabled rules off and reports how many.
	DisableAll(ctx context.Context, now time.Time) (int, error)
}

// InstanceEvent is one row of the installation's own journal: evidence of a control-plane act,
// where a per-tenant trail cannot hold it (audit.md §6). Identifiers, a slug, counts - never
// content, and never an end user's name.
type InstanceEvent struct {
	ID         shared.ID
	OccurredAt time.Time
	// Action is the act, `tenant.provisioned` through `tenant.hard_deleted`.
	Action string
	// TenantID and TenantSlug name the workspace the act was about. Bare values, no reference:
	// the row they name is usually gone, which is the reason the journal exists.
	TenantID   shared.ID
	TenantSlug string
	// ActorLabel is the acting operator's own label - the installation's administrator, never a
	// tenant's person.
	ActorLabel string
	// Details carries the counts and moments of the act, as the evidence entry records them.
	Details map[string]any
}

// Journal is the instance's own record. Append-only by grant, and read by one screen: the
// dashboard's journal, newest first, one page at a time.
type Journal interface {
	// Record writes one entry. The table has no row-level-security policy, so this works inside
	// whatever transaction the act runs in - including the one that ends the tenant it names.
	Record(ctx context.Context, entry InstanceEvent) error

	// Page walks the journal backwards from `cursor`, empty for the first page. The entries and
	// whether there is another page behind them.
	Page(ctx context.Context, cursor string, size int) ([]InstanceEvent, PageInfo, error)
}

// PlatformEvent is one installation event as it is kept: the envelope and its number, which is
// the feed's order.
type PlatformEvent struct {
	Number   int64
	Envelope event.Envelope
}

// PlatformPosition is where a reader of the installation's events stands: after the event numbered
// Number, having read up to the moment At. The moment is what decides whether events after the
// position may already have been swept.
type PlatformPosition struct {
	Number int64
	At     time.Time
}

// PlatformEvents are the installation's events for a purchase platform (multi-tenancy.md §4.3).
// Like the journal they belong to no tenant and carry no policy: written beside a workspace's
// change, in that workspace's transaction, and read under the installation's scope.
type PlatformEvents interface {
	// Append writes the event in the running transaction, numbered after every event whose
	// transaction committed before this one does: it takes the installation's numbering lock,
	// held until the transaction ends. A non-empty dedupe key writes the event once however often
	// it is appended, and Append reports whether this call wrote it.
	Append(ctx context.Context, envelope event.Envelope, dedupeKey string) (bool, error)

	// After reads up to `limit` events numbered after `after`, in their order.
	After(ctx context.Context, after int64, limit int) ([]PlatformEvent, error)

	// ForgetReference removes the platform's reference from the kept events of the transaction's
	// own workspace, as its purge removes it from the workspace.
	ForgetReference(ctx context.Context) error

	// Sweep removes up to `batch` events that occurred before `cutoff`, and answers how many.
	Sweep(ctx context.Context, cutoff time.Time, batch int) (int, error)
}

// PageInfo is the walk's state, as every other listing in this product reports it.
type PageInfo struct {
	NextCursor string
	HasMore    bool
}

// Census is the installation at a glance (ADR-0070 §5): counts, states and limits, never
// rows. The tenant boundary is a database policy rather than a role, and the dashboard does not go
// around it - what it reads is five integers, through a function that can answer nothing else.
type Census struct {
	WorkspacesActive          int64
	WorkspacesSuspended       int64
	WorkspacesPendingDeletion int64
	AccountsActive            int64
	AccountsTotal             int64
	// WorkspacesServiceOnly are workspaces holding no person, only service accounts: not customers,
	// so the three workspace counts above leave them out (UC-INS-12 check 5).
	WorkspacesServiceOnly int64
}

// Installation answers the census.
type Installation interface {
	Census(ctx context.Context) (Census, error)
}

// Footprint is the §5 stores, counted: what the evidence entry records before the fall, and what
// the acceptance proof counts to zero afterwards. Search rides inside the items (their vectors
// are columns of the row), so the item count is the search count.
type Footprint struct {
	Items        int64
	Containers   int64
	MediaObjects int64
	MediaBytes   int64
	OutboxEvents int64
	AuditEntries int64
}

// Purge is the hard delete's surface (§5's final phase). Every method except the two that say
// otherwise runs inside the dying tenant's own bounded transaction.
type Purge interface {
	// Footprint counts the stores before the fall.
	Footprint(ctx context.Context) (Footprint, error)

	// StorageKeys pages through the tenant's object keys, for the store-first byte deletion -
	// ReconcileMedia's order: an orphaned byte object is reclaimable, an orphaned row lies.
	StorageKeys(ctx context.Context, after string, batch int) ([]string, error)

	// DropStructure fells the structure in dependency order - collections, hubs, media rows,
	// automation rules, restore runs, retention rules - because a bare cascade would trip its
	// own RESTRICT edges. It reports how many rows fell.
	DropStructure(ctx context.Context) (int64, error)

	// DeleteOutbox removes one batch of the tenant's outbox and reports how many; the caller
	// loops until zero. No foreign key reaches the outbox.
	DeleteOutbox(ctx context.Context, batch int) (int, error)

	// DeleteIdempotency clears the replay guards; no foreign key reaches them either.
	DeleteIdempotency(ctx context.Context) (int, error)

	// ArchiveUsage moves the workspace's daily counts to the installation's ledger, keyed by the
	// bare id with no name and no reference, before the row goes and the cascade takes them
	// (multi-tenancy.md §4.3). It reports how many counts moved.
	ArchiveUsage(ctx context.Context) (int64, error)

	// DeleteJobs removes the tenant's queue rows except the one running this purge - the job
	// table has no policy, so the adapter writes the transaction's own tenant as an explicit
	// predicate.
	DeleteJobs(ctx context.Context, keep shared.ID) (int, error)

	// PurgeTrail removes the tenant's audit trail through the one narrow SECURITY DEFINER act
	// migration 0067 reasons through, aimed by the transaction's own scope, and reports how
	// many entries went.
	PurgeTrail(ctx context.Context) (int64, error)

	// HardDelete removes the tenant row itself; the cascade takes everything still standing.
	// The guard re-reads the two facts the grace could have changed - false means the state
	// moved, and the caller rolls the whole act back.
	HardDelete(ctx context.Context, now time.Time) (bool, error)

	// MarkStarted writes the purge's mark in the tenant row before the first byte goes, so that a
	// cancel can no longer land. False means the workspace is not pending or its deadline lies
	// ahead - a cancel came first, and nothing may be removed.
	MarkStarted(ctx context.Context, now time.Time) (bool, error)
}
