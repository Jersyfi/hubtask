// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The firing pass against a real database (D-03): the guarded transition under concurrency, the
// delay that SLO-5 is written about, and the announcements that happen once per deadline.

// reminderTold records what the pass handed to the Notification context.
type reminderTold struct {
	mu   sync.Mutex
	told []shared.ID
}

func (r *reminderTold) Execute(
	_ context.Context, _, itemID shared.ID, _ []shared.ID,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.told = append(r.told, itemID)
	return nil
}

func (r *reminderTold) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.told)
}

// everybodyCanSee is the visibility question as this suite answers it: the reach of a named
// recipient is C-01's business and is tested there, and what is under test here is the firing.
type everybodyCanSee struct{}

func (everybodyCanSee) CanSee(
	context.Context, appshared.ActorContext, shared.ID, []identity.Scope,
) (bool, error) {
	return true, nil
}

// announcements collects what the pass published, since the outbox itself is C-05's evidence.
type announcementLog struct {
	mu       sync.Mutex
	appended []event.Envelope
}

func (a *announcementLog) Append(_ context.Context, envelope event.Envelope) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.appended = append(a.appended, envelope)
	return nil
}

func (a *announcementLog) typed(want event.Type) []event.Envelope {
	a.mu.Lock()
	defer a.mu.Unlock()

	var found []event.Envelope
	for _, envelope := range a.appended {
		if envelope.Type == want {
			found = append(found, envelope)
		}
	}
	return found
}

// delayLog records the one measurement SLO-5 is written about.
type delayLog struct {
	mu      sync.Mutex
	seconds []float64
}

func (d *delayLog) ReminderFired(_ context.Context, _ string, delaySeconds float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seconds = append(d.seconds, delaySeconds)
}

// firingTenant is this file's own, and the reason is the pass itself: it fires everything the
// tenant owes, so a shared tenant would make every count here depend on what another test left
// pending. Its own tenant is the only way these assertions can say a number.
var (
	firingTenant = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000000f")
	firingAuthor = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000000af")
)

// seedFiringTenant creates it, once per run.
func seedFiringTenant(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)

	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'tenant-firing', 'Firing')
		ON CONFLICT (id) DO NOTHING`, firingTenant.String()); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Cara')
		ON CONFLICT (id) DO NOTHING`,
		firingAuthor.String(), firingTenant.String()); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
}

type firingFixture struct {
	pass   work.FireReminders
	told   *reminderTold
	events *announcementLog
	delays *delayLog
}

func newFiring(t *testing.T, at time.Time) firingFixture {
	t.Helper()

	fixture := firingFixture{
		told: &reminderTold{}, events: &announcementLog{}, delays: &delayLog{},
	}
	fixture.pass = work.FireReminders{
		Reminders: reminderRepo(), Items: itemRepo(), Schedule: itemRepo(),
		Containers: containerRepo(), ItemMembers: postgres.NewItemMemberRepository(),
		Visibility: everybodyCanSee{}, Notifier: fixture.told, Events: fixture.events,
		Clock: clock.Fixed(at), IDs: clockadapter.NewUUIDv7(clock.Fixed(at)),
		Signals:   fixture.delays,
		BatchSize: 50,
	}
	return fixture
}

func firingActor() appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: firingTenant}
}

