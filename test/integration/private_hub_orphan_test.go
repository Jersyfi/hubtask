// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/application/service/notification"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/text"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// lastMemberFor is work.LastMember the way main.go builds it.
func lastMemberFor(ctx context.Context, t *testing.T) work.LastMember {
	t.Helper()
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	return work.LastMember{
		Hubs: postgres.NewPrivateHubRepository(),
		Writer: work.ContainerWriter{
			Containers: containerRepo(), Events: postgres.NewOutbox(jobQueue(t)),
			Changes: postgres.NewChangeLog(), Audit: postgres.NewAuditSink(ids),
			UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)), Clock: fixed, IDs: ids, HLC: hybrid,
			Queue: jobQueue(t), Text: text.Composing{},
		},
		Owners: notification.RecordPrivateHubTrashed{
			Notifications: postgres.NewNotificationRepository(), Accounts: postgres.NewAccountRepository(),
			Preferences: postgres.NewNotificationPreferenceRepository(), Jobs: jobQueue(t),
			Clock: fixed, IDs: ids,
		},
	}
}

// orphanWorld is a workspace of its own - the pass reads every private hub of a tenant, and the
// shared tenants carry other tests' hubs - with an owner of the workspace, a person, and the
// person's private hub with a collection and an entry in it.
type orphanWorld struct {
	tenant, owner, person, hub, collection, item shared.ID
	hubName                                      string
}

