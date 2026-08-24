// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

// Package retention is the evidence catalogue of data-retention.md §7, run against a real
// PostgreSQL: the whole deletion path from the period in the table to the row that is gone.
//
// Not the same tests as the unit ones a layer down. Those prove that the engine decides correctly
// given what it was handed; these prove that what it is handed is what the database holds - the
// period, the holds, the order the rows go in, and the records left behind. RE-4, RE-7 and RE-9
// belong to the parts of ADR-0020 this milestone does not implement (the mark phase, the activation
// switch and multi-stage chains) and are not here; the gap is on the record in the task's issue.
package retention

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	work "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
	"github.com/Jersyfi/hubtask/test/dbtest"
)

// now is when every run in this file happens. Fixed, because the subject is a boundary in time and a
// test that read the wall clock would be testing the machine it runs on.
var now = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

// installationSecret keys the cursor codec. A fixed value rather than a random one, so that a cursor
// printed in a failing test is the same value on a rerun.
var installationSecret = secret.New("retention evidence installation secret")

// ids is the generator the server uses. One per suite, so that every identifier in this file is a
// real UUIDv7 rather than a constant that happens to parse.
var ids = clockadapter.NewUUIDv7(clockadapter.System{})

// freshID hands out one identifier, so that the suite can share a container without the tests
// sharing a trash.
func freshID(t *testing.T) shared.ID {
	t.Helper()
	return ids.NewID()
}

type suite struct {
	tenant shared.ID
	author shared.ID
	run    lifecycle.RunRetention
	uow    persistence.UnitOfWork
	admin  *pgxpool.Pool
	ctx    context.Context
}

// newSuite wires the real thing: the repositories the server wires, the engine it wires them into,
// and one tenant of its own.
func newSuite(t *testing.T, window time.Duration) *suite {
	t.Helper()
	ctx := context.Background()

	s := &suite{
		tenant: freshID(t), author: freshID(t), ctx: ctx,
		admin: dbtest.AdminPool(ctx, t),
		uow:   postgres.NewUnitOfWork(dbtest.AppPool(ctx, t)),
	}

	if _, err := s.admin.Exec(ctx,
		`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Retention')`,
		s.tenant.String(), slugFor(s.tenant)); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	if _, err := s.admin.Exec(ctx,
		`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Anna')`,
		s.author.String(), s.tenant.String()); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}

	store := postgres.NewLifecycleRepository()
	s.run = lifecycle.RunRetention{
		Policies: store, Runs: store, History: postgres.NewNotificationRepository(),
		Purger: lifecycle.Purger{
			Trash:    postgres.NewTrashRepository(security.NewCursorCodec(installationSecret)),
			Expired:  store,
			Holds:    store,
			Removals: store,
			Events:   postgres.NewOutbox(noJobs{}),
			Audit:    postgres.NewAuditSink(ids),
			Clock:    clock.Fixed(now),
			IDs:      ids,
			// The window is the test's, because it is one of the boundaries under test: RE-1 is
			// about the period and RE-6 is about the window, and a suite that shared one value
			// could only ever measure whichever of them was larger.
			TombstoneWindow: window,
			BatchSize:       100,
		},
		Clock: clock.Fixed(now),
		IDs:   ids,
	}
	return s
}

// noJobs is the queue as the outbox needs it here. The retention path writes events; whether a
// dispatcher is woken for them is the outbox's own evidence, not this catalogue's.
type noJobs struct{}

func (noJobs) Enqueue(context.Context, queue.Request) error            { return nil }
func (noJobs) Claim(context.Context, queue.Lease) ([]queue.Job, error) { return nil, nil }
func (noJobs) Complete(context.Context, queue.Job) error               { return nil }
func (noJobs) Repeat(context.Context, queue.Job, time.Time) error      { return nil }
func (noJobs) Fail(context.Context, queue.Failure) error               { return nil }
func (noJobs) Depth(context.Context) ([]queue.Depth, error)            { return nil, nil }

// slugFor turns an identifier into something the slug constraint accepts: lower case, no separators
// beyond the hyphen, and at most forty characters. A UUID with its hyphens kept is already all of
// that once it is prefixed with a letter.
func slugFor(id shared.ID) string {
	return "t" + strings.ToLower(id.String())
}

func (s *suite) actor() appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: s.tenant}
}

