// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// B-5 (backup-restore.md §12, N-11) against the database: a workspace whose device holds a valid
// cursor is restored into - rows land without change log entries and the epoch advances as the
// restore succeeds, which is what Applier.succeed does through the same repository - and the
// device's next pull is refused, the full synchronisation delivers the restored rows, and the
// stream refuses the same cursor.
func TestARestoredWorkspaceResynchronisesByItself(t *testing.T) {
	ctx := context.Background()
	tenant, author := seedMaterialisingTenant(ctx, t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'OWNER')`,
		freshID(t).String(), tenant.String(), author.String()); err != nil {
		t.Fatalf("seeding the membership: %v", err)
	}
	_, collection := hubWithCollection(ctx, t, tenant, author)
	pull, stream, actor := pullFor(ctx, t), streamFor(ctx, t), streamActor(tenant, author)

	// The device synchronises from nothing and holds a valid delta cursor.
	request := syncservice.PullRequest{DeviceID: freshID(t), Limit: 100}
	var cursor syncservice.Position
	for pages := 0; ; pages++ {
		page, err := pull.Pull(ctx, actor, request)
		if err != nil {
			t.Fatalf("walking: %v", err)
		}
		if !page.More {
			cursor = page.Cursor
			break
		}
		request.Cursor = pull.Encode(page.Cursor)
		if pages > 1000 {
			t.Fatalf("the walk does not end")
		}
	}
	if cursor.Epoch != 0 {
		t.Fatalf("a fresh workspace's cursor is from epoch %d, want the start", cursor.Epoch)
	}
	if _, err := pull.Pull(ctx, actor, syncservice.PullRequest{DeviceID: request.DeviceID, Cursor: pull.Encode(cursor), Limit: 100}); err != nil {
		t.Fatalf("the cursor is not valid before the restore: %v", err)
	}

	// The restore: a row lands without a change log entry, and the epoch advances with the
	// success, in one transaction.
	restored := seedTask(ctx, t, tenant, author, collection)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		_, err := postgres.NewEpochRepository().Advance(ctx)
		return err
	}); err != nil {
		t.Fatalf("advancing the epoch: %v", err)
	}

	_, err := pull.Pull(ctx, actor, syncservice.PullRequest{DeviceID: request.DeviceID, Cursor: pull.Encode(cursor), Limit: 100})
	if !errors.Is(err, shared.ErrGone) || shared.AsError(err).DetailCode != "sync.cursor_too_old" {
		t.Fatalf("the pull after the restore answered %v, want cursor_too_old", err)
	}
	if _, err := stream.Resume(ctx, actor, stream.Encode(cursor)); !errors.Is(err, shared.ErrGone) {
		t.Errorf("the stream resumed a cursor from before the restore: %v", err)
	}

	// Starting over, the walk delivers the restored row under the new epoch.
	request = syncservice.PullRequest{DeviceID: request.DeviceID, Limit: 100}
	found := false
	for pages := 0; ; pages++ {
		page, err := pull.Pull(ctx, actor, request)
		if err != nil {
			t.Fatalf("walking again: %v", err)
		}
		for _, record := range page.Records {
			found = found || record.EntityID == restored
		}
		if page.Cursor.Epoch != 1 {
			t.Fatalf("a position after the restore is from epoch %d, want 1", page.Cursor.Epoch)
		}
		if !page.More {
			break
		}
		request.Cursor = pull.Encode(page.Cursor)
		if pages > 1000 {
			t.Fatalf("the walk does not end")
		}
	}
	if !found {
		t.Error("the full synchronisation did not deliver the restored row")
	}
}

// A restore into a new workspace advances nothing (the applier's rule, tested there); here, that
// a workspace nobody restored into stands at its start, and that another tenant's restore does
// not move it - the cross-tenant negative for both methods (gate SG-3).
func TestTheEpochIsTheWorkspacesOwn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	fresh, _ := seedMaterialisingTenant(ctx, t)
	epochs := postgres.NewEpochRepository()

	var before, after, foreign int64
	if err := read(ctx, t, fresh, func(ctx context.Context) error {
		var err error
		before, err = epochs.Current(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if before != 0 {
		t.Errorf("a new workspace starts at epoch %d", before)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		foreign, err = epochs.Advance(ctx)
		return err
	}); err != nil {
		t.Fatalf("advancing the other tenant: %v", err)
	}
	if err := read(ctx, t, fresh, func(ctx context.Context) error {
		var err error
		after, err = epochs.Current(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading again: %v", err)
	}
	if after != before {
		t.Errorf("another tenant's restore moved this workspace's epoch from %d to %d", before, after)
	}
	if foreign < 1 {
		t.Errorf("the other tenant's advance answered %d", foreign)
	}
}
