// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	eventbusport "github.com/Jersyfi/hubtask/core/port/eventbus"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/eventbus"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func announcement(t *testing.T, tenant shared.ID, container work.Container) event.Envelope {
	t.Helper()
	envelope, err := event.NewContainerCreated(freshID(t), container,
		event.Actor{Kind: shared.ActorUser, ID: authorA}, created, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	return envelope
}

func outboxRows(ctx context.Context, t *testing.T, tenant shared.ID) map[string]string {
	t.Helper()
	admin := adminPool(ctx, t)

	rows, err := admin.Query(ctx,
		`SELECT id::text, event_type, subject, payload::text, dispatched_at IS NULL
		 FROM outbox_event WHERE tenant_id = $1`, tenant.String())
	if err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}
	defer rows.Close()

	found := map[string]string{}
	for rows.Next() {
		var id, eventType, subject, payload string
		var pending bool
		if err := rows.Scan(&id, &eventType, &subject, &payload, &pending); err != nil {
			t.Fatalf("scanning the outbox: %v", err)
		}
		if !pending {
			t.Errorf("event %s was written as already dispatched", id)
		}
		found[id] = eventType + " " + subject + " " + payload
	}
	return found
}

// An event is written with its payload, its subject and its causal chain, and it is pending -
// nothing here delivers it, which is the whole point of an outbox (ADR-0007).
func TestAnEventIsWrittenIntoTheOutbox(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	name := freshName(t)

	container := containerIn(tenantA, authorA, freshID(t), name, "a0")
	envelope := announcement(t, tenantA, container)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewOutbox(jobQueue(t)).Append(ctx, envelope)
	}); err != nil {
		t.Fatalf("writing the event: %v", err)
	}

	row, found := outboxRows(ctx, t, tenantA)[envelope.ID.String()]
	if !found {
		t.Fatal("the event is not in the outbox")
	}
	if !strings.Contains(row, string(event.ContainerCreated)) {
		t.Errorf("the event type did not survive: %s", row)
	}
	if !strings.Contains(row, "container/"+container.ID.String()) {
		t.Errorf("the subject did not survive: %s", row)
	}
	if !strings.Contains(row, name) {
		t.Errorf("the snapshot did not survive: %s", row)
	}
}

// The cross-tenant negative test for Append: the tenant comes from the transaction, so an event
// cannot be written into another tenant's stream - a subscriber of tenant A must not be able to
// be fed by tenant B.
func TestAnEventCannotBeWrittenIntoAnotherTenantsStream(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	// The container claims tenant A; the transaction belongs to tenant B.
	envelope := announcement(t, tenantA, containerIn(tenantA, authorA, freshID(t), freshName(t), "a0"))

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return postgres.NewOutbox(jobQueue(t)).Append(ctx, envelope)
	}); err != nil {
		t.Fatalf("writing the event: %v", err)
	}

	if _, found := outboxRows(ctx, t, tenantA)[envelope.ID.String()]; found {
		t.Error("the event landed in tenant A although the transaction was tenant B's")
	}
	if _, found := outboxRows(ctx, t, tenantB)[envelope.ID.String()]; !found {
		t.Error("the event is not in the tenant that wrote it")
	}
}

// The change log is the other half of a write: the same snapshot, a different recipient
// (offline-sync.md §10).
func TestAChangeIsRecordedWithItsClock(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	container := containerIn(tenantA, authorA, freshID(t), freshName(t), "a0")
	reading, err := shared.HLC{}.Tick(created, "server-1")
	if err != nil {
		t.Fatalf("stamping: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewChangeLog().Record(ctx, changelog.Change{
			TenantID: tenantA, Entity: "container", EntityID: container.ID,
			Op: changelog.Upsert, ContainerID: container.ID, ActorID: authorA,
			HLC: reading, Payload: map[string]any{"name": container.Name},
		})
	}); err != nil {
		t.Fatalf("recording the change: %v", err)
	}

	admin := adminPool(ctx, t)
	var seq int64
	var hlc, op string
	err = admin.QueryRow(ctx, `
		SELECT seq, hlc, op FROM change_log
		WHERE tenant_id = $1 AND entity_id = $2`, tenantA.String(), container.ID.String()).
		Scan(&seq, &hlc, &op)
	if err != nil {
		t.Fatalf("reading the change log: %v", err)
	}

	if seq <= 0 {
		t.Errorf("the cursor value is %d - a client cannot page on it", seq)
	}
	if hlc != reading.String() {
		t.Errorf("the clock reading is %q, want %q", hlc, reading.String())
	}
	if op != string(changelog.Upsert) {
		t.Errorf("operation %q", op)
	}
}

