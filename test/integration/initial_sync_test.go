// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The initial synchronisation against a real database and the real service (N-02): a member of
// the hub walks the workspace from nothing to a delta cursor, every kind this test seeded is in
// the walk, a change landing mid-walk is on the first delta, and the walk's size and duration are
// measured rather than assumed.

func pullFor(ctx context.Context, t *testing.T) syncservice.PullChanges {
	t.Helper()
	return syncservice.PullChanges{Stream: streamFor(ctx, t), Snapshot: snapshotRepo()}
}

// memberOf gives a fresh account a role on the hub, the way hubWithRole does for a hub it creates.
func memberOf(ctx context.Context, t *testing.T, tenant, hub shared.ID, role string) shared.ID {
	t.Helper()
	account := seedAccount(ctx, t, tenant)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		 VALUES ($1, $2, $3, 'HUB', $4, $5)`,
		freshID(t).String(), tenant.String(), account.String(), hub.String(), role); err != nil {
		t.Fatalf("seeding the membership: %v", err)
	}
	return account
}

func TestAMemberWalksTheWorkspaceFromNothingToADeltaCursor(t *testing.T) {
	ctx := context.Background()
	f := seedSnapshot(ctx, t)
	member := memberOf(ctx, t, tenantA, f.hub, "MEMBER")
	pull := pullFor(ctx, t)
	actor := streamActor(tenantA, member)

	seen := map[string]map[shared.ID]bool{}
	var pages, records int
	request := syncservice.PullRequest{DeviceID: freshID(t), Limit: 7}
	started := time.Now()
	var cursor syncservice.Position
	for {
		page, err := pull.Pull(ctx, actor, request)
		if err != nil {
			t.Fatalf("pulling page %d: %v", pages, err)
		}
		pages++
		for _, record := range page.Records {
			records++
			if seen[record.Entity] == nil {
				seen[record.Entity] = map[shared.ID]bool{}
			}
			seen[record.Entity][record.EntityID] = true
			if record.Payload == nil {
				t.Errorf("a walk record without a payload: %+v", record)
			}
		}
		if page.More && !page.Cursor.Walking() {
			t.Fatalf("a page with more ended on a delta cursor")
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
	elapsed := time.Since(started)
	t.Logf("walked %d records in %d pages of 7 in %v", records, pages, elapsed)

	for entity, id := range map[string]shared.ID{
		"container": f.collection, "bucket": f.bucket.ID, "label": f.label.ID, "item": f.task,
		"comment": f.comment.ID, "reminder": f.reminder.ID, "recurrence_rule": f.rule.ID,
		"template": f.template.ID,
	} {
		if !seen[entity][id] {
			t.Errorf("the %s this test seeded is not in the walk", entity)
		}
	}
	if cursor.Walking() {
		t.Fatalf("the walk ended on a walk cursor: %+v", cursor)
	}

	// A change landing after the walk began is on the first delta.
	edited := recordChange(ctx, t, tenantA, member, f.collection, "item")
	delta, err := pull.Pull(ctx, actor, syncservice.PullRequest{DeviceID: request.DeviceID, Cursor: pull.Encode(cursor)})
	if err != nil {
		t.Fatalf("pulling the delta: %v", err)
	}
	found := false
	for _, record := range delta.Records {
		if record.EntityID == edited {
			found = true
		}
	}
	if !found {
		t.Errorf("the change recorded after the walk is not on the first delta")
	}
}

// A member of a different hub is told about nothing this test seeded: the permission is checked
// per row, by the container the row belongs to.
func TestAMemberOfAnotherHubWalksNoneOfIt(t *testing.T) {
	ctx := context.Background()
	f := seedSnapshot(ctx, t)
	elsewhere := hubWithRole(ctx, t, tenantA, seedAccount(ctx, t, tenantA), "")
	stranger := memberOf(ctx, t, tenantA, elsewhere, "MEMBER")
	pull := pullFor(ctx, t)

	request := syncservice.PullRequest{DeviceID: freshID(t), Limit: 100}
	for pages := 0; ; pages++ {
		page, err := pull.Pull(ctx, streamActor(tenantA, stranger), request)
		if err != nil {
			t.Fatalf("pulling: %v", err)
		}
		for _, record := range page.Records {
			if record.ContainerID == f.collection || record.ContainerID == f.hub {
				t.Errorf("a stranger was handed %s %s", record.Entity, record.EntityID)
			}
		}
		if !page.More {
			return
		}
		request.Cursor = pull.Encode(page.Cursor)
		if pages > 1000 {
			t.Fatalf("the walk does not end")
		}
	}
}

// The snapshot against the real database (SY-C, P-12): the same records as the page sequence in
// the same order, and a cursor at its end that the delta accepts without a gap - a change landing
// after the snapshot began is on the first delta, as it is after a walk in pages.
func TestTheSnapshotAnswersThePageSequenceAndEndsOnACursorTheDeltaAccepts(t *testing.T) {
	ctx := context.Background()
	f := seedSnapshot(ctx, t)
	member := memberOf(ctx, t, tenantA, f.hub, "MEMBER")
	pull := pullFor(ctx, t)
	actor := streamActor(tenantA, member)

	var paged []syncservice.Record
	request := syncservice.PullRequest{DeviceID: freshID(t), Limit: 7}
	for pages := 0; ; pages++ {
		page, err := pull.Pull(ctx, actor, request)
		if err != nil {
			t.Fatalf("pulling page %d: %v", pages, err)
		}
		paged = append(paged, page.Records...)
		if !page.More {
			break
		}
		request.Cursor = pull.Encode(page.Cursor)
		if pages > 1000 {
			t.Fatalf("the walk does not end")
		}
	}

	var streamed []syncservice.Record
	started := time.Now()
	cursor, err := pull.WalkAll(ctx, actor, syncservice.SnapshotRequest{DeviceID: freshID(t)}, func(record syncservice.Record) error {
		streamed = append(streamed, record)
		return nil
	})
	if err != nil {
		t.Fatalf("the snapshot: %v", err)
	}
	t.Logf("streamed %d records in %v, against %d in pages", len(streamed), time.Since(started), len(paged))
	if len(streamed) != len(paged) {
		t.Fatalf("the snapshot streamed %d records, the pages %d", len(streamed), len(paged))
	}
	for i := range streamed {
		if streamed[i].Entity != paged[i].Entity || streamed[i].EntityID != paged[i].EntityID {
			t.Errorf("record %d: the snapshot has %s %s, the pages %s %s", i,
				streamed[i].Entity, streamed[i].EntityID, paged[i].Entity, paged[i].EntityID)
		}
	}
	if cursor.Walking() {
		t.Fatalf("the snapshot ended on a walk cursor: %+v", cursor)
	}

	edited := recordChange(ctx, t, tenantA, member, f.collection, "item")
	delta, err := pull.Pull(ctx, actor, syncservice.PullRequest{DeviceID: request.DeviceID, Cursor: pull.Encode(cursor)})
	if err != nil {
		t.Fatalf("pulling the delta after the snapshot: %v", err)
	}
	found := false
	for _, record := range delta.Records {
		if record.EntityID == edited {
			found = true
		}
	}
	if !found {
		t.Errorf("the change recorded after the snapshot is not on the first delta")
	}
}
