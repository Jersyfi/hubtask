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
