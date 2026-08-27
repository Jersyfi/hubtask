// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	integrationservice "github.com/Jersyfi/hubtask/core/application/service/integration"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/eventbus"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The polling trigger against a real database (G-04). Three things only PostgreSQL can answer:
// that the keyset pages a table nobody is holding still, that the horizon covers a writer whose
// transaction is open while a poll runs, and that a poll cannot reach into another tenant.

// pollLagUnderTest is generous on purpose. It has to outlast the writers this file deliberately
// keeps open, and the test waits it out once rather than racing it.
const pollLagUnderTest = 3 * time.Second

// triggerSecret is fixed, so that a cursor printed by a failing test is the same value on a rerun.
func triggerCursors() security.TriggerCursorCodec {
	return security.NewTriggerCursorCodec(secret.New("integration test installation secret"))
}

// pollingService is the real use case over the real adapters. Assembled per call rather than
// shared, because two of the tests want a different lag - and because a poller in production is
// exactly this: a fresh handler over a pool it did not open.
func pollingService(ctx context.Context, t *testing.T, lag time.Duration) integrationservice.PollTriggerEvents {
	t.Helper()
	return pollingServiceOn(postgres.NewUnitOfWork(appPool(ctx, t)), t, lag)
}

// pollingServiceOn is the same handler over a unit of work the caller already has.
//
// dbtest.AppPool builds a new pgxpool every time it is asked, and PostgreSQL has a finite number of
// connection slots. A test that runs writers and a poller at once has to put them on one pool, or
// it fails on the slots rather than on what it is testing.
func pollingServiceOn(
	uow persistence.UnitOfWork, t *testing.T, lag time.Duration,
) integrationservice.PollTriggerEvents {
	t.Helper()

	return integrationservice.PollTriggerEvents{
		Events:     postgres.NewOutbox(jobQueue(t)),
		Policies:   postgres.NewLifecycleRepository(),
		Cursors:    triggerCursors(),
		Rendering:  cloudEvents{},
		UnitOfWork: uow,
		Clock:      clockadapter.System{},
		Lag:        lag,
	}
}

// cloudEvents is the same mapping the webhook deliverer sends, which is the point of the port.
type cloudEvents struct{}

func (cloudEvents) Render(envelope event.Envelope) map[string]any {
	return eventbus.ToCloudEvent(envelope, "https://hubtask.test")
}

func poller(tenant shared.ID, scopes ...string) appshared.ActorContext {
	return appshared.ActorContext{
		TenantID: tenant, AccountID: authorA, AccountName: "Anna",
		Kind: shared.ActorUser, Scopes: scopes,
	}
}

// pollingScopes is what an ordinary poller of container events holds: the endpoint's and the
// event's.
func pollingScopes() []string {
	return []string{"automation:manage", event.ContainerCreated.ReadScope()}
}

