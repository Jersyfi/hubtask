// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package privacy

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
	"github.com/Jersyfi/hubtask/test/dbtest"
)

// PG-2: after the erasure of a person, **no** storage location still holds personal data - apart
// from the permitted audit metadata (data-protection.md §10, ADR-0018, QS-19).
//
// ADR-0018 calls this "the expensive integration test" with "the highest protective value", and the
// value is in how it looks rather than in what it knows: it sweeps **every table in the schema** for
// the person's identifier and for their address, rather than checking the tables somebody
// remembered. A table added next year is covered on the day it is added, which is the whole of what
// risk R-09 - overlooked derived data - is about.
//
// What is permitted is named here rather than assumed, which is the other half of §10's wording:
// the trail keeps the actor's identifier because it is exempt from erasure and answers a pseudonym
// at the boundary (audit.md §6), and the deletion journal and the tombstone keep it because they
// are the records that stop a restore bringing the person back (ADR-0020 §6).

// permittedAfterErasure is where the person's identifier may still appear, and why.
//
// Keyed by **table and column** rather than by table. A whole table would be a licence: `work_item`
// may keep the identifier of whoever created an entry and must not grow a second column holding
// something of the person's, and a per-table exception could not tell the two apart.
var permittedAfterErasure = map[string]string{
	// The trail is exempt from erasure and cannot be edited in place - the grants, the trigger and
	// the hash chain all refuse it - and it answers a pseudonym at the boundary instead
	// (audit.md §6).
	"audit_log.actor_id":        "the trail is exempt from erasure; the boundary answers a pseudonym",
	"audit_log.on_behalf_of_id": "the same, for a rule or an agent acting for somebody",
	"audit_log.target_id":       "an entry about the person's own account names it as its target",
	"audit_pseudonym.actor_id":  "the substitution itself: an identifier and a label with no meaning outside the workspace",
	// The two records that stop the person coming back.
	"deletion_journal.entity_id": "the record that stops a restore bringing them back (backup-restore.md §7)",
	"tombstone.entity_id":        "the marker that stops a device which was offline recreating them (offline-sync.md §7)",
	// Authorship of the *workspace's* content. PG-2 surfaced these on its first run, and the
	// decision they force is the one ADR-0018 decision 5 already took: an entry, a hub and a
	// collection belong to the workspace as much as to whoever created them, and a full deletion
	// takes the person's own contributions rather than the workspace's record of its own work. What
	// is left is an identifier pointing at nobody - the same position `audit_log.actor_id` is in,
	// and `audit_pseudonym` is what renders it as "former user".
	"container.created_by":            "who created a hub or a collection: the workspace's own history, pointing at nobody now",
	"work_item.created_by":            "who created an entry: the same",
	"media_object.created_by":         "who uploaded a file that is attached to somebody else's work: the same",
	"automation_rule.created_by":      "who wrote a rule the workspace still runs: the same",
	"webhook_subscription.created_by": "who connected an integration the workspace still uses: the same",
	// The item's own history, which the catalogue takes with the item rather than with the person.
	"activity_entry.actor_id": "the item's history: `CASCADE` with the item (data-catalog.md), and it renders as a former user once the account is gone",
	// A dispatch record with a life of days, and a reference rather than content.
	"outbox_event.actor_id": "a dispatch record pruned 7 days after delivery (data-catalog.md): a reference, and nothing of the person's in it",
}

func TestPG2AnErasureLeavesNothingBehind(t *testing.T) {
	ctx := context.Background()
	subject, tenant, email := seedErasableSubject(ctx, t, runsAsARobot)

	objects := newBucket()
	eraser := eraserFor(ctx, t, objects)

	if _, err := eraser.Erase(ctx, appshared.ActorContext{
		Kind: appshared.ActorSystem, TenantID: tenant, AccountName: "the installation",
	}, domain.Request{
		ID: freshID(), Kind: domain.KindErasure, Status: domain.StatusInProgress,
		SubjectAccountID: subject, ErasureMode: domain.ModeFullDelete,
	}); err != nil {
		t.Fatalf("erasing: %v", err)
	}

	// The bytes, which no sweep of the database can see: the store was asked to release the
	// person's upload, and "the row is gone" is not the same statement.
	if !objects.deleted[mediaKey(subject)] {
		t.Errorf("the erasure removed the media row and left the bytes in the store (PG-2): "+
			"%v was asked to release nothing but %v", objects, objects.deleted)
	}

	// The sweep. Every table, every column that can hold an identifier or an address.
	for _, hit := range sweepFor(ctx, t, subject.String(), email) {
		if reason, permitted := permittedAfterErasure[hit.table+"."+hit.column]; permitted {
			t.Logf("%s.%s still names the person, and may: %s", hit.table, hit.column, reason)
			continue
		}
		t.Errorf("%s.%s still holds the erased person (%d row(s)) — PG-2. Either the erasure has "+
			"to serve that location, or it belongs in permittedAfterErasure with the reason it is "+
			"evidence rather than data", hit.table, hit.column, hit.rows)
	}
}

