// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

// Package retention is the evidence catalogue of data-retention.md §7, run against a real
// PostgreSQL: the whole deletion path from the period in the table to the row that is gone.
//
// Not the same tests as the unit ones a layer down. Those prove that the engine decides correctly
// given what it was handed; these prove that what it is handed is what the database holds - the
// period, the holds, the order the rows go in, and the records left behind. RE-4, RE-7 and RE-9
// arrived with E-07, which built the parts of ADR-0020 they are about: the marking phase, the
// activation switch and multi-stage chains.
package retention

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	lifecycleDomain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
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
	store  postgres.LifecycleRepository
	window time.Duration
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
	s.store = store
	s.window = window
	s.run = s.engineAt(now)
	return s
}

// engineAt is the engine as the server wires it, at one instant.
//
// A function rather than a field, because a chain is a thing that happens over years: RE-9 runs the
// same engine twice, at two moments that are two periods apart, and an engine with the clock baked
// in could only ever prove the first stage.
func (s *suite) engineAt(at time.Time) lifecycle.RunRetention {
	store := s.store
	purger := lifecycle.Purger{
		Trash:    postgres.NewTrashRepository(security.NewCursorCodec(installationSecret)),
		Expired:  store,
		Holds:    store,
		Removals: store,
		Events:   postgres.NewOutbox(noJobs{}),
		Audit:    postgres.NewAuditSink(ids),
		Clock:    clock.Fixed(at),
		IDs:      ids,
		// The window is the test's, because it is one of the boundaries under test: RE-1 is about
		// the period and RE-6 is about the window, and a suite that shared one value could only
		// ever measure whichever of them was larger.
		TombstoneWindow: s.window,
		BatchSize:       100,
	}
	return lifecycle.RunRetention{
		Policies: store, Runs: store, History: postgres.NewNotificationRepository(),
		Purger: purger,
		Clock:  clock.Fixed(at),
		IDs:    ids,
		Rules:  postgres.NewRetentionRuleRepository(),
		Sweeper: lifecycle.Sweeper{
			Rules:   postgres.NewRetentionRuleRepository(),
			Marking: postgres.NewRetentionMarkingRepository(),
			Holds:   store,
			Items:   postgres.NewItemRepository(security.NewCursorCodec(installationSecret)),
			Purger:  purger,
			Changes: postgres.NewChangeLog(),
			Clock:   clock.Fixed(at), IDs: ids, HLC: hybridAt(at), Batch: 100,
		},
	}
}

// hybridAt stamps the changes a marking writes. One per engine, because a hybrid clock counts
// within itself and two engines sharing one would order their entries against each other rather
// than against the moment each ran at.
func hybridAt(at time.Time) *clockadapter.HybridClock {
	hybrid, err := clockadapter.NewHybridClock(clock.Fixed(at), "retention-evidence")
	if err != nil {
		panic(err)
	}
	return hybrid
}

// noJobs is the queue as the outbox needs it here. The retention path writes events; whether a
// dispatcher is woken for them is the outbox's own evidence, not this catalogue's.
type noJobs struct{}

func (noJobs) Enqueue(context.Context, queue.Request) (shared.ID, error) { return "", nil }
func (noJobs) Claim(context.Context, queue.Lease) ([]queue.Job, error)   { return nil, nil }
func (noJobs) Hold(context.Context, queue.Job) error                     { return nil }
func (noJobs) Complete(context.Context, queue.Job) error                 { return nil }
func (noJobs) Repeat(context.Context, queue.Job, time.Time) error        { return nil }
func (noJobs) Fail(context.Context, queue.Failure) error                 { return nil }
func (noJobs) Depth(context.Context) ([]queue.Depth, error)              { return nil, nil }

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

// ── The rule model (E-07) ─────────────────────────────────────────────────────────────────────