// collection seeds a hub with a collection in it and returns the collection.
func (s *suite) collection(t *testing.T) shared.ID {
	t.Helper()
	hubID, collectionID := freshID(t), freshID(t)

	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		VALUES ($1, $2, 'HUB', NULL, $3, 'a0', $5), ($4, $2, 'COLLECTION', $1, 'Shopping', 'a0', $5)`,
		hubID.String(), s.tenant.String(), "Hub "+hubID.String(), collectionID.String(),
		s.author.String()); err != nil {
		t.Fatalf("seeding the containers: %v", err)
	}
	return collectionID
}

// trashedItem writes one entry that went into the trash `daysAgo` days before the run.
func (s *suite) trashedItem(t *testing.T, collectionID shared.ID, daysAgo int) shared.ID {
	t.Helper()
	id := freshID(t)
	deletedAt := now.AddDate(0, 0, -daysAgo)

	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO work_item (
			id, tenant_id, collection_id, type, path, depth, title, order_key,
			deleted_at, trash_batch_id, created_by
		) VALUES ($1, $2, $3, 'TASK', $4, 1, $5, 'a0', $6, $7, $8)`,
		id.String(), s.tenant.String(), collectionID.String(), work.RootPath(id),
		"Entry "+id.String(), deletedAt, freshID(t).String(), s.author.String()); err != nil {
		t.Fatalf("seeding the trashed entry: %v", err)
	}
	return id
}

// sweep runs one pass as the worker would.
func (s *suite) sweep(t *testing.T) lifecycle.Outcome {
	t.Helper()

	var outcome lifecycle.Outcome
	err := s.uow.Within(s.ctx, persistence.Scope{TenantID: s.tenant}, func(ctx context.Context) error {
		var err error
		outcome, err = s.run.Execute(ctx, s.actor())
		return err
	})
	if err != nil {
		t.Fatalf("the retention run failed: %v", err)
	}
	return outcome
}

func (s *suite) exists(t *testing.T, id shared.ID) bool {
	t.Helper()

	var count int
	if err := s.admin.QueryRow(s.ctx,
		`SELECT count(*) FROM work_item WHERE id = $1`, id.String()).Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return count == 1
}

func (s *suite) count(t *testing.T, query string, args ...any) int {
	t.Helper()

	var count int
	if err := s.admin.QueryRow(s.ctx, query, args...).Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return count
}

// RE-1: a period removes exactly what is past it and nothing else, with the boundary tested on both
// sides of a single day.
func TestRE1ThePeriodRemovesExactlyWhatIsPastIt(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	collectionID := s.collection(t)

	// The default period is thirty days, so 31 goes and 29 stays. One day either side of the
	// boundary, which is where an off-by-one lives.
	past := s.trashedItem(t, collectionID, 31)
	inside := s.trashedItem(t, collectionID, 29)

	outcome := s.sweep(t)

	if outcome.Matched != 1 || outcome.Removed != 1 {
		t.Errorf("the pass reports %+v, want one matched and one removed", outcome)
	}
	if s.exists(t, past) {
		t.Error("an entry a day past its period is still there")
	}
	if !s.exists(t, inside) {
		t.Error("an entry a day short of its period was removed")
	}
}

// RE-2: a legal hold prevents the removal, and the run says so. Visible in the run rather than only
// refused, which is the acceptance criterion of the task: an operator has to be able to see that a
// deletion did not happen and why.
func TestRE2ALegalHoldBlocksTheRemovalAndIsVisibleInTheRun(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	collectionID := s.collection(t)
	held := s.trashedItem(t, collectionID, 60)

	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO legal_hold (id, tenant_id, scope_kind, scope_id, reason, placed_by)
		VALUES ($1, $2, 'ITEM', $3, 'Pending litigation', $4)`,
		freshID(t).String(), s.tenant.String(), held.String(), s.author.String()); err != nil {
		t.Fatalf("placing the hold: %v", err)
	}

	outcome := s.sweep(t)

	if outcome.Removed != 0 || outcome.Blocked[lifecycle.BlockedByLegalHold] != 1 {
		t.Errorf("the pass reports %+v, want nothing removed and one held", outcome)
	}
	if !s.exists(t, held) {
		t.Fatal("an entry under a legal hold was removed")
	}

	// And in the log, keyed by reason: "one was kept" is not something an operator can act on.
	if blocked := s.count(t, `
		SELECT blocked FROM retention_run WHERE tenant_id = $1 ORDER BY started_at DESC LIMIT 1`,
		s.tenant.String()); blocked != 1 {
		t.Errorf("the run logs %d blocked, want 1", blocked)
	}
	if reasons := s.count(t, `
		SELECT (blocked_reasons->>'legal_hold')::int FROM retention_run
		WHERE tenant_id = $1 ORDER BY started_at DESC LIMIT 1`,
		s.tenant.String()); reasons != 1 {
		t.Errorf("the run logs %d blocked by a legal hold, want 1", reasons)
	}
}

// RE-3: the lower bound cannot be undercut. A tenant that sets a period below the floor gets the
// floor, which is what stops a misconfigured rule emptying a trash the moment something lands in it.
func TestRE3TheLowerBoundCannotBeUndercut(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	collectionID := s.collection(t)

	// Seed the defaults, then set a period the floor forbids. The column's own CHECK refuses a
	// period below min_days, so the floor is lowered with it - which is exactly the misconfiguration
	// the domain has to survive rather than trust.
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO retention_policy (tenant_id, data_kind, retain_days, min_days)
		VALUES ($1, 'TRASH', 1, 1)`, s.tenant.String()); err != nil {
		t.Fatalf("writing the period: %v", err)
	}

	// Three days in the trash: past the tenant's one-day period, short of the seven-day floor.
	recent := s.trashedItem(t, collectionID, 3)

	outcome := s.sweep(t)

	if outcome.Matched != 0 || outcome.Removed != 0 {
		t.Errorf("the pass reports %+v, want nothing - the floor is seven days", outcome)
	}
	if !s.exists(t, recent) {
		t.Error("a period below the documented floor was honoured")
	}
}