// eraserFor is the real service over the real repositories, with only the object store faked - a
// bucket is the one dependency a container cannot supply here, and what it was asked to release is
// exactly what the database cannot show.
func eraserFor(ctx context.Context, t *testing.T, objects *bucket) privacyservice.Eraser {
	t.Helper()
	return privacyservice.Eraser{
		Requests:   postgres.NewPrivacyRepository(cursors()),
		Erasure:    postgres.NewPrivacyRepository(cursors()),
		Pseudonyms: postgres.NewPrivacyRepository(cursors()),
		Removals:   postgres.NewLifecycleRepository(),
		Objects:    objects,
		Audit:      postgres.NewAuditSink(identifiers{}),
		UnitOfWork: postgres.NewUnitOfWork(dbtest.AppPool(ctx, t)),
		Clock:      portclock.Fixed(time.Now().UTC()),

		TombstoneWindow: 90 * 24 * time.Hour,
	}
}

// mediaKey is where the person's upload lives in the object store.
func mediaKey(subject shared.ID) string { return "media/" + subject.String() }

// bucket is an object store that remembers what was asked of it. The erasure's last step is bytes
// rather than rows, and a sweep of the database cannot see whether it happened.
type bucket struct {
	deleted map[string]bool
}

func newBucket() *bucket { return &bucket{deleted: map[string]bool{}} }

func (b *bucket) Delete(_ context.Context, key string) error { b.deleted[key] = true; return nil }

// Put and Get exist because the port has them. An erasure only ever deletes.
func (b *bucket) Put(context.Context, storage.Upload) error { return nil }

func (b *bucket) Get(context.Context, string) (storage.Object, error) {
	return storage.Object{}, shared.ErrNotFound.WithDetail("media.not_found")
}

// hit is one place the person is still named.
type hit struct {
	table  string
	column string
	rows   int
}

// sweepFor asks every table in the schema whether it still names the person.
//
// It reads the catalogue of columns from the database rather than from a list, which is what makes
// it survive the next migration: a `uuid` column is asked about the identifier, a `text` column
// about the address, and a table nobody thought of is asked like every other.
func sweepFor(ctx context.Context, t *testing.T, subject, email string) []hit {
	t.Helper()
	pool := dbtest.AdminPool(ctx, t)

	rows, err := pool.Query(ctx, `
		SELECT c.relname, a.attname, format_type(a.atttypid, a.atttypmod)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
		LEFT JOIN pg_inherits i ON i.inhrelid = c.oid
		WHERE c.relkind = 'r' AND i.inhrelid IS NULL
		  AND format_type(a.atttypid, a.atttypmod) IN ('uuid', 'text')
		ORDER BY 1, 2`)
	if err != nil {
		t.Fatalf("reading the schema: %v", err)
	}

	type candidate struct{ table, column, kind string }
	var candidates []candidate
	for rows.Next() {
		var found candidate
		if err := rows.Scan(&found.table, &found.column, &found.kind); err != nil {
			t.Fatalf("reading the schema: %v", err)
		}
		candidates = append(candidates, found)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("PG-2 found no column to sweep - the schema query no longer matches")
	}

	var hits []hit
	for _, column := range candidates {
		value := subject
		if column.kind == "text" {
			value = email
		}

		var count int
		query := fmt.Sprintf(`SELECT count(*) FROM %q WHERE %q::text = $1`, column.table, column.column)
		if err := pool.QueryRow(ctx, query, value).Scan(&count); err != nil {
			// A column the comparison cannot be made on - a generated one, a domain type - is
			// skipped rather than failing the sweep: what matters is that every column that *can*
			// hold the value is asked.
			continue
		}
		if count > 0 {
			hits = append(hits, hit{table: column.table, column: column.column, rows: count})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].table != hits[j].table {
			return hits[i].table < hits[j].table
		}
		return hits[i].column < hits[j].column
	})
	return hits
}

// Whom the workspace's automation acts as. A rule that acts as the person is the one thing that
// stops a full deletion, so the two cases are seeded from one function and told apart here.
const (
	runsAsARobot    = false
	runsAsThePerson = true
)

