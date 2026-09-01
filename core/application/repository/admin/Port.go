// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package admin holds the control plane's outbound ports (H-06, multi-tenancy.md §5).
//
// The methods on the tenant row itself deliberately take no tenant parameter, the same shape as
// every other repository: the control plane opens an ordinary bounded transaction per tenant it
// touches, and row level security holds even here. The two exceptions say so in their comments -
// the enumerator, which is decision 6's one legitimate listing, and the instance journal, which
// belongs to no tenant at all.
package admin

import (
	"context"
	"time"

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
	Version    int
}

// Tenants is the lifecycle of the tenant row.
type Tenants interface {
	// List answers every workspace, oldest first. It reads through the installation-scoped
	// SECURITY DEFINER enumerator migration 0067 pins down - the one legitimate place tenants
	// are enumerated (0.6.0 decision 6).
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

// Journal is the instance's own record. Append-only by grant; there is no read method because no
// API serves it - reading it is the operator's, at the database.
type Journal interface {
	// Record writes one entry. The table has no row-level-security policy, so this works inside
	// whatever transaction the act runs in - including the one that ends the tenant it names.
	Record(ctx context.Context, entry InstanceEvent) error
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
}

// ExportLoad answers how many export jobs of the transaction's tenant are alive - the §4
// concurrency quota's question (multi-tenancy.md §4; the general quota machinery is H-08's,
// this is the one limit H-07 already owes).
type ExportLoad interface {
	// LiveExports counts the tenant's pending and running export jobs.
	LiveExports(ctx context.Context) (int, error)
}