// appendAt writes one event stamped at a chosen moment, in its own transaction.
//
// The unit of work is the caller's, never a fresh one per call: dbtest.AppPool builds a new pool
// every time it is asked, and a loop that asked it a hundred times would exhaust PostgreSQL's
// connection slots rather than test anything.
func appendAt(
	ctx context.Context, t *testing.T, uow persistence.UnitOfWork, tenant shared.ID,
	id shared.ID, at time.Time, hold time.Duration,
) error {
	t.Helper()

	container := containerIn(tenant, authorA, freshID(t), freshName(t), "a0")
	envelope, err := event.NewContainerCreated(id, container,
		event.Actor{Kind: shared.ActorUser, ID: authorA}, at, event.Cause{})
	if err != nil {
		return err
	}

	// An error rather than a t.Fatalf, because this is called from the writers' goroutines and
	// failing a test from one is a panic rather than a failure.
	return uow.Within(ctx, persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
		if err := postgres.NewOutbox(jobQueue(t)).Append(ctx, envelope); err != nil {
			return err
		}
		// The transaction stays open for a while after the row exists but before anybody can see
		// it. That gap is the whole hazard the horizon covers.
		if hold > 0 {
			select {
			case <-time.After(hold):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
}

// appended is appendAt for the tests that write one event and want it to have worked.
func appended(
	ctx context.Context, t *testing.T, uow persistence.UnitOfWork, tenant shared.ID, at time.Time,
) shared.ID {
	t.Helper()

	id := freshID(t)
	if err := appendAt(ctx, t, uow, tenant, id, at, 0); err != nil {
		t.Fatalf("appending the event: %v", err)
	}
	return id
}

// walk polls until the log is exhausted and returns the identifiers in the order they came, with
// the cursor it finished on. Bounded, because a walk that could not end would hang the suite
// rather than fail it.
func walk(
	ctx context.Context, t *testing.T, handler integrationservice.PollTriggerEvents,
	actor appshared.ActorContext, cursor string, limit int,
) ([]string, string) {
	t.Helper()

	var seen []string
	for round := range 50 {
		page, err := handler.Execute(ctx, actor, integrationservice.PollTriggerEventsCommand{
			EventType: string(event.ContainerCreated), Cursor: cursor, Limit: limit,
		})
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		for _, rendered := range page.Events {
			id, _ := rendered["id"].(string)
			seen = append(seen, id)
		}
		cursor = triggerCursors().Encode(page.Cursor)
		if !page.More {
			return seen, cursor
		}
	}
	t.Fatal("the walk did not end in fifty rounds")
	return nil, ""
}

// The acceptance criterion, and the reason the horizon exists.
//
// Twelve writers, each stamping its event at the moment it starts and each holding its transaction
// open for a decreasing while afterwards - so the last writer to stamp is among the first to
// commit, and a row appears *behind* a row that was already committed. That is the interleaving
// that would make a naive cursor step over an event and never know.
//
// While they run, a poller polls. It must see nothing: every row is younger than the lag. Once the
// lag has passed, the same cursor must yield all twelve, once each, in the order they were stamped.
func TestInterleavedWritersNeitherRepeatNorSkip(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	// Six rather than a dozen: every writer holds a connection for the length of its hold, and the
	// poller is on the same pool. What the test needs is that commit order and stamp order differ,
	// and six writers with shrinking holds give that as surely as twenty would.
	const writers = 6

	// One pool for the writers and the poller together, for the reason pollingServiceOn gives.
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	handler := pollingServiceOn(uow, t, pollLagUnderTest)
	actor := poller(tenantA, pollingScopes()...)

	// Where the walk stands before anything is written, so that the events of other tests in this
	// tenant are behind the cursor rather than in the answer.
	cursor, _ := atEndOfStream(ctx, t, handler, actor)

	stamped := make([]time.Time, writers)
	written := make([]shared.ID, writers)
	failures := make([]error, writers)
	for i := range writers {
		written[i] = freshID(t)
	}

	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		concurrency.Go(ctx, "test-writer", func(ctx context.Context) {
			defer wg.Done()
			// Staggered stamps, and a hold that shrinks as the stamp grows: the writer that stamps
			// last is the first to commit, so a row appears behind one already committed.
			time.Sleep(time.Duration(i) * 40 * time.Millisecond)
			stamped[i] = time.Now().UTC()
			failures[i] = appendAt(ctx, t, uow, tenantA, written[i], stamped[i],
				time.Duration(writers-i)*80*time.Millisecond)
		})
	}

	// Polling while they are open, with the cursor each round returns. It must not run ahead of
	// what is settled - the horizon is a lag behind the present and every one of these rows is
	// younger than that - and whatever it answers has to be answered once.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		page, err := handler.Execute(ctx, actor, integrationservice.PollTriggerEventsCommand{
			EventType: string(event.ContainerCreated), Cursor: cursor, Limit: 200,
		})
		if err != nil {
			t.Fatalf("polling during the writes: %v", err)
		}
		if len(page.Events) != 0 {
			t.Fatalf("%d events were answered while their writers were still open", len(page.Events))
		}
		cursor = triggerCursors().Encode(page.Cursor)
		time.Sleep(20 * time.Millisecond)
	}
	wg.Wait()
	for i, err := range failures {
		if err != nil {
			t.Fatalf("writer %d: %v", i, err)
		}
	}

	// Waiting the lag out once, rather than racing it.
	time.Sleep(pollLagUnderTest)

	seen, _ := walk(ctx, t, handler, actor, cursor, 5)

	if len(seen) != writers {
		t.Fatalf("saw %d events, want %d: %v", len(seen), writers, seen)
	}
	counted := map[string]int{}
	for _, id := range seen {
		counted[id]++
	}
	for i, id := range written {
		switch counted[id.String()] {
		case 1:
		case 0:
			t.Errorf("the event of writer %d (%s, stamped %v) was stepped over", i, id, stamped[i])
		default:
			t.Errorf("the event of writer %d (%s) was answered %d times", i, id, counted[id.String()])
		}
	}

	// Ascending, whatever order they committed in. A poller that deduplicates on `id` still needs
	// the order to be the log's rather than the scheduler's.
	for i := 1; i < len(seen); i++ {
		if seen[i-1] >= seen[i] && stampOf(t, stamped, written, seen[i-1]).
			After(stampOf(t, stamped, written, seen[i])) {
			t.Errorf("position %d came out of order", i)
		}
	}
}

