// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// QS-19 against a real PostgreSQL (E-10): "a data subject requests erasure of their data - a
// `data_subject_request` with a deadline; every storage location from the data catalogue is served;
// audit references are pseudonymised; the deletion journal prevents return on restore."
//
// Against the database rather than fakes, because the claim is about *storage*: what an erasure
// leaves behind is a property of the rows, the cascades and the journal, and a fake of those would
// only test the fake.

// person is one data subject with something in every storage location an erasure has to serve.
// `seedSubject` rather than `seedPerson`, which the notification tests already use for a plain
// account: this one seeds a whole presence.
type person struct {
	tenant  shared.ID
	account shared.ID
	item    shared.ID
	comment shared.ID
	token   shared.ID
	feed    shared.ID
	media   shared.ID
}

func seedSubject(ctx context.Context, t *testing.T, email string) person {
	t.Helper()
	admin := adminPool(ctx, t)

	subject := person{
		tenant: freshID(t), account: freshID(t), item: freshID(t), comment: freshID(t),
		token: freshID(t), feed: freshID(t), media: freshID(t),
	}
	collection, hub := freshID(t), freshID(t)

	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Privacy')`,
			[]any{subject.tenant.String(), slugOf(subject.tenant)}},
		{`INSERT INTO account (id, tenant_id, email, display_name) VALUES ($1, $2, $3, 'Anna Beispiel')`,
			[]any{subject.account.String(), subject.tenant.String(), email}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		  VALUES ($1, $2, $3, 'TENANT', 'MEMBER')`,
			[]any{freshID(t).String(), subject.tenant.String(), subject.account.String()}},
		{`INSERT INTO access_token (id, tenant_id, account_id, name, token_prefix, token_hash, expires_at)
		  VALUES ($1, $2, $3, 'laptop', 'hbt_pat_', $4, now() + interval '30 days')`,
			[]any{subject.token.String(), subject.tenant.String(), subject.account.String(),
				"hash-" + subject.token.String()}},
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		  VALUES ($1, $2, 'HUB', 'Private', 'a0', $3)`,
			[]any{hub.String(), subject.tenant.String(), subject.account.String()}},
		{`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  VALUES ($1, $2, 'COLLECTION', $3, 'Shopping', 'a1', $4)`,
			[]any{collection.String(), subject.tenant.String(), hub.String(), subject.account.String()}},
		{`INSERT INTO work_item (id, tenant_id, collection_id, type, title, path, depth, order_key,
		    assignee_id, created_by)
		  VALUES ($1, $2, $3, 'TASK', 'Weekly shop', $4, 1, 'a0', $5, $5)`,
			[]any{subject.item.String(), subject.tenant.String(), collection.String(),
				subject.item.String(), subject.account.String()}},
		{`INSERT INTO comment (id, tenant_id, item_id, author_id, body)
		  VALUES ($1, $2, $3, $4, 'mine')`,
			[]any{subject.comment.String(), subject.tenant.String(), subject.item.String(),
				subject.account.String()}},
		{`INSERT INTO calendar_feed (id, tenant_id, account_id, view_id, token_hash)
		  VALUES ($1, $2, $3, NULL, $4)`,
			[]any{subject.feed.String(), subject.tenant.String(), subject.account.String(),
				"feed-" + subject.feed.String()}},
		{`INSERT INTO media_object (id, tenant_id, storage_key, mime_type, byte_size, checksum,
		    usage, created_by)
		  VALUES ($1, $2, $3, 'text/plain', 12, $4, 'ATTACHMENT', $5)`,
			[]any{subject.media.String(), subject.tenant.String(),
				"media/" + subject.media.String(), "sha-" + subject.media.String(),
				subject.account.String()}},
		{`INSERT INTO notification (id, tenant_id, recipient_id, category, channel, state)
		  VALUES ($1, $2, $3, 'ASSIGNMENT', 'EMAIL', 'SENT')`,
			[]any{freshID(t).String(), subject.tenant.String(), subject.account.String()}},
	}

	for _, statement := range statements {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the subject: %v\n%s", err, statement.sql)
		}
	}
	return subject
}

func privacyRepo() postgres.PrivacyRepository {
	return postgres.NewPrivacyRepository(pageCursors())
}

func eraserFor(t *testing.T) privacyservice.Eraser {
	t.Helper()
	ctx := context.Background()

	return privacyservice.Eraser{
		Requests: privacyRepo(), Erasure: privacyRepo(), Pseudonyms: privacyRepo(),
		Removals: postgres.NewLifecycleRepository(), Objects: nil,
		Audit:      postgres.NewAuditSink(generator{t}),
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock:      portclock.Fixed(created), TombstoneWindow: 90 * 24 * time.Hour,
	}
}