// RE-5: a hard delete leaves no orphans. Every row that went has a journal entry and a tombstone,
// and nothing it referred to is left dangling.
func TestRE5AHardDeleteLeavesNoOrphans(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	collectionID := s.collection(t)
	removed := s.trashedItem(t, collectionID, 60)

	if outcome := s.sweep(t); outcome.Removed != 1 {
		t.Fatalf("the pass removed %d rows, want 1", outcome.Removed)
	}

	if s.exists(t, removed) {
		t.Fatal("the entry is still there")
	}
	if count := s.count(t, `
		SELECT count(*) FROM deletion_journal
		WHERE tenant_id = $1 AND entity = 'work_item' AND entity_id = $2 AND reason = 'RETENTION'`,
		s.tenant.String(), removed.String()); count != 1 {
		t.Errorf("%d journal entries for the removed row, want 1", count)
	}
	if count := s.count(t, `
		SELECT count(*) FROM tombstone
		WHERE tenant_id = $1 AND entity = 'work_item' AND entity_id = $2`,
		s.tenant.String(), removed.String()); count != 1 {
		t.Errorf("%d tombstones for the removed row, want 1", count)
	}
	// Nothing below it survived it either - the foreign keys would have taken them, and a row
	// removed by a cascade nobody counted is the orphan this test is named after.
	if count := s.count(t, `
		SELECT count(*) FROM work_item WHERE tenant_id = $1 AND path LIKE $2 || '%'`,
		s.tenant.String(), work.RootPath(removed)); count != 0 {
		t.Errorf("%d rows survive under the removed entry", count)
	}
	// And the run is closed rather than left saying RUNNING. Narrowed to the trash, because a pass
	// logs one run per data kind and the notification history's is beside it (C-09).
	if count := s.count(t, `
		SELECT count(*) FROM retention_run
		WHERE tenant_id = $1 AND data_kind = 'TRASH'
		  AND status = 'SUCCEEDED' AND finished_at IS NOT NULL`,
		s.tenant.String()); count != 1 {
		t.Errorf("%d closed trash runs, want 1", count)
	}
}

// RE-6: the minimum tombstone period is observed. A deletion past its retention period but still
// inside the maximum offline window stays, and the run counts it - a device that has not checked in
// must have the chance to learn of the deletion before the object disappears for good.
func TestRE6TheTombstoneWindowHoldsTheRemovalBack(t *testing.T) {
	s := newSuite(t, 90*24*time.Hour)
	collectionID := s.collection(t)

	// Sixty days: well past the thirty-day period, well inside the ninety-day window.
	waiting := s.trashedItem(t, collectionID, 60)
	// Two hundred: past both.
	overdue := s.trashedItem(t, collectionID, 200)

	outcome := s.sweep(t)

	if outcome.Removed != 1 {
		t.Errorf("the pass removed %d rows, want 1", outcome.Removed)
	}
	if outcome.Blocked[lifecycle.BlockedByTombstoneWindow] != 1 {
		t.Errorf("the pass reports %+v, want one held by the offline window", outcome)
	}
	if !s.exists(t, waiting) {
		t.Error("an entry still inside the offline window was removed")
	}
	if s.exists(t, overdue) {
		t.Error("an entry past both bounds is still there")
	}
	if reasons := s.count(t, `
		SELECT (blocked_reasons->>'tombstone_window')::int FROM retention_run
		WHERE tenant_id = $1 ORDER BY started_at DESC LIMIT 1`,
		s.tenant.String()); reasons != 1 {
		t.Errorf("the run logs %d held by the window, want 1", reasons)
	}
}