// A reminder whose moment has passed fires once, and how late it was lands in the metric. Running
// the pass again changes nothing: the row left PENDING in the first pass and cannot leave it twice.
func TestAReminderFiresOnceAndReportsHowLateItWas(t *testing.T) {
	ctx := context.Background()
	seedFiringTenant(ctx, t)
	_, collection := hubWithCollection(ctx, t, firingTenant, firingAuthor)
	task := seedTask(ctx, t, firingTenant, firingAuthor, collection)

	promised := created.Add(time.Hour)
	reminder := seedReminder(ctx, t, firingTenant, task, "ABS:"+promised.Format(time.RFC3339), nil)

	fixture := newFiring(t, promised.Add(90*time.Second))
	var outcome work.ReminderOutcome
	if err := write(ctx, t, firingTenant, func(ctx context.Context) error {
		var err error
		outcome, err = fixture.pass.Execute(ctx, firingActor())
		return err
	}); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	if outcome.Fired != 1 || outcome.NextAt != nil {
		t.Fatalf("the pass reported %+v", outcome)
	}
	if fixture.told.count() != 1 {
		t.Fatalf("%d messages were written", fixture.told.count())
	}
	if len(fixture.delays.seconds) != 1 || fixture.delays.seconds[0] != 90 {
		t.Errorf("the delay reported is %v, want 90 seconds", fixture.delays.seconds)
	}

	var stored workdomain.Reminder
	if err := read(ctx, t, firingTenant, func(ctx context.Context) error {
		var err error
		stored, err = reminderRepo().Find(ctx, reminder.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if stored.State != workdomain.ReminderSent {
		t.Errorf("the reminder is %s rather than sent", stored.State)
	}

	second := newFiring(t, promised.Add(2*time.Hour))
	if err := write(ctx, t, firingTenant, func(ctx context.Context) error {
		var err error
		outcome, err = second.pass.Execute(ctx, firingActor())
		return err
	}); err != nil {
		t.Fatalf("the second pass failed: %v", err)
	}
	if outcome.Fired != 0 || second.told.count() != 0 {
		t.Errorf("the second pass fired %d", outcome.Fired)
	}
}

// The acceptance's concurrency case, with two real transactions: two leaders that both wake up
// produce exactly one send. The claim's SKIP LOCKED keeps them apart and the guarded transition
// decides it if they ever meet.
func TestTwoPassesOverOneReminderProduceOneSend(t *testing.T) {
	ctx := context.Background()
	seedFiringTenant(ctx, t)
	_, collection := hubWithCollection(ctx, t, firingTenant, firingAuthor)
	task := seedTask(ctx, t, firingTenant, firingAuthor, collection)

	promised := created.Add(time.Hour)
	for i := 0; i < 5; i++ {
		seedReminder(ctx, t, firingTenant, task,
			"ABS:"+promised.Add(time.Duration(i)*time.Minute).Format(time.RFC3339), nil)
	}

	first, second := newFiring(t, promised.Add(time.Hour)), newFiring(t, promised.Add(time.Hour))
	var wait sync.WaitGroup
	fired := make([]int, 2)
	for index, fixture := range []firingFixture{first, second} {
		wait.Add(1)
		go func(index int, fixture firingFixture) {
			defer wait.Done()
			_ = write(ctx, t, firingTenant, func(ctx context.Context) error {
				outcome, err := fixture.pass.Execute(ctx, firingActor())
				fired[index] = outcome.Fired
				return err
			})
		}(index, fixture)
	}
	wait.Wait()

	if total := fired[0] + fired[1]; total != 5 {
		t.Errorf("the two passes fired %d of five reminders", total)
	}
	if told := first.told.count() + second.told.count(); told != 5 {
		t.Errorf("%d messages were written for five reminders", told)
	}

	var left int
	if err := read(ctx, t, firingTenant, func(ctx context.Context) error {
		reminders, err := reminderRepo().ListForItem(ctx, task)
		for _, reminder := range reminders {
			if reminder.State != workdomain.ReminderSent {
				left++
			}
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d reminders are still pending", left)
	}
}

// The server was down across the moment: the reminder fires once on its return, and the metric
// says how late it was rather than pretending it was on time.
func TestAReminderMissedDuringAnOutageFiresOnceAndVisiblyLate(t *testing.T) {
	ctx := context.Background()
	seedFiringTenant(ctx, t)
	_, collection := hubWithCollection(ctx, t, firingTenant, firingAuthor)
	task := seedTask(ctx, t, firingTenant, firingAuthor, collection)

	promised := created.Add(time.Hour)
	seedReminder(ctx, t, firingTenant, task, "ABS:"+promised.Format(time.RFC3339), nil)

	// Two hours of nothing running, which is the shape of §6's bounded catch-up.
	fixture := newFiring(t, promised.Add(2*time.Hour))
	var outcome work.ReminderOutcome
	if err := write(ctx, t, firingTenant, func(ctx context.Context) error {
		var err error
		outcome, err = fixture.pass.Execute(ctx, firingActor())
		return err
	}); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	if outcome.Fired != 1 || fixture.told.count() != 1 {
		t.Fatalf("the pass reported %+v and told %d", outcome, fixture.told.count())
	}
	if len(fixture.delays.seconds) != 1 || fixture.delays.seconds[0] != 2*time.Hour.Seconds() {
		t.Errorf("the delay reported is %v", fixture.delays.seconds)
	}
}

// item.overdue is announced once per deadline, and never for work that is done, deleted or put
// away.
func TestTheOverdueAnnouncementHappensOnceAndNotForClosedWork(t *testing.T) {
	ctx := context.Background()
	seedFiringTenant(ctx, t)
	_, collection := hubWithCollection(ctx, t, firingTenant, firingAuthor)
	open := seedTask(ctx, t, firingTenant, firingAuthor, collection)
	done := seedTask(ctx, t, firingTenant, firingAuthor, collection)
	trashed := seedTask(ctx, t, firingTenant, firingAuthor, collection)

	overdueAt := created.Add(time.Hour)
	for _, id := range []shared.ID{open, done, trashed} {
		setDueDate(ctx, t, id, overdueAt)
	}
	stampColumn(ctx, t, trashed, "deleted_at")
	completeItem(ctx, t, done)

	fixture := newFiring(t, overdueAt.Add(time.Minute))
	if err := write(ctx, t, firingTenant, func(ctx context.Context) error {
		_, err := fixture.pass.Execute(ctx, firingActor())
		return err
	}); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	announced := fixture.events.typed(event.ItemOverdue)
	if len(announced) != 1 {
		t.Fatalf("%d entries were announced overdue, want the open one alone", len(announced))
	}
	if announced[0].Payload["item_id"] != open.String() {
		t.Errorf("the announcement is about %v", announced[0].Payload["item_id"])
	}

	// And again: the stamp is what makes it once, so a second pass announces nothing.
	second := newFiring(t, overdueAt.Add(time.Hour))
	if err := write(ctx, t, firingTenant, func(ctx context.Context) error {
		_, err := second.pass.Execute(ctx, firingActor())
		return err
	}); err != nil {
		t.Fatalf("the second pass failed: %v", err)
	}
	if announced := second.events.typed(event.ItemOverdue); len(announced) != 0 {
		t.Errorf("the deadline was announced again: %+v", announced)
	}
}

// setDueDate puts a plain instant on an entry through the writer that owns the trio, so that the
// announcement stamps are cleared exactly as they are in production.
func setDueDate(ctx context.Context, t *testing.T, id shared.ID, at time.Time) {
	t.Helper()

	item := findWorkItem(ctx, t, firingTenant, id)
	item.Due = &workdomain.DueDate{At: at}
	item.UpdatedAt = at
	if err := write(ctx, t, firingTenant, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, item, item.Version)
	}); err != nil {
		t.Fatalf("setting the due date: %v", err)
	}
}

// completeItem marks an entry done through the column the scan reads, since what is under test is
// the scan rather than the completion use case.
func completeItem(ctx context.Context, t *testing.T, id shared.ID) {
	t.Helper()

	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE work_item SET is_completed = true, completed_at = now() WHERE id = $1`,
		id.String()); err != nil {
		t.Fatalf("completing the entry: %v", err)
	}
}