func subjectActor(subject person) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorSystem, TenantID: subject.tenant, AccountName: "the installation",
	}
}

func erasureRequest(subject person, mode domain.ErasureMode) domain.Request {
	return domain.Request{
		ID: freshIDOf(subject), Kind: domain.KindErasure, Status: domain.StatusInProgress,
		SubjectAccountID: subject.account, ErasureMode: mode,
	}
}

// freshIDOf keeps the case's identifier stable per subject, so that a failing assertion names the
// same row on a rerun.
func freshIDOf(subject person) shared.ID { return subject.item }

// QS-19, the full chain: every storage location, the journal, the tombstone, the pseudonym.
func TestAFullErasureServesEveryStorageLocation(t *testing.T) {
	ctx := context.Background()
	subject := seedSubject(ctx, t, "anna-"+freshID(t).String()+"@example.org")

	erased, err := eraserFor(t).Erase(ctx, subjectActor(subject),
		erasureRequest(subject, domain.ModeFullDelete))
	if err != nil {
		t.Fatalf("erasing: %v", err)
	}
	if !erased.AccountRemoved {
		t.Fatalf("the account was not removed: %+v", erased)
	}

	// Every location the data catalogue names for a person, checked in the database.
	for what, query := range map[string]string{
		"the account":         `SELECT count(*) FROM account WHERE id = $1`,
		"their memberships":   `SELECT count(*) FROM membership WHERE account_id = $1`,
		"their tokens":        `SELECT count(*) FROM access_token WHERE account_id = $1`,
		"their feeds":         `SELECT count(*) FROM calendar_feed WHERE account_id = $1`,
		"their comments":      `SELECT count(*) FROM comment WHERE author_id = $1`,
		"their notifications": `SELECT count(*) FROM notification WHERE recipient_id = $1`,
	} {
		if rows := countIn(ctx, t, query, subject.account.String()); rows != 0 {
			t.Errorf("%s: %d rows survived the erasure", what, rows)
		}
	}

	// The work stays and loses the person: an entry belongs to the workspace.
	var assignee *string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT assignee_id::text FROM work_item WHERE id = $1`, subject.item.String()).
		Scan(&assignee); err != nil {
		t.Fatalf("reading the entry: %v", err)
	}
	if assignee != nil {
		t.Errorf("the entry is still assigned to %v", *assignee)
	}

	// The deletion journal is what stops a restore bringing the person back (backup-restore.md §7).
	journal := countIn(ctx, t,
		`SELECT count(*) FROM deletion_journal WHERE tenant_id = $1 AND reason = 'DSR_ERASURE'`,
		subject.tenant.String())
	if journal < 2 {
		t.Errorf("%d journal entries were written - the comment and the account each owe one", journal)
	}
	// And the tombstone is what stops a device that was offline recreating it.
	if rows := countIn(ctx, t,
		`SELECT count(*) FROM tombstone WHERE tenant_id = $1 AND entity = 'account'`,
		subject.tenant.String()); rows != 1 {
		t.Errorf("%d tombstones were written for the account", rows)
	}

	// The trail is exempt from erasure and pseudonymises instead (audit.md §6).
	if rows := countIn(ctx, t,
		`SELECT count(*) FROM audit_pseudonym WHERE tenant_id = $1 AND actor_id = $2`,
		subject.tenant.String(), subject.account.String()); rows != 1 {
		t.Errorf("%d pseudonyms were recorded", rows)
	}
	// The erasure itself is in the trail, as a critical entry naming its occasion.
	if rows := countIn(ctx, t, `
		SELECT count(*) FROM audit_log
		WHERE tenant_id = $1 AND action = 'dsr.erased' AND legal_basis = 'dsr.erasure'
		  AND severity = 'CRITICAL'`, subject.tenant.String()); rows != 1 {
		t.Errorf("%d erasures were recorded in the trail", rows)
	}
}

// The other mode: the authorship stays, and everything of the person's in the account goes.
func TestAnAnonymisationKeepsTheWorkspacesContent(t *testing.T) {
	ctx := context.Background()
	subject := seedSubject(ctx, t, "bea-"+freshID(t).String()+"@example.org")

	if _, err := eraserFor(t).Erase(ctx, subjectActor(subject),
		erasureRequest(subject, domain.ModeAnonymize)); err != nil {
		t.Fatalf("erasing: %v", err)
	}

	var name, status string
	var email *string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT display_name, status::text, email FROM account WHERE id = $1`,
		subject.account.String()).Scan(&name, &status, &email); err != nil {
		t.Fatalf("the account did not survive an anonymisation: %v", err)
	}
	if name != privacyservice.FormerUser || status != "ANONYMIZED" || email != nil {
		t.Errorf("the account reads %q / %s / %v", name, status, email)
	}

	// The workspace's content - which belongs to third parties as much as to the person - stays.
	if rows := countIn(ctx, t, `SELECT count(*) FROM comment WHERE author_id = $1`,
		subject.account.String()); rows != 1 {
		t.Errorf("%d of the person's comments survived an anonymisation, want 1", rows)
	}
	// And the credentials still go: an anonymised account must not keep a token that works.
	if rows := countIn(ctx, t, `SELECT count(*) FROM access_token WHERE account_id = $1`,
		subject.account.String()); rows != 0 {
		t.Errorf("%d credentials survived", rows)
	}
}

