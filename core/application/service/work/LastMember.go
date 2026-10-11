// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ReasonPrivateHubOrphaned is the audit entry's reason when the system trashed a private hub
// nobody was left in.
const ReasonPrivateHubOrphaned = "private_hub_orphaned"

// LastMember moves a private hub into the trash once nobody is left in it (ADR-0073 §5).
//
// Nobody else may open a private hub, so one without a member is a hub nobody can reach - not the
// workspace's owners, who hold nothing in it. Trashed rather than purged: the ordinary grace runs,
// and the workspace's owners are told that one went, without its name. Taking it over is the
// emergency access; letting it go is the purge.
//
// Asked by whatever ends a membership - a revocation, an erasure - in that act's own transaction,
// and by the retention pass for whatever a path missed. A member is an account, neither
// anonymised nor deleted, holding a role on the hub, on a collection in it or on an entry in one.
type LastMember struct {
	Hubs   repository.OrphanedHubs
	Writer ContainerWriter
	// Owners tells the workspace's owners. Optional: wired without it the hub still goes to the
	// trash, which is the half nobody could do by hand.
	Owners OwnersNotice
}

// OwnersNotice tells the workspace's owners that a private hub went to the trash, naming nothing
// - the notification context's recorder.
type OwnersNotice interface {
	PrivateHubTrashed(ctx context.Context, tenantID shared.ID, recipients []shared.ID) error
}

// HeldBy answers the private hubs the account is a member of, for a caller about to remove the
// account's memberships wholesale.
func (l LastMember) HeldBy(ctx context.Context, accountID shared.ID) ([]shared.ID, error) {
	return l.Hubs.HeldBy(ctx, accountID)
}

// AfterMemberLeft trashes those of the hubs the named hubs, collections or entries sit under that
// have no member left, inside the caller's transaction, and answers how many went. Nothing named
// asks about nothing.
func (l LastMember) AfterMemberLeft(
	ctx context.Context, tenantID shared.ID, named []shared.ID,
) (int, error) {
	if len(named) == 0 {
		return 0, nil
	}
	return l.settle(ctx, tenantID, named)
}

// Sweep trashes every private hub of the workspace without a member: the retention pass's
// catch-all for a hub a path missed - a cascade, a pod older than this rule.
func (l LastMember) Sweep(ctx context.Context, tenantID shared.ID) (int, error) {
	return l.settle(ctx, tenantID, nil)
}

func (l LastMember) settle(ctx context.Context, tenantID shared.ID, named []shared.ID) (int, error) {
	candidates, err := l.Hubs.Orphaned(ctx, named)
	if err != nil || len(candidates) == 0 {
		return 0, err
	}
	// Asked again under the hubs' locks: a grant under a hub takes the same row, so a person given
	// a role a moment ago is seen here rather than losing the hub they were just given.
	if err := l.Hubs.Lock(ctx, candidates); err != nil {
		return 0, err
	}
	orphaned, err := l.Hubs.Orphaned(ctx, candidates)
	if err != nil || len(orphaned) == 0 {
		return 0, err
	}

	system := appshared.ActorContext{
		Kind: appshared.ActorSystem, TenantID: tenantID, AccountName: "the installation",
	}
	for _, hubID := range orphaned {
		if err := l.Writer.trashOrphanedHub(ctx, system, hubID); err != nil {
			return 0, err
		}
	}

	if l.Owners == nil {
		return len(orphaned), nil
	}
	owners, err := l.Hubs.WorkspaceOwners(ctx)
	if err != nil {
		return 0, err
	}
	for range orphaned {
		if err := l.Owners.PrivateHubTrashed(ctx, tenantID, owners); err != nil {
			return 0, err
		}
	}
	return len(orphaned), nil
}

// trashOrphanedHub moves the hub into the trash as the system, inside the caller's transaction.
//
// No permission is asked: nobody asked for this, and the system holds no role. What it writes is
// what a person's deletion writes - the batch, the event, the change log for devices, the audit
// entry - with the reason the system had.
func (w ContainerWriter) trashOrphanedHub(
	ctx context.Context, system appshared.ActorContext, hubID shared.ID,
) error {
	now := w.Clock.Now()
	hub, err := findContainer(ctx, w.Containers, hubID)
	if err != nil {
		return err
	}
	batch := w.IDs.NewID()
	wanted, changes, err := hub.Trashed(now, batch, deletedBy(system))
	if err != nil || len(changes) == 0 {
		return err
	}
	_, err = w.writeTrash(ctx, system, orphanedHub, repository.ContainerTrash{
		Container: wanted, BatchID: batch,
	}, hub.Version, now)
	return err
}
