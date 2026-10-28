// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// OwnDomains is the workspace's own domain and the block it still sees (ADR-0095 §4, migration
// 0211). Bound to the transaction's workspace: no method takes a tenant.
type OwnDomains interface {
	// Find answers the workspace's own domain; shared.ErrNotFound where it has none.
	Find(ctx context.Context) (identity.OwnDomain, error)
	// Insert writes a new one. A host another workspace answers at is identity.OwnDomainTaken.
	Insert(ctx context.Context, host identity.TenantHost) error
	// Remove deletes it and answers what it was; shared.ErrNotFound where there is none.
	Remove(ctx context.Context) (identity.OwnDomain, error)
	// Blocked is whether the operator blocked a host, whoever it was taken from.
	Blocked(ctx context.Context, host string) (bool, error)
	// BlockInView is the block the workspace's owner still sees; shared.ErrNotFound where none.
	BlockInView(ctx context.Context) (identity.DomainBlock, error)
	// DismissBlocks removes every block from the owner's view; the blocks stay.
	DismissBlocks(ctx context.Context, now time.Time) (int, error)
	// Block keeps a block, taken from the transaction's workspace. One already kept is
	// `admin.own_domain_already_blocked`.
	Block(ctx context.Context, block identity.DomainBlock) error
	// RecordLook writes one look's outcome, guarded on the state it was taken from; false where the
	// row moved or went in between.
	RecordLook(ctx context.Context, host string, from, next identity.HostState, look identity.DomainLookup, now time.Time) (bool, error)
	// AddressHost answers the workspace's own address row - its subdomain or assigned host - which
	// is the way back from an own domain; empty where it has none.
	AddressHost(ctx context.Context) (string, error)
	// MainToOwnDomain makes the active own domain the main address and the row that was main no
	// longer, answering that row's host; `admin.own_domain_not_active` where none is active. It
	// clears a fall-back's moment.
	MainToOwnDomain(ctx context.Context) (string, error)
	// MainToAddress makes the address row the main address again and answers it, empty where there
	// is none; a non-zero fellBack keeps that moment on the own domain (migration 0212).
	MainToAddress(ctx context.Context, fellBack time.Time) (string, error)
}

// BrowserHosts answers where a browser on a host is sent instead: the workspace's main address,
// where the host is a serving row beside it and one of the two is an own domain; empty otherwise
// (migration 0212). Callable in any scope.
type BrowserHosts interface {
	BrowserHost(ctx context.Context, host string) (string, error)
}

// DueOwnDomain is an own domain the check owes a look.
type DueOwnDomain struct {
	TenantID shared.ID
	Host     string
}

// ListedOwnDomain is one row of the operator's list.
type ListedOwnDomain struct {
	Host string
	// TenantID is the workspace; empty for a block no live workspace holds.
	TenantID      shared.ID
	TenantName    string
	State         identity.HostState
	Since         time.Time
	Main          bool
	BlockedReason string
	// FellBackAt is when a main one broke and its workspace's address row became main again.
	FellBackAt time.Time
	// AddressHost is the workspace's subdomain or assigned host, which a main one falls back to.
	AddressHost string
}

// HostStateBlocked is how the operator's list and the owner's view name a blocked domain. Not a
// row's state: a blocked domain has no row, only its block.
const HostStateBlocked identity.HostState = "BLOCKED"

// OwnDomainDoors cross workspaces through the SECURITY DEFINER functions of migration 0211, each
// answering only what its caller needs.
type OwnDomainDoors interface {
	// Due answers the own domains the check owes a look: pending and broken ones last looked at
	// before pendingBefore, serving ones before servingBefore; at most `most`.
	Due(ctx context.Context, pendingBefore, servingBefore time.Time, most int) ([]DueOwnDomain, error)
	// All answers every own domain and every block, for the operator.
	All(ctx context.Context) ([]ListedOwnDomain, error)
	// Certificate answers the ingress whether a host may have a certificate, and stamps an own
	// domain as asked.
	Certificate(ctx context.Context, host string, at time.Time) (bool, error)
}

// ServingHosts is what a TenantDirectory may answer beside ResolveHost: the workspace a host
// names, and whether it is a verified own domain the ingress asked about that awaits the first
// request which makes it active (ADR-0095 §5). Served makes it so.
type ServingHosts interface {
	ResolveServing(ctx context.Context, host string) (shared.ID, bool, error)
	Served(ctx context.Context, host string) (bool, error)
}