// RE-8: one tenant's run never reaches another tenant's objects. The boundary is row level security
// rather than a filter in the query, which is why this is worth proving against the database: the
// run names no tenant anywhere.
func TestRE8ARunNeverReachesAnotherTenant(t *testing.T) {
	first := newSuite(t, 24*time.Hour)
	second := newSuite(t, 24*time.Hour)

	mine := first.trashedItem(t, first.collection(t), 60)
	theirs := second.trashedItem(t, second.collection(t), 60)

	outcome := first.sweep(t)

	if outcome.Matched != 1 || outcome.Removed != 1 {
		t.Errorf("the pass reports %+v, want only this tenant's entry", outcome)
	}
	if first.exists(t, mine) {
		t.Error("this tenant's entry survived its own run")
	}
	if !second.exists(t, theirs) {
		t.Fatal("one tenant's retention run removed another tenant's entry")
	}
	if count := second.count(t, `
		SELECT count(*) FROM deletion_journal WHERE tenant_id = $1`,
		second.tenant.String()); count != 0 {
		t.Errorf("%d journal entries landed in the untouched tenant", count)
	}
	if count := second.count(t, `
		SELECT count(*) FROM retention_run WHERE tenant_id = $1`,
		second.tenant.String()); count != 0 {
		t.Errorf("%d runs were logged against the untouched tenant", count)
	}
}

// A pass that filled its batch says so, which is how the job knows to come back at once rather than
// waiting out the interval with a trash still full.
func TestAFullBatchIsReportedAsUnfinished(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	s.run.Purger.BatchSize = 2
	collectionID := s.collection(t)
	for range 3 {
		s.trashedItem(t, collectionID, 60)
	}

	outcome := s.sweep(t)

	if outcome.Removed != 2 {
		t.Errorf("the pass removed %d rows, want the batch of 2", outcome.Removed)
	}
	if s.run.Exhausted(outcome) {
		t.Error("a full batch reads as the end of the trash")
	}

	if outcome := s.sweep(t); !s.run.Exhausted(outcome) {
		t.Errorf("the second pass reports %+v, want the end of the trash", outcome)
	}
	if left := s.count(t,
		`SELECT count(*) FROM work_item WHERE tenant_id = $1`, s.tenant.String()); left != 0 {
		t.Errorf("%d entries are left after two passes", left)
	}
}

// The NOTIFICATION class of data-retention.md §3, against the real table: seeded at ninety days
// on the first pass, and enforced by the same run that empties the trash (C-09).
func TestTheNotificationHistoryIsSweptAtNinetyDays(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	ctx := s.ctx

	expired := s.seedNotification(ctx, t, s.author, now.AddDate(0, 0, -91))
	fresh := s.seedNotification(ctx, t, s.author, now.AddDate(0, 0, -89))

	s.sweep(t)

	// The period is in the table rather than in the code, which is what the pass seeded on its
	// way through (ADR-0020).
	var retainDays int
	if err := s.admin.QueryRow(ctx,
		`SELECT retain_days FROM retention_policy WHERE tenant_id = $1 AND data_kind = 'NOTIFICATION'`,
		s.tenant.String()).Scan(&retainDays); err != nil {
		t.Fatalf("reading the seeded period: %v", err)
	}
	if retainDays != 90 {
		t.Errorf("the seeded period is %d days, want 90", retainDays)
	}

	if s.notificationExists(ctx, t, expired) {
		t.Error("a record past its period survived the sweep")
	}
	if !s.notificationExists(ctx, t, fresh) {
		t.Error("a record inside its period was swept")
	}

	// And the run is in the log under its own kind, so an operator can see that the notification
	// history is being kept to as well as the trash.
	var runs int
	if err := s.admin.QueryRow(ctx,
		`SELECT count(*) FROM retention_run WHERE tenant_id = $1 AND data_kind = 'NOTIFICATION'`,
		s.tenant.String()).Scan(&runs); err != nil {
		t.Fatalf("reading the run log: %v", err)
	}
	if runs != 1 {
		t.Errorf("%d logged runs for the notification history, want 1", runs)
	}
}

// seedNotification writes one record straight into the table: what this file is about is the
// period, and how a record comes to exist is the notification path's own evidence.
func (s *suite) seedNotification(
	ctx context.Context, t *testing.T, recipient shared.ID, createdAt time.Time,
) shared.ID {
	t.Helper()

	id := ids.NewID()
	if _, err := s.admin.Exec(ctx, `
		INSERT INTO notification (id, tenant_id, recipient_id, category, channel, state, created_at)
		VALUES ($1, $2, $3, 'INVITATION', 'EMAIL', 'SENT', $4)`,
		id.String(), s.tenant.String(), recipient.String(), createdAt); err != nil {
		t.Fatalf("seeding the notification: %v", err)
	}
	return id
}

func (s *suite) notificationExists(ctx context.Context, t *testing.T, id shared.ID) bool {
	t.Helper()

	var found bool
	if err := s.admin.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM notification WHERE id = $1)`, id.String()).Scan(&found); err != nil {
		t.Fatalf("reading the notification: %v", err)
	}
	return found
}