// What the horizon buys, constructed rather than raced.
//
// The hazard in one sentence: `occurred_at` is stamped by the writing transaction and the row
// becomes visible at its commit, so a row can appear *behind* a row that has already been answered.
// A cursor sitting on the answered row then steps over it, silently and forever.
//
// The two halves here are the same three writes with the horizon in two places. With the horizon at
// the present - no lag at all - the poll between the writes advances past the late arrival and the
// event is lost. With the horizon behind both rows, the poll between them answers nothing, advances
// only as far as is settled, and the later poll finds both in order.
//
// The horizon moves by changing the lag rather than by sleeping: it is a moment, and what the test
// needs is that moment on either side of the rows.
func TestTheHorizonCoversAWriteThatLandsBehindTheCursor(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	actor := poller(tenantA, pollingScopes()...)

	t.Run("without a horizon the late arrival is stepped over", func(t *testing.T) {
		handler := pollingServiceOn(uow, t, 0)
		cursor, at := atEndOfStream(ctx, t, handler, actor)

		// Both stamps sit just after where the walk stands and just before the present, so that a
		// horizon at the present covers neither of them. The one that lands second carries the
		// earlier stamp: that is the interleaving, stated rather than raced for.
		behindAt := at.OccurredAt.Add(time.Millisecond)
		aheadAt := at.OccurredAt.Add(2 * time.Millisecond)

		ahead := appended(ctx, t, uow, tenantA, aheadAt)

		page, err := handler.Execute(ctx, actor, integrationservice.PollTriggerEventsCommand{
			EventType: string(event.ContainerCreated), Cursor: cursor, Limit: 200,
		})
		if err != nil {
			t.Fatalf("the poll between the writes: %v", err)
		}
		if len(page.Events) != 1 || page.Events[0]["id"] != ahead.String() {
			t.Fatalf("the poll between the writes answered %v, want %s", page.Events, ahead)
		}

		// The transaction that began earlier commits now, with the earlier stamp.
		behind := appended(ctx, t, uow, tenantA, behindAt)

		seen, _ := walk(ctx, t, handler, actor, triggerCursors().Encode(page.Cursor), 200)
		for _, id := range seen {
			if id == behind.String() {
				t.Fatal("the late arrival was answered, so this half proves nothing about the horizon")
			}
		}
	})

	t.Run("with a horizon behind both, neither is lost", func(t *testing.T) {
		// Far enough back that neither row is settled yet, so the poll between the writes cannot
		// reach either.
		withheld := pollingServiceOn(uow, t, 10*time.Minute)
		cursor, at := atEndOfStream(ctx, t, withheld, actor)

		// After where the walk stands, and still well inside the ten minutes the horizon withholds.
		behindAt := at.OccurredAt.Add(time.Minute)
		aheadAt := at.OccurredAt.Add(2 * time.Minute)

		ahead := appended(ctx, t, uow, tenantA, aheadAt)

		page, err := withheld.Execute(ctx, actor, integrationservice.PollTriggerEventsCommand{
			EventType: string(event.ContainerCreated), Cursor: cursor, Limit: 200,
		})
		if err != nil {
			t.Fatalf("the poll between the writes: %v", err)
		}
		if len(page.Events) != 0 {
			t.Fatalf("a row younger than the horizon was answered: %v", page.Events)
		}

		behind := appended(ctx, t, uow, tenantA, behindAt)

		// The horizon has moved past both rows - which is what waiting would have done, without
		// the waiting. The same cursor now finds them, oldest first.
		settled := pollingServiceOn(uow, t, time.Minute)
		seen, _ := walk(ctx, t, settled, actor, triggerCursors().Encode(page.Cursor), 200)

		var order []string
		for _, id := range seen {
			if id == behind.String() || id == ahead.String() {
				order = append(order, id)
			}
		}
		if len(order) != 2 {
			t.Fatalf("saw %v, want both rows exactly once", order)
		}
		if order[0] != behind.String() || order[1] != ahead.String() {
			t.Errorf("saw %v, want the earlier stamp first", order)
		}
	})
}

