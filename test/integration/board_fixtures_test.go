// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
)

// What the board tests (domain-model.md §3.5a) share against a real database: the column writer,
// columns and entries seeded straight into the tables, and reading them back.

// onlyAdministrator lets one account through: the permission question itself is the application
// layer's and is tested there.
type onlyAdministrator struct{ account shared.ID }

func (a onlyAdministrator) Authorize(_ context.Context, actor appshared.ActorContext, _ access.Request) error {
	if actor.AccountID != a.account {
		return shared.ErrForbidden
	}
	return nil
}

func boardColumns(ctx context.Context, t *testing.T) work.BucketWriter {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	boards := work.Boards{Buckets: bucketRepo(), Statuses: bucketStatusRepo()}
	return work.BucketWriter{
		Buckets: bucketRepo(), Containers: containerRepo(), Authorizer: onlyAdministrator{account: authorA},
		Events: postgres.NewOutbox(jobQueue(t)), Changes: postgres.NewChangeLog(),
		Audit: postgres.NewAuditSink(ids), UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
		Text: textadapter.Forms{}, Boards: boards,
		Activity: work.ActivityJournal{Entries: historyRepo(), IDs: ids},
	}
}

func seedBoardColumn(ctx context.Context, t *testing.T, tenant, collection shared.ID, orderKey string, done bool) domain.Bucket {
	t.Helper()
	column := bucketIn(tenant, collection, freshID(t), freshName(t), orderKey)
	column.IsDoneBucket = done
	if err := write(ctx, t, tenant, func(ctx context.Context) error { return bucketRepo().Insert(ctx, column) }); err != nil {
		t.Fatalf("seeding the column: %v", err)
	}
	return column
}

func seedOnBoard(ctx context.Context, t *testing.T, tenant, author, collection, column shared.ID, completed bool, status string) shared.ID {
	t.Helper()
	item := taskIn(tenant, author, collection, freshID(t), freshName(t), "a0")
	item.BucketID, item.Status = column, status
	if completed {
		item = item.Completed(author, created)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		if _, err := itemRepo().Insert(ctx, item); err != nil || !completed {
			return err
		}
		// The insert carries no completion; it is written as completing does.
		return itemRepo().SetCompletion(ctx, item, item.Version)
	}); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}
	return item.ID
}

func readEntry(ctx context.Context, t *testing.T, tenant, id shared.ID) domain.WorkItem {
	t.Helper()
	var item domain.WorkItem
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		item, err = itemRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the entry: %v", err)
	}
	return item
}

func liveColumns(ctx context.Context, t *testing.T, tenant, collection shared.ID) []domain.Bucket {
	t.Helper()
	var columns []domain.Bucket
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		columns, err = bucketRepo().List(ctx, collection)
		return err
	}); err != nil {
		t.Fatalf("reading the board: %v", err)
	}
	return columns
}