// The cross-tenant negative test for Record: a change is written into the tenant of the
// transaction, so `:pull` in another tenant can never see it (test SY-11).
func TestAChangeCannotBeRecordedForAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	entityID := freshID(t)
	reading, err := shared.HLC{}.Tick(created, "server-1")
	if err != nil {
		t.Fatalf("stamping: %v", err)
	}

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return postgres.NewChangeLog().Record(ctx, changelog.Change{
			TenantID: tenantA, Entity: "container", EntityID: entityID,
			Op: changelog.Upsert, HLC: reading,
		})
	}); err != nil {
		t.Fatalf("recording the change: %v", err)
	}

	admin := adminPool(ctx, t)
	var inA, inB int
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM change_log WHERE tenant_id = $1 AND entity_id = $2`,
		tenantA.String(), entityID.String()).Scan(&inA); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM change_log WHERE tenant_id = $1 AND entity_id = $2`,
		tenantB.String(), entityID.String()).Scan(&inB); err != nil {
		t.Fatalf("counting: %v", err)
	}

	if inA != 0 {
		t.Error("the change landed in tenant A although the transaction was tenant B's")
	}
	if inB != 1 {
		t.Errorf("%d changes in the tenant that wrote it, want 1", inB)
	}
}

// A change without a clock reading cannot be merged against a concurrent edit, so it is refused
// rather than written as unmergeable.
func TestAChangeWithoutAClockIsRefused(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewChangeLog().Record(ctx, changelog.Change{
			TenantID: tenantA, Entity: "container", EntityID: freshID(t), Op: changelog.Upsert,
		})
	})
	if err == nil || shared.AsError(err).DetailCode != "sync.change_without_clock" {
		t.Fatalf("error %v, want the change to be refused", err)
	}
}

// dispatchOnce runs one dispatcher round for the tenant, the way the worker does: inside one
// transaction, opened for the tenant the job names.
func dispatchOnce(ctx context.Context, t *testing.T, tenant shared.ID, subscribers ...eventbusport.Subscriber) {
	t.Helper()
	dispatcher := eventbus.Dispatcher{
		Events:      postgres.NewOutbox(jobQueue(t)),
		Consumed:    postgres.NewConsumption(clockadapter.System{}),
		Subscribers: subscribers,
		Clock:       clockadapter.System{},
		Batch:       50,
		MinInterval: time.Second,
		MaxInterval: 15 * time.Second,
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		_, err := dispatcher.Run(ctx, queue.Job{
			ID: freshID(t), TenantID: tenant, Kind: queue.KindOutboxDispatch,
			Attempts: 1, MaxAttempts: 8, Lease: time.Now().Add(time.Minute),
		})
		return err
	}); err != nil {
		t.Fatalf("dispatching for %s: %v", tenant, err)
	}
}

// counter is a subscriber that reacts to one event and counts how often it did. Anything else in
// the tenant's outbox is somebody else's test and is not this one's business.
type counter struct {
	watching  shared.ID
	reactions int
}

func (c *counter) Name() string          { return "test.counter" }
func (c *counter) Wants(event.Type) bool { return true }

func (c *counter) Deliver(_ context.Context, envelope event.Envelope) error {
	if envelope.ID == c.watching {
		c.reactions++
	}
	return nil
}