// seedErasableSubject puts a person into every location an erasure has to serve, and answers who
// they are.
func seedErasableSubject(
	ctx context.Context, t *testing.T, ruleRunsAsThePerson bool,
) (subject, tenant shared.ID, email string) {
	t.Helper()
	admin := dbtest.AdminPool(ctx, t)

	tenant, subject = freshID(), freshID()
	hub, collection, item := freshID(), freshID(), freshID()
	rule, subscription, robot := freshID(), freshID(), freshID()
	runAs := robot
	if ruleRunsAsThePerson {
		runAs = subject
	}
	email = "erasable-" + strings.ReplaceAll(subject.String(), "-", "") + "@example.org"

	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'PG-2')`,
			[]any{tenant.String(), "pg2-" + strings.ReplaceAll(tenant.String(), "-", "")}},
		{`INSERT INTO account (id, tenant_id, email, display_name) VALUES ($1, $2, $3, 'Anna Beispiel')`,
			[]any{subject.String(), tenant.String(), email}},
		{`INSERT INTO account (id, tenant_id, kind, email, display_name)
		  VALUES ($1, $2, 'SERVICE_ACCOUNT', $3, 'the automation')`,
			[]any{robot.String(), tenant.String(),
				"robot-" + strings.ReplaceAll(robot.String(), "-", "") + "@example.org"}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		  VALUES ($1, $2, $3, 'TENANT', 'MEMBER')`,
			[]any{freshID().String(), tenant.String(), subject.String()}},
		{`INSERT INTO access_token (id, tenant_id, account_id, name, token_prefix, token_hash, expires_at)
		  VALUES ($1, $2, $3, 'laptop', 'hbt_pat_', $4, now() + interval '30 days')`,
			[]any{freshID().String(), tenant.String(), subject.String(), "hash-" + subject.String()}},
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		  VALUES ($1, $2, 'HUB', 'Private', 'a0', $3)`,
			[]any{hub.String(), tenant.String(), subject.String()}},
		{`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  VALUES ($1, $2, 'COLLECTION', $3, 'Shopping', 'a1', $4)`,
			[]any{collection.String(), tenant.String(), hub.String(), subject.String()}},
		{`INSERT INTO work_item (id, tenant_id, collection_id, type, title, path, depth, order_key,
		    assignee_id, created_by)
		  VALUES ($1, $2, $3, 'TASK', 'Weekly shop', $4, 1, 'a0', $5, $5)`,
			[]any{item.String(), tenant.String(), collection.String(), item.String(), subject.String()}},
		{`INSERT INTO comment (id, tenant_id, item_id, author_id, body) VALUES ($1, $2, $3, $4, 'mine')`,
			[]any{freshID().String(), tenant.String(), item.String(), subject.String()}},
		{`INSERT INTO item_member (tenant_id, item_id, account_id) VALUES ($1, $2, $3)`,
			[]any{tenant.String(), item.String(), subject.String()}},
		{`INSERT INTO notification (id, tenant_id, recipient_id, category, channel, state)
		  VALUES ($1, $2, $3, 'ASSIGNMENT', 'EMAIL', 'SENT')`,
			[]any{freshID().String(), tenant.String(), subject.String()}},
		{`INSERT INTO notification_preference (tenant_id, account_id, category, channel, enabled)
		  VALUES ($1, $2, 'COMMENT', 'EMAIL', false)`,
			[]any{tenant.String(), subject.String()}},
		{`INSERT INTO consent_record (id, tenant_id, account_id, purpose, granted)
		  VALUES ($1, $2, $3, 'ai_processing', true)`,
			[]any{freshID().String(), tenant.String(), subject.String()}},
		{`INSERT INTO calendar_feed (id, tenant_id, account_id, view_id, token_hash)
		  VALUES ($1, $2, $3, NULL, $4)`,
			[]any{freshID().String(), tenant.String(), subject.String(), "feed-" + subject.String()}},
		{`INSERT INTO media_object (id, tenant_id, storage_key, mime_type, byte_size, checksum, usage, created_by)
		  VALUES ($1, $2, $3, 'text/plain', 12, $4, 'ATTACHMENT', $5)`,
			[]any{freshID().String(), tenant.String(), mediaKey(subject),
				"sha-" + subject.String(), subject.String()}},
		// The derived locations the acceptance of E-11 names one by one: the outbox, the rule runs,
		// the deliveries, the activity feed, the person's devices and what they sent in by mail.
		// Seeding them is the whole point - a sweep only proves something about a location that had
		// the person in it to begin with.
		{`INSERT INTO outbox_event (id, tenant_id, event_type, payload, actor_type, actor_id)
		  VALUES ($1, $2, 'de.hubtask.work.item.created.v1', '{}'::jsonb, 'USER', $3)`,
			[]any{freshID().String(), tenant.String(), subject.String()}},
		{`INSERT INTO activity_entry (id, tenant_id, item_id, actor_type, actor_id, verb)
		  VALUES ($1, $2, $3, 'USER', $4, 'item.created')`,
			[]any{freshID().String(), tenant.String(), item.String(), subject.String()}},
		{`INSERT INTO sync_device (id, tenant_id, account_id, platform, display_name, push_token)
		  VALUES ($1, $2, $3, 'ios', 'Annas Telefon', $4)`,
			[]any{freshID().String(), tenant.String(), subject.String(), "push-" + subject.String()}},
		{`INSERT INTO jumble_entry (id, tenant_id, channel, sender, raw_subject)
		  VALUES ($1, $2, 'EMAIL', $3, 'Fwd: the shopping list')`,
			[]any{freshID().String(), tenant.String(), email}},
		{`INSERT INTO automation_rule (id, tenant_id, name, scope_type, run_as, trigger, actions, created_by)
		  VALUES ($1, $2, 'when an entry is created', 'TENANT', $3, '{}'::jsonb, '[]'::jsonb, $4)`,
			[]any{rule.String(), tenant.String(), runAs.String(), subject.String()}},
		{`INSERT INTO rule_run (id, tenant_id, rule_id, status) VALUES ($1, $2, $3, 'SUCCEEDED')`,
			[]any{freshID().String(), tenant.String(), rule.String()}},
		{`INSERT INTO webhook_subscription (id, tenant_id, target_url, secret_enc, event_types, created_by)
		  VALUES ($1, $2, 'https://example.org/hook', '\x00'::bytea,
		          ARRAY['de.hubtask.work.item.created.v1'], $3)`,
			[]any{subscription.String(), tenant.String(), subject.String()}},
		{`INSERT INTO webhook_delivery (id, tenant_id, subscription_id, event_id, status)
		  VALUES ($1, $2, $3, $4, 'SUCCEEDED')`,
			[]any{freshID().String(), tenant.String(), subscription.String(), freshID().String()}},
	}

	for _, statement := range statements {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the subject: %v\n%s", err, statement.sql)
		}
	}
	return subject, tenant, email
}

// identifiers is the generator the audit sink takes. A counter would do; what matters is that two
// entries do not collide.
type identifiers struct{}

func (identifiers) NewID() shared.ID { return freshID() }

// freshID is a UUIDv7-shaped identifier in a namespace of this package's own, so that a row seeded
// here can never collide with one another suite wrote.
var sequence int64

func freshID() shared.ID {
	sequence++
	return shared.MustParseID(fmt.Sprintf("01936f2a-7c1e-7000-8e00-%012x", sequence))
}

// cursors is the page cursor codec the repositories take. A fixed secret, so that a cursor printed
// in a failing test is the same value on a rerun.
func cursors() security.CursorCodec {
	return security.NewCursorCodec(secret.New("privacy gate installation secret"))
}

// And the case PG-2 turned up on its way there: the person is the identity an automation rule acts
// as. `automation_rule.run_as` is the one reference to an account this schema declares `ON DELETE
// RESTRICT`, so before E-11 the deletion reached the database and came back as a foreign key
// violation - a dependency error, in a case with a statutory deadline, telling the operator
// nothing. It is a refusal with a count now.
func TestPG2AnErasureIsRefusedWhileARuleActsAsThePerson(t *testing.T) {
	ctx := context.Background()
	subject, tenant, _ := seedErasableSubject(ctx, t, runsAsThePerson)

	_, err := eraserFor(ctx, t, newBucket()).Erase(ctx, appshared.ActorContext{
		Kind: appshared.ActorSystem, TenantID: tenant, AccountName: "the installation",
	}, domain.Request{
		ID: freshID(), Kind: domain.KindErasure, Status: domain.StatusInProgress,
		SubjectAccountID: subject, ErasureMode: domain.ModeFullDelete,
	})

	problem := shared.AsError(err)
	if problem == nil || problem.DetailCode != domain.CodeErasureBlockedByRule {
		t.Fatalf("the erasure was not refused with a reason the operator can act on: %v", err)
	}
	if problem.Params["rules"] != "1" {
		t.Errorf("the refusal does not say what stands in the way: %v", problem.Params)
	}

	var accounts int
	if err := dbtest.AdminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM account WHERE id = $1`, subject.String()).Scan(&accounts); err != nil {
		t.Fatalf("counting the account: %v", err)
	}
	if accounts != 1 {
		t.Error("the refusal did not leave the account where it was")
	}
}