// completedItem writes one entry that was finished `daysAgo` days before the run.
func (s *suite) completedItem(t *testing.T, collectionID shared.ID, daysAgo int) shared.ID {
	t.Helper()
	id := freshID(t)
	completedAt := now.AddDate(0, 0, -daysAgo)

	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO work_item (
			id, tenant_id, collection_id, type, path, depth, title, order_key,
			is_completed, completed_at, completed_by, created_by
		) VALUES ($1, $2, $3, 'TASK', $4, 1, $5, 'a0', true, $6, $7, $7)`,
		id.String(), s.tenant.String(), collectionID.String(), work.RootPath(id),
		"Entry "+id.String(), completedAt, s.author.String()); err != nil {
		t.Fatalf("seeding the completed entry: %v", err)
	}
	return id
}

// rules is the three rule use cases, wired the way the server wires them, with an authorizer that
// says yes: what is under test here is the engine, and who may configure it is proved a layer down.
func (s *suite) rules() lifecycle.Rules {
	return lifecycle.Rules{
		Rules:      postgres.NewRetentionRuleRepository(),
		Policies:   s.store,
		Marking:    postgres.NewRetentionMarkingRepository(),
		Holds:      s.store,
		Authorizer: allowAll{},
		Audit:      postgres.NewAuditSink(ids),
		UnitOfWork: s.uow,
		Clock:      clock.Fixed(now),
		IDs:        ids,
	}
}

type allowAll struct{}

func (allowAll) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return nil
}

// createRule writes one rule through the use case, which is the only way a rule is ever written.
func (s *suite) createRule(
	t *testing.T, cmd lifecycle.CreateRetentionPolicyCommand,
) (lifecycleDomain.Rule, lifecycle.Preview) {
	t.Helper()

	rule, preview, err := (lifecycle.CreateRetentionPolicy{Rules: s.rules()}).
		Execute(s.ctx, s.actor(), cmd)
	if err != nil {
		t.Fatalf("creating the rule: %v", err)
	}
	return rule, preview
}

// sweepAt runs one pass at an instant, which is how a chain is walked: the second stage falls due
// two periods after the first one acted.
func (s *suite) sweepAt(t *testing.T, at time.Time) lifecycle.Outcome {
	t.Helper()

	engine := s.engineAt(at)
	var outcome lifecycle.Outcome
	err := s.uow.Within(s.ctx, persistence.Scope{TenantID: s.tenant}, func(ctx context.Context) error {
		var err error
		outcome, err = engine.Execute(ctx, s.actor())
		return err
	})
	if err != nil {
		t.Fatalf("the retention run failed: %v", err)
	}
	return outcome
}

func (s *suite) marking(t *testing.T, id shared.ID) (time.Time, string) {
	t.Helper()

	var at *time.Time
	var action *string
	if err := s.admin.QueryRow(s.ctx,
		`SELECT retention_pending_until, retention_action FROM work_item WHERE id = $1`,
		id.String()).Scan(&at, &action); err != nil {
		t.Fatalf("reading the marking: %v", err)
	}
	if at == nil {
		return time.Time{}, ""
	}
	return *at, *action
}

func (s *suite) archivedAt(t *testing.T, id shared.ID) *time.Time {
	t.Helper()

	var at *time.Time
	if err := s.admin.QueryRow(s.ctx,
		`SELECT archived_at FROM work_item WHERE id = $1`, id.String()).Scan(&at); err != nil {
		t.Fatalf("reading the archive date: %v", err)
	}
	return at
}

// RE-3, the upper-bound half: exceeding `max_days` demands a justification, and the rule that
// carries one writes an audit entry saying so.
func TestRE3TheUpperBoundDemandsAJustification(t *testing.T) {
	s := newSuite(t, 24*time.Hour)

	// The bound is the operator's, and this is where an operator puts one (§4.4).
	const ceiling = 400
	if _, err := s.admin.Exec(s.ctx, `
		INSERT INTO retention_policy (tenant_id, data_kind, retain_days, min_days, max_days)
		VALUES ($1, 'TRASH', 30, 7, $2)
		ON CONFLICT (tenant_id, data_kind) DO UPDATE SET max_days = excluded.max_days`,
		s.tenant.String(), ceiling); err != nil {
		t.Fatalf("setting the upper bound: %v", err)
	}

	beyond := lifecycle.CreateRetentionPolicyCommand{
		Scope:    lifecycleDomain.Scope{Kind: lifecycleDomain.ScopeTenant},
		DataKind: lifecycleDomain.KindTrash, RetainDays: ceiling + 1,
		Action: lifecycleDomain.ActionHardDelete,
	}

	_, _, err := (lifecycle.CreateRetentionPolicy{Rules: s.rules()}).Execute(s.ctx, s.actor(), beyond)
	if err == nil {
		t.Fatal("a period past the upper bound was accepted with no justification")
	}

	beyond.Justification = "The works council agreed a longer period"
	rule, _ := s.createRule(t, beyond)
	if !rule.ExceedsCeiling(ceiling) {
		t.Fatal("the stored rule does not know it is past the bound")
	}

	// The justification is in the trail, which is what makes §4.4 evidence rather than a field.
	entries := s.count(t, `
		SELECT count(*) FROM audit_log
		WHERE tenant_id = $1 AND action = 'lifecycle.rule_changed'
		  AND changes::text LIKE '%works council%'`, s.tenant.String())
	if entries != 1 {
		t.Fatalf("%d audit entries carry the justification", entries)
	}
}

// RE-4: an object marked in phase one and taken out with `:retain` is not deleted in phase two.
func TestRE4AnObjectTakenOutIsNotDeleted(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	collectionID := s.collection(t)
	kept := s.completedItem(t, collectionID, 400)
	going := s.completedItem(t, collectionID, 400)
	// A workspace with work in it, so that the two entries are a fraction of what it holds rather
	// than all of it - a rule that would take everything is the one §5's switch turns into an
	// announcement, which is RE-7's subject rather than this one's.
	for range 60 {
		s.completedItem(t, collectionID, 1)
	}

	s.createRule(t, lifecycle.CreateRetentionPolicyCommand{
		Scope:    lifecycleDomain.Scope{Kind: lifecycleDomain.ScopeTenant},
		DataKind: lifecycleDomain.KindCompletedItem, RetainDays: 365,
		Action: lifecycleDomain.ActionHardDelete, GraceDays: graceOf(14),
	})

	// Phase one: both are announced, and neither has gone.
	s.sweep(t)
	for _, id := range []shared.ID{kept, going} {
		at, action := s.marking(t, id)
		if at.IsZero() || action != string(lifecycleDomain.ActionHardDelete) {
			t.Fatalf("%s was not announced: %s / %q", id, at, action)
		}
	}

	// One of them is taken out.
	if err := s.uow.Within(s.ctx, persistence.Scope{TenantID: s.tenant},
		func(ctx context.Context) error {
			_, err := postgres.NewRetentionMarkingRepository().
				Clear(ctx, []shared.ID{kept}, false, now)
			return err
		}); err != nil {
		t.Fatalf("retaining: %v", err)
	}

	// Phase two, once the grace period has run out.
	s.sweepAt(t, now.AddDate(0, 0, 15))

	if !s.exists(t, kept) {
		t.Error("an entry somebody took out of its period was deleted anyway")
	}
	if s.exists(t, going) {
		t.Error("the entry nobody took out survived phase two")
	}
}

// RE-7: the first activation of a broadly matching rule warns instead of deleting, and the share it
// reports is the share a preview reports.
func TestRE7ABroadRuleWarnsRatherThanDeletes(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	collectionID := s.collection(t)

	// Ten entries, nine of them past the period: ninety per cent, which is well past the twentieth
	// the switch is about.
	var going []shared.ID
	for range 9 {
		going = append(going, s.completedItem(t, collectionID, 400))
	}
	s.completedItem(t, collectionID, 10)

	rule, preview := s.createRule(t, lifecycle.CreateRetentionPolicyCommand{
		Scope:    lifecycleDomain.Scope{Kind: lifecycleDomain.ScopeTenant},
		DataKind: lifecycleDomain.KindCompletedItem, RetainDays: 365,
		Action: lifecycleDomain.ActionHardDelete,
	})

	if !preview.Broad {
		t.Fatalf("a rule taking %.0f per cent was not called broad", preview.ShareOfScope*100)
	}
	if rule.Action != lifecycleDomain.ActionNotifyOnly {
		t.Fatalf("the stored rule is a %s rather than an announcement", rule.Action)
	}

	// The share the activation reported is the share a preview reports, which is what makes the
	// notice checkable.
	again, err := (lifecycle.PreviewRetentionPolicy{Rules: s.rules()}).Execute(s.ctx, s.actor(), rule.ID)
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	if again.Matched != preview.Matched || again.ShareOfScope != preview.ShareOfScope {
		t.Fatalf("the preview says %d / %v and the activation said %d / %v",
			again.Matched, again.ShareOfScope, preview.Matched, preview.ShareOfScope)
	}

	// And two passes later nothing has gone: an announcement acts on nothing.
	s.sweep(t)
	s.sweepAt(t, now.AddDate(0, 0, 30))
	for _, id := range going {
		if !s.exists(t, id) {
			t.Fatalf("%s was deleted by a rule that was supposed to announce", id)
		}
	}
}

// RE-9: a chained rule passes through completed → archive → deletion, and the anchor of each stage
// is the right column - the second period counts from the archiving rather than from the
// completion.
func TestRE9AChainCountsEachStageFromItsOwnColumn(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	collectionID := s.collection(t)
	id := s.completedItem(t, collectionID, 400)
	for range 60 {
		s.completedItem(t, collectionID, 1)
	}

	s.createRule(t, lifecycle.CreateRetentionPolicyCommand{
		Scope:    lifecycleDomain.Scope{Kind: lifecycleDomain.ScopeTenant},
		DataKind: lifecycleDomain.KindCompletedItem, RetainDays: 365,
		Action: lifecycleDomain.ActionArchive, GraceDays: graceOf(0),
		ThenAfterDays: 730, ThenAction: lifecycleDomain.ActionHardDelete,
	})

	// The first stage: announced and acted on in one pass, because the grace period is zero.
	s.sweep(t)
	archived := s.archivedAt(t, id)
	if archived == nil {
		t.Fatal("the first stage did not archive the entry")
	}
	if !archived.Equal(now) {
		t.Fatalf("the entry was archived at %s rather than at the moment of the run", archived)
	}

	// A year later - past the first period twice over and short of the second - the entry is still
	// there. If the second stage counted from the completion it would have gone by now.
	s.sweepAt(t, now.AddDate(0, 0, 400))
	if !s.exists(t, id) {
		t.Fatal("the second stage counted from the completion rather than from the archiving")
	}

	// And two more years after the archiving, it goes.
	s.sweepAt(t, now.AddDate(0, 0, 731))
	if s.exists(t, id) {
		t.Fatal("the second stage never fired")
	}
}

func graceOf(days int) *int { return &days }

// ── QS-23 (E-08) ──────────────────────────────────────────────────────────────────────────────

// holds is the three legal hold use cases, wired the way the server wires them.
func (s *suite) holds() lifecycle.Holds {
	return lifecycle.Holds{
		Holds:      postgres.NewLegalHoldRepository(),
		Authorizer: allowAll{},
		Audit:      postgres.NewAuditSink(ids),
		UnitOfWork: s.uow,
		Clock:      clock.Fixed(now),
		IDs:        ids,
	}
}

func (s *suite) actorWithAccount() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: s.tenant, AccountID: s.author,
		AccountName: "Anna", Locale: "en", TimeZone: "UTC",
	}
}

func (s *suite) itemRetention(t *testing.T, id shared.ID) (string, string) {
	t.Helper()

	var action, blockedBy *string
	if err := s.admin.QueryRow(s.ctx,
		`SELECT retention_action, retention_blocked_by FROM work_item WHERE id = $1`,
		id.String()).Scan(&action, &blockedBy); err != nil {
		t.Fatalf("reading the entry's retention: %v", err)
	}
	return textOr(action), textOr(blockedBy)
}

func textOr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// QS-23, end to end rather than by construction: a hold placed through the use case stops a
// retention run and a manual purge, and shows up in the run's blocked reasons and on the object.
func TestQS23AHoldPlacedThroughTheAPIStopsEveryDeletion(t *testing.T) {
	s := newSuite(t, 24*time.Hour)
	collectionID := s.collection(t)
	held := s.completedItem(t, collectionID, 400)
	trashed := s.trashedItem(t, collectionID, 400)
	for range 60 {
		s.completedItem(t, collectionID, 1)
	}

	s.createRule(t, lifecycle.CreateRetentionPolicyCommand{
		Scope:    lifecycleDomain.Scope{Kind: lifecycleDomain.ScopeTenant},
		DataKind: lifecycleDomain.KindCompletedItem, RetainDays: 365,
		Action: lifecycleDomain.ActionHardDelete, GraceDays: graceOf(0),
	})

	// The hold, placed the way a person places one.
	hold, err := (lifecycle.PlaceLegalHold{Holds: s.holds()}).Execute(s.ctx, s.actorWithAccount(),
		lifecycle.PlaceLegalHoldCommand{
			Scope: lifecycleDomain.HoldContainer, ScopeID: collectionID,
			Reason: "Pending litigation, ref. 4 O 128/26",
		})
	if err != nil {
		t.Fatalf("placing the hold: %v", err)
	}

	outcome := s.sweep(t)

	// Nothing went - not the completed entry the rule caught, and not the trashed one the trash's
	// own period caught.
	if !s.exists(t, held) {
		t.Error("a completed entry under a legal hold was deleted")
	}
	if !s.exists(t, trashed) {
		t.Error("a trashed entry under a legal hold was deleted")
	}
	if outcome.Blocked[lifecycleDomain.BlockedByLegalHold] == 0 {
		t.Errorf("the run reported %+v", outcome.Blocked)
	}

	// The run's own record says so, which is what an operator reads.
	blocked := s.count(t, `
		SELECT count(*) FROM retention_run
		WHERE tenant_id = $1 AND blocked_reasons ? 'legal_hold'`, s.tenant.String())
	if blocked == 0 {
		t.Error("no retention run recorded a legal hold among its blocked reasons")
	}

	// And the object says so, which is what the person looking at it reads.
	action, reason := s.itemRetention(t, held)
	if reason != lifecycleDomain.BlockedByLegalHold {
		t.Errorf("the entry says %q is stopping it", reason)
	}
	if action != string(lifecycleDomain.ActionHardDelete) {
		t.Errorf("the entry says the rule would %q", action)
	}

	// A manual purge is stopped too: a hold outranks a person emptying their own trash.
	err = s.uow.Within(s.ctx, persistence.Scope{TenantID: s.tenant}, func(ctx context.Context) error {
		_, err := s.engineAt(now).Purger.Sweep(ctx, s.actor(), lifecycle.Selection{
			Cutoff: now, Reason: lifecycleDomain.DeletedByUser,
		}, now)
		return err
	})
	if err != nil {
		t.Fatalf("emptying the trash: %v", err)
	}
	if !s.exists(t, trashed) {
		t.Fatal("a manual purge removed an entry under a legal hold")
	}

	// Lifting it lets the next pass through, and records who and why.
	if _, err := (lifecycle.ReleaseLegalHold{Holds: s.holds()}).
		Execute(s.ctx, s.actorWithAccount(), hold.ID, "The proceedings ended"); err != nil {
		t.Fatalf("lifting the hold: %v", err)
	}

	lifted := s.count(t, `
		SELECT count(*) FROM legal_hold
		WHERE id = $1 AND released_by = $2 AND released_at IS NOT NULL
		  AND released_reason = 'The proceedings ended'`, hold.ID.String(), s.author.String())
	if lifted != 1 {
		t.Fatal("the lifting did not record who, when and why")
	}
	entries := s.count(t, `
		SELECT count(*) FROM audit_log
		WHERE tenant_id = $1 AND action = 'lifecycle.hold_released'`, s.tenant.String())
	if entries != 1 {
		t.Fatalf("%d audit entries for the lifting", entries)
	}

	// Two passes, because the rule announces before it acts and the grace period is zero.
	s.sweepAt(t, now.Add(time.Minute))
	if s.exists(t, held) {
		t.Error("the entry survived after the hold was lifted")
	}
}