// Test RT-4: an event delivered twice has no duplicate effect.
//
// The duplicate is produced the way a real one arises - the delivery was recorded and then lost,
// here by putting the row back to undelivered - and the subscriber reacts once, because the claim
// on the consumption record is what a consumer asks before it acts (ADR-0007).
func TestRT4ADuplicateDeliveryHasNoDuplicateEffect(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	container := containerIn(tenantA, authorA, freshID(t), freshName(t), "a0")
	envelope := announcement(t, tenantA, container)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewOutbox(jobQueue(t)).Append(ctx, envelope)
	}); err != nil {
		t.Fatalf("writing the event: %v", err)
	}

	subscriber := &counter{watching: envelope.ID}
	dispatchOnce(ctx, t, tenantA, subscriber)
	if subscriber.reactions != 1 {
		t.Fatalf("the subscriber reacted %d times to the first delivery, want once", subscriber.reactions)
	}

	// The delivery is lost after the fact: the dispatcher died between handing the event over and
	// committing that it had. At-least-once means this event comes round again.
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`UPDATE outbox_event SET dispatched_at = NULL WHERE id = $1`, envelope.ID.String()); err != nil {
		t.Fatalf("undoing the delivery: %v", err)
	}

	dispatchOnce(ctx, t, tenantA, subscriber)
	if subscriber.reactions != 1 {
		t.Errorf("the subscriber reacted %d times in total, want once - a duplicate delivery had a duplicate effect", subscriber.reactions)
	}

	// And the second round still marks it: the outbox is not where the duplicate is corrected, so
	// an event nobody needs to react to again is still delivered rather than kept forever.
	var pending bool
	if err := admin.QueryRow(ctx,
		`SELECT dispatched_at IS NULL FROM outbox_event WHERE id = $1`, envelope.ID.String()).Scan(&pending); err != nil {
		t.Fatalf("reading the event: %v", err)
	}
	if pending {
		t.Error("the event is still pending after a second round")
	}
}

// The cross-tenant negative test for Claim: a dispatcher opened for one tenant cannot see another
// tenant's events, however it asks.
func TestADispatcherCannotClaimAnotherTenantsEvents(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	container := containerIn(tenantA, authorA, freshID(t), freshName(t), "a0")
	envelope := announcement(t, tenantA, container)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewOutbox(jobQueue(t)).Append(ctx, envelope)
	}); err != nil {
		t.Fatalf("writing the event: %v", err)
	}

	var inB []event.Envelope
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		inB, err = postgres.NewOutbox(jobQueue(t)).Claim(ctx, 100)
		return err
	}); err != nil {
		t.Fatalf("claiming in tenant B: %v", err)
	}
	for _, claimed := range inB {
		if claimed.ID == envelope.ID {
			t.Fatal("a dispatcher in tenant B claimed tenant A's event")
		}
	}

	// And it is claimable where it belongs, so the test above is not passing for want of an event.
	var found bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		claimed, err := postgres.NewOutbox(jobQueue(t)).Claim(ctx, 100)
		for _, c := range claimed {
			found = found || c.ID == envelope.ID
		}
		return err
	}); err != nil {
		t.Fatalf("claiming in tenant A: %v", err)
	}
	if !found {
		t.Error("the event was not claimable in its own tenant")
	}
}

// The cross-tenant negative test for MarkDispatched: naming another tenant's event identifiers
// marks nothing, so no tenant can make another tenant's events disappear from its dispatcher.
func TestMarkingAnotherTenantsEventLeavesItPending(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	container := containerIn(tenantA, authorA, freshID(t), freshName(t), "a0")
	envelope := announcement(t, tenantA, container)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewOutbox(jobQueue(t)).Append(ctx, envelope)
	}); err != nil {
		t.Fatalf("writing the event: %v", err)
	}

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return postgres.NewOutbox(jobQueue(t)).MarkDispatched(ctx, []shared.ID{envelope.ID}, created)
	}); err != nil {
		t.Fatalf("marking from tenant B: %v", err)
	}

	admin := adminPool(ctx, t)
	var pending bool
	if err := admin.QueryRow(ctx,
		`SELECT dispatched_at IS NULL FROM outbox_event WHERE id = $1`, envelope.ID.String()).Scan(&pending); err != nil {
		t.Fatalf("reading the event: %v", err)
	}
	if !pending {
		t.Error("tenant B marked tenant A's event as delivered")
	}
}

// The cross-tenant negative test for the consumption record: it is per tenant, so one tenant's
// consumer cannot silence another tenant's - and within a tenant, the second claim is refused,
// which is the whole mechanism.
func TestConsumptionIsRecordedPerTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	consumption := postgres.NewConsumption(clockadapter.System{})
	eventID := freshID(t)

	claimIn := func(tenant shared.ID) bool {
		t.Helper()
		var first bool
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			first, err = consumption.Claim(ctx, "test.consumer", eventID)
			return err
		}); err != nil {
			t.Fatalf("claiming in %s: %v", tenant, err)
		}
		return first
	}

	if !claimIn(tenantA) {
		t.Error("the first claim in tenant A was refused")
	}
	if claimIn(tenantA) {
		t.Error("the same event was claimed twice in one tenant - a duplicate would have an effect")
	}
	if !claimIn(tenantB) {
		t.Error("tenant A's consumption silenced tenant B's consumer")
	}
}