// atEndOfStream walks a tenant's stream to its end and returns the cursor there, so that a test starts
// behind whatever other tests in this tenant have written.
func atEndOfStream(
	ctx context.Context, t *testing.T, handler integrationservice.PollTriggerEvents,
	actor appshared.ActorContext,
) (string, outbox.Position) {
	t.Helper()

	_, cursor := walk(ctx, t, handler, actor, "", 200)
	at, err := triggerCursors().Decode(cursor)
	if err != nil {
		t.Fatalf("the cursor at the end of the stream does not decode: %v", err)
	}
	return cursor, at
}

func stampOf(t *testing.T, stamped []time.Time, written []shared.ID, id string) time.Time {
	t.Helper()
	for i, candidate := range written {
		if candidate.String() == id {
			return stamped[i]
		}
	}
	return time.Time{}
}

// The cursor names a position in the table, not one a process was holding, so a codec built fresh
// from the same installation secret - which is what the next process is - resumes exactly where the
// last one stopped.
func TestACursorSurvivesTheProcessThatIssuedIt(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	handler := pollingService(ctx, t, 0)
	actor := poller(tenantA, pollingScopes()...)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	at := time.Now().UTC().Add(-time.Hour)
	first := appended(ctx, t, uow, tenantA, at)
	second := appended(ctx, t, uow, tenantA, at.Add(time.Minute))

	page, err := handler.Execute(ctx, actor, integrationservice.PollTriggerEventsCommand{
		EventType: string(event.ContainerCreated),
		Cursor: triggerCursors().Encode(outbox.Position{
			OccurredAt: at.Add(-time.Second), ID: shared.ID(""),
		}),
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("the first poll: %v", err)
	}
	if len(page.Events) != 1 || page.Events[0]["id"] != first.String() {
		t.Fatalf("the first poll answered %v, want %s", page.Events, first)
	}

	// A different codec instance and a different handler: the process restarted, and the only
	// thing carried across is the string the caller held.
	carried := triggerCursors().Encode(page.Cursor)
	resumed := pollingService(ctx, t, 0)

	next, err := resumed.Execute(ctx, actor, integrationservice.PollTriggerEventsCommand{
		EventType: string(event.ContainerCreated), Cursor: carried, Limit: 1,
	})
	if err != nil {
		t.Fatalf("the resumed poll: %v", err)
	}
	if len(next.Events) != 1 || next.Events[0]["id"] != second.String() {
		t.Fatalf("the resumed poll answered %v, want %s", next.Events, second)
	}
}

// The far end of the window, against the tenant's real policy row.
func TestACursorOlderThanTheStoredRetentionIsRefused(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	writeOutboxPolicy(ctx, t, tenantA, 7)

	handler := pollingService(ctx, t, 0)

	stale := triggerCursors().Encode(outbox.Position{
		OccurredAt: time.Now().UTC().AddDate(0, 0, -8), ID: freshID(t),
	})
	_, err := handler.Execute(ctx, poller(tenantA, pollingScopes()...),
		integrationservice.PollTriggerEventsCommand{
			EventType: string(event.ContainerCreated), Cursor: stale,
		})
	if !errors.Is(err, shared.ErrGone) {
		t.Fatalf("error %v, want ErrGone", err)
	}
}

// A tenant that keeps events longer keeps them pollable longer. The window is the tenant's, not
// the code's (ADR-0020).
func TestTheWindowIsTheTenantsOwn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	writeOutboxPolicy(ctx, t, tenantB, 30)

	handler := pollingService(ctx, t, 0)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	at := time.Now().UTC().AddDate(0, 0, -20)
	old := appended(ctx, t, uow, tenantB, at)

	seen, _ := walk(ctx, t, handler, poller(tenantB, pollingScopes()...), "", 200)

	var found bool
	for _, id := range seen {
		if id == old.String() {
			found = true
		}
	}
	if !found {
		t.Errorf("a twenty-day-old event is outside a thirty-day window: %v", seen)
	}
}

