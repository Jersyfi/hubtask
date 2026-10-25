// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func tallyOf(ctx context.Context, t *testing.T, tenant shared.ID, ids ...shared.ID) map[shared.ID]repository.ItemTally {
	t.Helper()

	var counted map[shared.ID]repository.ItemTally
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		counted, err = postgres.NewItemTallyRepository().TallyFor(ctx, ids)
		return err
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return counted
}

// attach puts a file on an entry the way AttachMedia leaves it: the object, and the link to it.
func attach(ctx context.Context, t *testing.T, tenant, item shared.ID, gone bool) {
	t.Helper()

	media := freshID(t)
	pool := adminPool(ctx, t)
	if _, err := pool.Exec(ctx, `INSERT INTO media_object (id, tenant_id, storage_key, mime_type, byte_size, usage, status)
		VALUES ($1, $2, $3, 'application/pdf', 12, 'ATTACHMENT', 'READY')`,
		media.String(), tenant.String(), "test/"+media.String()); err != nil {
		t.Fatalf("the file: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO item_attachment (tenant_id, item_id, media_id) VALUES ($1, $2, $3)`,
		tenant.String(), item.String(), media.String()); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	if gone {
		if _, err := pool.Exec(ctx, `UPDATE media_object SET deleted_at = now() WHERE id = $1`, media.String()); err != nil {
			t.Fatalf("removing the file: %v", err)
		}
	}
}

// What a card counts is what a reader would find: parts in the trash are not parts, a comment the
// server filed or one that was deleted is not somebody's comment, a file that is gone is not an
// attachment - and an entry holding nothing is answered with zeros, not left out.
func TestATallyCountsWhatAReaderWouldFind(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	bare := seedTask(ctx, t, tenantA, authorA, collection)

	children := childrenUnder(ctx, t, tenantA, authorA, collection, task, 3, 1)
	stampColumn(ctx, t, children[2], "deleted_at")

	seedComment(ctx, t, tenantA, task, authorA, "Draft is attached.", created)
	seedComment(ctx, t, tenantA, task, authorA, "Shorter, please.", created)
	removed := seedComment(ctx, t, tenantA, task, authorA, "Never mind.", created)
	pool := adminPool(ctx, t)
	if _, err := pool.Exec(ctx, `UPDATE comment SET deleted_at = now() WHERE id = $1`, removed.ID.String()); err != nil {
		t.Fatalf("deleting the comment: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO comment (id, tenant_id, item_id, author_id, body, kind, system_code)
		VALUES ($1, $2, $3, $4, 'the version that lost', 'SYSTEM', 'sync.displaced_version')`,
		freshID(t).String(), tenantA.String(), task.String(), authorA.String()); err != nil {
		t.Fatalf("filing a system comment: %v", err)
	}

	attach(ctx, t, tenantA, task, false)
	attach(ctx, t, tenantA, task, true)

	counted := tallyOf(ctx, t, tenantA, task, bare)
	want := repository.ItemTally{Parts: 2, PartsDone: 1, Comments: 2, Attachments: 1}
	if counted[task] != want {
		t.Errorf("the entry counts %+v, want %+v", counted[task], want)
	}
	zero, present := counted[bare]
	if !present || zero != (repository.ItemTally{}) {
		t.Errorf("an entry holding nothing is answered as %+v (present %v)", zero, present)
	}
}

// The cross-tenant negative (gate SG-3): another workspace's entries are not counted, not even as
// zeros - the row-level security removes them before anything is counted.
func TestATallyIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	childrenUnder(ctx, t, tenantA, authorA, collection, task, 2, 0)

	if counted := tallyOf(ctx, t, tenantB, task); len(counted) != 0 {
		t.Errorf("another tenant counted %+v", counted)
	}
}