// The other half of ADR-0007's first countermeasure, end to end: enqueueing rings the doorbell,
// and it rings because of the trigger rather than because anything remembered to announce.
//
// Here rather than in the adapter's own tests for the change listener's reason: whether a `NOTIFY`
// reaches a `LISTEN` is a property of the schema, and a fake cannot have that property.
func TestEnqueueingAJobWakesTheWorkersListener(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	listener := postgres.NewJobListener(appPool(ctx, t))
	listening, stopListening := context.WithCancel(ctx)
	defer stopListening()
	concurrency.Go(listening, "test.job_listener", listener.Run)

	waitFor(t, 5*time.Second, "the job listener to connect", listener.Connected)

	// The ring the listener sends on connecting, so that the assertion below is about the trigger
	// rather than about that one.
	drain(listener.Woken())

	jobs := postgres.NewQueue(clockadapter.NewUUIDv7(clockadapter.System{}), clockadapter.System{})
	work := postgres.NewUnitOfWork(appPool(ctx, t))

	started := time.Now()
	if err := work.Within(ctx, persistence.Scope{TenantID: tenantA}, func(txCtx context.Context) error {
		_, err := jobs.Enqueue(txCtx, queue.Request{
			Kind: queue.KindOutboxDispatch, TenantID: tenantA, RunAt: time.Now(),
		})
		return err
	}); err != nil {
		t.Fatalf("enqueueing: %v", err)
	}

	select {
	case <-listener.Woken():
	case <-time.After(5 * time.Second):
		t.Fatal("enqueueing a job did not wake the listener")
	}

	// The point of the whole mechanism, and the assertion is deliberately loose: what has to be
	// true is that the wait was nothing like the two-second poll interval the queue is configured
	// with, not that it was any particular number of milliseconds. A tighter bound would be a
	// test that fails on a loaded CI runner and says nothing when it does.
	if waited := time.Since(started); waited > time.Second {
		t.Errorf("the doorbell took %v; the poll interval it exists to beat is 2s", waited)
	}
}

// Delivered at COMMIT and not before: a runner woken by an uncommitted insert would claim nothing
// and go back to sleep having missed the job. That is the one property of the trigger that a unit
// test cannot have an opinion about.
func TestTheDoorbellRingsAtCommitRatherThanAtInsert(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	listener := postgres.NewJobListener(appPool(ctx, t))
	listening, stopListening := context.WithCancel(ctx)
	defer stopListening()
	concurrency.Go(listening, "test.job_listener_commit", listener.Run)

	waitFor(t, 5*time.Second, "the job listener to connect", listener.Connected)
	drain(listener.Woken())

	jobs := postgres.NewQueue(clockadapter.NewUUIDv7(clockadapter.System{}), clockadapter.System{})
	work := postgres.NewUnitOfWork(appPool(ctx, t))

	inserted := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	concurrency.Go(ctx, "test.job_listener_holding_tx", func(context.Context) {
		done <- work.Within(ctx, persistence.Scope{TenantID: tenantA}, func(txCtx context.Context) error {
			if _, err := jobs.Enqueue(txCtx, queue.Request{
				Kind: queue.KindOutboxDispatch, TenantID: tenantA, RunAt: time.Now(),
			}); err != nil {
				return err
			}
			close(inserted)
			<-release
			return nil
		})
	})

	<-inserted
	select {
	case <-listener.Woken():
		close(release)
		t.Fatal("the doorbell rang while the transaction was still open")
	case <-time.After(300 * time.Millisecond):
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("the transaction failed: %v", err)
	}

	select {
	case <-listener.Woken():
	case <-time.After(5 * time.Second):
		t.Fatal("the doorbell did not ring after the commit")
	}
}

// drain empties the doorbell without blocking on an empty one.
func drain(woken <-chan struct{}) {
	select {
	case <-woken:
	default:
	}
}