// The cross-tenant negative test for Poll. Row level security narrows the table to the
// transaction's tenant, so tenant B's poller cannot be fed tenant A's stream whatever it asks for.
func TestAPollCannotReachAnotherTenantsEvents(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	mine := appended(ctx, t, uow, tenantA, time.Now().UTC().Add(-time.Hour))

	var answered []event.Envelope
	err := postgres.NewUnitOfWork(appPool(ctx, t)).WithinReadOnly(ctx,
		persistence.Scope{TenantID: tenantB}, func(ctx context.Context) error {
			var pollErr error
			answered, pollErr = postgres.NewOutbox(jobQueue(t)).Poll(ctx,
				event.ContainerCreated, outbox.Position{}, time.Now().UTC(), 200)
			return pollErr
		})
	if err != nil {
		t.Fatalf("polling as tenant B: %v", err)
	}

	for _, envelope := range answered {
		if envelope.ID == mine {
			t.Fatal("tenant B was answered an event of tenant A")
		}
		if envelope.TenantID != tenantB {
			t.Errorf("an event of tenant %s reached tenant B", envelope.TenantID)
		}
	}
}

// A restore's events go to nobody outward-facing, on the pull half as on the push half
// (migration 0033). The index is partial on the same predicate, so this also proves the two agree.
func TestAReplayedEventIsNotAnswered(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	at := time.Now().UTC().Add(-time.Hour)
	ordinary := appended(ctx, t, uow, tenantA, at)

	replayed := freshID(t)
	container := containerIn(tenantA, authorA, freshID(t), freshName(t), "a0")
	envelope, err := event.NewContainerCreated(replayed, container,
		event.Actor{Kind: shared.ActorUser, ID: authorA}, at.Add(time.Second), event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	envelope.Replay = true
	if err := uow.Within(ctx, persistence.Scope{TenantID: tenantA},
		func(ctx context.Context) error {
			return postgres.NewOutbox(jobQueue(t)).Append(ctx, envelope)
		}); err != nil {
		t.Fatalf("appending the replayed event: %v", err)
	}

	seen, _ := walk(ctx, t, pollingService(ctx, t, 0), poller(tenantA, pollingScopes()...), "", 200)

	var sawOrdinary bool
	for _, id := range seen {
		if id == replayed.String() {
			t.Error("a replayed event was answered")
		}
		if id == ordinary.String() {
			sawOrdinary = true
		}
	}
	if !sawOrdinary {
		t.Error("the ordinary event beside it was not answered either - the test proves nothing")
	}
}

// The acceptance criterion for the scope, end to end: the wrong one is refused before the database
// is touched at all.
func TestPollingWithTheWrongScopeIsRefused(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	handler := pollingService(ctx, t, 0)

	_, err := handler.Execute(ctx, poller(tenantA, "automation:manage", "items:read"),
		integrationservice.PollTriggerEventsCommand{EventType: string(event.ContainerCreated)})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want ErrForbidden", err)
	}
}

// writeOutboxPolicy gives a tenant a period of its own, the way a retention run would.
func writeOutboxPolicy(ctx context.Context, t *testing.T, tenant shared.ID, days int) {
	t.Helper()

	err := postgres.NewUnitOfWork(appPool(ctx, t)).Within(ctx,
		persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
			return postgres.NewLifecycleRepository().Ensure(ctx, []lifecycle.Policy{
				{DataKind: lifecycle.KindOutboxEvent, RetainDays: days},
			})
		})
	if err != nil {
		t.Fatalf("writing the retention policy: %v", err)
	}

	var stored lifecycle.Policy
	if err := postgres.NewUnitOfWork(appPool(ctx, t)).WithinReadOnly(ctx,
		persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
			var findErr error
			stored, findErr = postgres.NewLifecycleRepository().Find(ctx, lifecycle.KindOutboxEvent)
			return findErr
		}); err != nil {
		t.Fatalf("reading the retention policy back: %v", err)
	}
	if stored.RetainDays != days {
		t.Fatalf("the tenant keeps events for %d days, want %d - Ensure left an older row alone",
			stored.RetainDays, days)
	}
}