// The one cross-tenant question, asked of the function that answers identifiers and nothing else.
func TestTheSubjectLookupAnswersWorkspacesAndNothingElse(t *testing.T) {
	ctx := context.Background()
	email := "carla-" + freshID(t).String() + "@example.org"
	first := seedSubject(ctx, t, email)
	second := seedSubject(ctx, t, email)
	stranger := seedSubject(ctx, t, "dora-"+freshID(t).String()+"@example.org")

	var tenants []shared.ID
	if err := read(ctx, t, first.tenant, func(ctx context.Context) error {
		var err error
		tenants, err = privacyRepo().Tenants(ctx, email)
		return err
	}); err != nil {
		t.Fatalf("looking the person up: %v", err)
	}

	found := map[shared.ID]bool{}
	for _, tenant := range tenants {
		found[tenant] = true
	}
	if !found[first.tenant] || !found[second.tenant] {
		t.Errorf("the person's workspaces came back as %v", tenants)
	}
	if found[stranger.tenant] {
		t.Error("somebody else's workspace came back")
	}

	// The address is matched the way the uniqueness index is: two spellings are one person.
	var upper []shared.ID
	if err := read(ctx, t, first.tenant, func(ctx context.Context) error {
		var err error
		upper, err = privacyRepo().Tenants(ctx, strings.ToUpper(email))
		return err
	}); err != nil {
		t.Fatalf("looking the person up: %v", err)
	}
	if len(upper) != len(tenants) {
		t.Errorf("a different spelling of the address answered %d workspaces", len(upper))
	}
}

// An erasure in one workspace is an erasure in one workspace.
func TestAnErasureDoesNotReachAnotherWorkspace(t *testing.T) {
	ctx := context.Background()
	email := "erik-" + freshID(t).String() + "@example.org"
	here := seedSubject(ctx, t, email)
	elsewhere := seedSubject(ctx, t, email)

	if _, err := eraserFor(t).Erase(ctx, subjectActor(here),
		erasureRequest(here, domain.ModeFullDelete)); err != nil {
		t.Fatalf("erasing: %v", err)
	}

	if rows := countIn(ctx, t, `SELECT count(*) FROM account WHERE id = $1`,
		elsewhere.account.String()); rows != 1 {
		t.Error("the erasure reached the person's account in another workspace")
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM comment WHERE author_id = $1`,
		elsewhere.account.String()); rows != 1 {
		t.Error("the erasure reached another workspace's content")
	}
}

// The restriction, written and read back: a state of the account rather than a lock.
func TestARestrictionIsAStateOfTheAccount(t *testing.T) {
	ctx := context.Background()
	subject := seedSubject(ctx, t, "frida-"+freshID(t).String()+"@example.org")

	if err := write(ctx, t, subject.tenant, func(ctx context.Context) error {
		written, err := privacyRepo().SetStatus(ctx, subject.account, "RESTRICTED", created)
		if err != nil {
			return err
		}
		if !written {
			t.Error("the account was not found")
		}
		return nil
	}); err != nil {
		t.Fatalf("restricting: %v", err)
	}

	// And the batch question the assignment policy asks before it draws.
	var restricted map[shared.ID]bool
	if err := read(ctx, t, subject.tenant, func(ctx context.Context) error {
		var err error
		restricted, err = postgres.NewAccountRepository().Restricted(ctx, []shared.ID{subject.account})
		return err
	}); err != nil {
		t.Fatalf("reading the restriction: %v", err)
	}
	if !restricted[subject.account] {
		t.Error("a restricted account is not reported as restricted")
	}
}