func seedOrphanWorld(ctx context.Context, t *testing.T) orphanWorld {
	t.Helper()
	w := orphanWorld{
		tenant: freshID(t), owner: freshID(t), person: freshID(t),
		hub: freshID(t), collection: freshID(t), item: freshID(t),
	}
	w.hubName = "Tagebuch " + strings.ReplaceAll(w.hub.String(), "-", "")[20:]
	digits := strings.ReplaceAll(w.tenant.String(), "-", "")
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Family')`,
			[]any{w.tenant.String(), "orphan-" + digits[len(digits)-20:]}},
		{`INSERT INTO account (id, tenant_id, email, display_name) VALUES ($1, $2, $3, 'Paula Parent')`,
			[]any{w.owner.String(), w.tenant.String(), "paula-" + digits + "@example.org"}},
		{`INSERT INTO account (id, tenant_id, email, display_name) VALUES ($1, $2, $3, 'Kim Kind')`,
			[]any{w.person.String(), w.tenant.String(), "kim-" + digits + "@example.org"}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'OWNER')`,
			[]any{freshID(t).String(), w.tenant.String(), w.owner.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'MEMBER')`,
			[]any{freshID(t).String(), w.tenant.String(), w.person.String()}},
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by, private)
		  VALUES ($1, $2, 'HUB', $3, 'a0', $4, true)`,
			[]any{w.hub.String(), w.tenant.String(), w.hubName, w.person.String()}},
		{`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  VALUES ($1, $2, 'COLLECTION', $3, 'Notes', 'a0', $4)`,
			[]any{w.collection.String(), w.tenant.String(), w.hub.String(), w.person.String()}},
		{`INSERT INTO work_item (id, tenant_id, collection_id, type, title, path, depth, order_key, created_by)
		  VALUES ($1, $2, $3, 'TASK', 'Secret', $4, 1, 'a0', $5)`,
			[]any{w.item.String(), w.tenant.String(), w.collection.String(), w.item.String(), w.person.String()}},
	} {
		if _, err := adminPool(ctx, t).Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the workspace: %v\n%s", err, statement.sql)
		}
	}
	return w
}

// grant gives the account a role at the scope, directly in storage.
func (w orphanWorld) grant(ctx context.Context, t *testing.T, account shared.ID, scope identity.Scope, role identity.Role) identity.Grant {
	t.Helper()
	return grantOn(ctx, t, w.tenant, account, scope, role)
}

// assertOrphaned checks UC-ID-16 check 6 in storage: the hub and what is in it in the trash under
// one batch, by the system; the deletion in the trail with its reason; one membership mail per
// owner of the workspace, about nothing; and the hub's name nowhere in either.
func (w orphanWorld) assertOrphaned(ctx context.Context, t *testing.T) {
	t.Helper()
	var by string
	var batch *string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT coalesce(deleted_by_type, ''), trash_batch_id::text FROM container WHERE id = $1 AND deleted_at IS NOT NULL`,
		w.hub.String()).Scan(&by, &batch); err != nil {
		t.Fatalf("the hub is not in the trash: %v", err)
	}
	if by != "SYSTEM" || batch == nil {
		t.Errorf("trashed by %q in batch %v, want the system and a batch", by, batch)
	}
	if rows := countIn(ctx, t,
		`SELECT count(*) FROM work_item WHERE id = $1 AND trash_batch_id::text = $2`, w.item.String(), *batch); rows != 1 {
		t.Error("the entry did not go with the hub's batch")
	}
	if rows := countIn(ctx, t, `
		SELECT count(*) FROM audit_log
		WHERE tenant_id = $1 AND action = 'container.deleted' AND target_id = $2
		  AND actor_type = 'SYSTEM' AND changes->'reason'->>'to' = 'private_hub_orphaned'`,
		w.tenant.String(), w.hub.String()); rows != 1 {
		t.Errorf("%d deletions by the system with the reason in the trail, want 1", rows)
	}
	if rows := countIn(ctx, t, `
		SELECT count(*) FROM notification
		WHERE tenant_id = $1 AND recipient_id = $2 AND category = 'MEMBERSHIP'
		  AND item_id IS NULL AND rule_id IS NULL AND subscription_id IS NULL AND actor_id IS NULL`,
		w.tenant.String(), w.owner.String()); rows != 1 {
		t.Errorf("%d mails to the workspace's owner, want 1", rows)
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM notification WHERE tenant_id = $1 AND recipient_id <> $2`,
		w.tenant.String(), w.owner.String()); rows != 0 {
		t.Errorf("%d mails to somebody other than the workspace's owner", rows)
	}
	if rows := countIn(ctx, t, `
		SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND (changes::text LIKE $2 OR coalesce(target_label, '') LIKE $2)`,
		w.tenant.String(), "%"+w.hubName+"%"); rows != 0 {
		t.Errorf("the trail names the hub %d times", rows)
	}
}

func (w orphanWorld) assertKept(ctx context.Context, t *testing.T, hub shared.ID) {
	t.Helper()
	if rows := countIn(ctx, t, `SELECT count(*) FROM container WHERE id = $1 AND deleted_at IS NULL`, hub.String()); rows != 1 {
		t.Errorf("the hub %s went to the trash, and it has a member", hub)
	}
}

// UC-ID-16 check 6, the revocation: the last member gives up their role, and the hub goes; a hub
// where somebody still holds a role on a collection stays.
func TestRevokingTheLastMemberTrashesThePrivateHub(t *testing.T) {
	ctx := context.Background()
	w := seedOrphanWorld(ctx, t)
	own := w.grant(ctx, t, w.person, identity.HubScope(w.hub), identity.RoleOwner)

	// A second private hub of the person's, where somebody else holds a role on a collection.
	other, otherCollection := freshID(t), freshID(t)
	for _, statement := range []string{
		`INSERT INTO container (id, tenant_id, type, name, order_key, created_by, private) VALUES ($1, $2, 'HUB', 'Second', 'a1', $3, true)`,
		`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by) VALUES ($4, $2, 'COLLECTION', $1, 'Shared list', 'a0', $3)`,
	} {
		args := []any{other.String(), w.tenant.String(), w.person.String(), otherCollection.String()}
		if !strings.Contains(statement, "$4") {
			args = args[:3]
		}
		if _, err := adminPool(ctx, t).Exec(ctx, statement, args...); err != nil {
			t.Fatalf("seeding the second hub: %v", err)
		}
	}
	ownOther := w.grant(ctx, t, w.person, identity.HubScope(other), identity.RoleOwner)
	w.grant(ctx, t, w.owner, identity.CollectionScope(otherCollection), identity.RoleViewer)

	revoke := revokerFor(ctx, t)
	revoke.LastMember = lastMemberFor(ctx, t)
	revoke.StepUp = provenStepUp{}
	actor := reader(w.tenant, w.person)
	actor.Scopes = []string{"members:write"}
	for _, grant := range []identity.Grant{own, ownOther} {
		if err := revoke.Execute(ctx, actor, identityservice.RevokeMembershipCommand{
			MembershipID: grant.ID, StepUpToken: "proven",
		}); err != nil {
			t.Fatalf("revoking %s: %v", grant.ID, err)
		}
	}

	w.assertOrphaned(ctx, t)
	w.assertKept(ctx, t, other)
}

// UC-ID-16 check 6, the erasure, in both modes: an anonymised account is no member, and a deleted
// one takes its memberships with it - asked before they go.
func TestErasingTheLastMemberTrashesThePrivateHub(t *testing.T) {
	for _, mode := range []domain.ErasureMode{domain.ModeAnonymize, domain.ModeFullDelete} {
		t.Run(string(mode), func(t *testing.T) {
			ctx := context.Background()
			w := seedOrphanWorld(ctx, t)
			w.grant(ctx, t, w.person, identity.ItemScope(w.item), identity.RoleMember)

			eraser := eraserFor(t)
			eraser.LastMember = lastMemberFor(ctx, t)
			subject := person{tenant: w.tenant, account: w.person, item: w.item}
			if _, err := eraser.Erase(ctx, subjectActor(subject), erasureRequest(ctx, t, subject, mode)); err != nil {
				t.Fatalf("erasing: %v", err)
			}
			w.assertOrphaned(ctx, t)
		})
	}
}

// UC-ID-16 check 6, the catch-all: a private hub a path missed - its only member deleted by a
// cascade nothing asked about - is found by the retention pass; a private hub with a member, a
// shared hub without one and another workspace's orphan stay.
func TestTheRetentionPassTrashesAPrivateHubAPathMissed(t *testing.T) {
	ctx := context.Background()
	w := seedOrphanWorld(ctx, t)
	w.grant(ctx, t, w.person, identity.CollectionScope(w.collection), identity.RoleContributor)
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE account SET deleted_at = now() WHERE id = $1`, w.person.String()); err != nil {
		t.Fatalf("deleting the member behind every path's back: %v", err)
	}
	kept, sharedHub := freshID(t), freshID(t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by, private) VALUES ($1, $2, 'HUB', 'Kept', 'a1', $3, true)`,
			[]any{kept.String(), w.tenant.String(), w.owner.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role) VALUES ($1, $2, $3, 'HUB', $4, 'OWNER')`,
			[]any{freshID(t).String(), w.tenant.String(), w.owner.String(), kept.String()}},
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by) VALUES ($1, $2, 'HUB', 'Shared', 'a2', $3)`,
			[]any{sharedHub.String(), w.tenant.String(), w.owner.String()}},
	} {
		if _, err := adminPool(ctx, t).Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	neighbour := seedOrphanWorld(ctx, t)

	last := lastMemberFor(ctx, t)
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		_, err := last.Sweep(ctx, w.tenant)
		return err
	}); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	w.assertOrphaned(ctx, t)
	w.assertKept(ctx, t, kept)
	w.assertKept(ctx, t, sharedHub)
	neighbour.assertKept(ctx, t, neighbour.hub)
}
