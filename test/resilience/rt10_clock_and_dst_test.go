// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build resilience

package resilience

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	clockport "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	recurrenceadapter "github.com/Jersyfi/hubtask/infrastructure/recurrence"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// RT-10, from observability-reliability.md §12: the scheduler across a time change and after a two
// hour outage - no double firing and no missed firing.
//
// Both duties run here, against a real database and through the shipped adapters: the
// materialisation that turns a series into entries (D-05) and the firing that turns a reminder
// into a notification (D-03). What makes it RT-10 rather than two unit tests is the clock: it is a
// value this test moves, so a transition and an outage are things that happen to the passes rather
// than things a mock asserts about.
//
// The zone is Europe/Berlin and the days are the spring transition of 2026. What must hold across
// it is the local time: a daily series at 09:00 is a series of 09:00s, whatever that costs in UTC.

// steppingClock is the scheduler's clock as this test moves it.
type steppingClock struct{ at time.Time }

func (c *steppingClock) Now() time.Time { return c.at }

var _ clockport.Clock = (*steppingClock)(nil)

// rt10Notifier records what the firing pass handed to the notification context, which is what
// "fired" means here: the delivery itself is C-09's evidence.
type rt10Notifier struct{ told []shared.ID }

func (n *rt10Notifier) Execute(
	_ context.Context, _, itemID shared.ID, _ []shared.ID,
) error {
	n.told = append(n.told, itemID)
	return nil
}

// rt10Visibility answers the reach question this test does not vary.
type rt10Visibility struct{}

func (rt10Visibility) CanSee(
	context.Context, appshared.ActorContext, shared.ID, []identity.Scope,
) (bool, error) {
	return true, nil
}

// rt10Signals records the two measurements the duties publish, so the test can say what the run
// cost as well as what it did.
type rt10Signals struct {
	delays []float64
	lags   []float64
}

func (s *rt10Signals) ReminderFired(_ context.Context, _ string, delay float64) {
	s.delays = append(s.delays, delay)
}

func (s *rt10Signals) OccurrenceMaterialized(_ context.Context, _ string, lag float64) {
	s.lags = append(s.lags, lag)
}

func TestRT10TheScheduleSurvivesATimeChangeAndAnOutage(t *testing.T) {
	ctx := context.Background()
	unitOfWork := openUnitOfWork(ctx, t, testDSN(t))
	// Started at a real moment: the hybrid clock refuses a reading with no time in it, and the
	// test moves this value the moment the first pass runs anyway.
	clock := &steppingClock{at: time.Now()}
	ids := clockadapter.NewUUIDv7(clockport.Fixed(time.Now()))
	hlc, err := clockadapter.NewHybridClock(clock, "rt10")
	if err != nil {
		t.Fatalf("the hybrid clock: %v", err)
	}
	cursors := security.NewCursorCodec(secret.New("rt10 installation secret"))

	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("the zone does not load: %v", err)
	}

	// The template is due at 09:00 on the day before Berlin springs forward, and the series
	// repeats daily: the next two occurrences are on either side of the transition.
	due := time.Date(2026, 3, 28, 9, 0, 0, 0, berlin)
	tenant, collection, template := seedRT10Fixture(ctx, t, unitOfWork, cursors, ids, due)

	items := postgres.NewItemRepository(cursors)
	recurrences := postgres.NewRecurrenceRepository()
	reminders := postgres.NewReminderRepository()
	containers := postgres.NewContainerRepository(cursors)
	jobs := postgres.NewQueue(ids, clock)
	outbox := postgres.NewOutbox(jobs)
	signals := &rt10Signals{}
	notifier := &rt10Notifier{}

	materialisation := work.MaterializeOccurrences{
		Recurrences: recurrences, Items: items, Containers: containers,
		Copy: work.DuplicateWorkItem{
			Items: items, ItemLabels: postgres.NewItemLabelRepository(),
			ItemMembers: postgres.NewItemMemberRepository(),
			Labels:      postgres.NewLabelRepository(), Buckets: postgres.NewBucketRepository(),
			Fields: postgres.NewCustomFieldRepository(), Containers: containers,
			Attachments: postgres.NewMediaRepository(cursors),
			Media:       postgres.NewMediaRepository(cursors),
			Profiles:    postgres.NewCapabilityProfileRepository(),
			Events:      outbox, Changes: postgres.NewChangeLog(),
			Audit:      postgres.NewAuditSink(ids),
			Activity:   work.ActivityJournal{Entries: postgres.NewActivityRepository(cursors), IDs: ids},
			UnitOfWork: unitOfWork, Clock: clock, IDs: ids, HLC: hlc,
		},
		Expander: recurrenceadapter.New(), Events: outbox,
		Clock: clock, IDs: ids, Signals: signals,
		RuleBatch: work.DefaultRuleBatch, OccurrenceBatch: work.DefaultOccurrenceBatch,
	}
	firing := work.FireReminders{
		Reminders: reminders, Items: items, Schedule: items, Containers: containers,
		ItemMembers: postgres.NewItemMemberRepository(),
		Visibility:  rt10Visibility{}, Notifier: notifier, Events: outbox,
		Changes: postgres.NewChangeLog(),
		Clock:   clock, IDs: ids, HLC: hlc, Signals: signals,
		BatchSize: work.DefaultReminderBatch,
	}
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenant}

	runPasses := func(at time.Time) (work.MaterializationOutcome, work.ReminderOutcome) {
		clock.at = at
		var materialised work.MaterializationOutcome
		var fired work.ReminderOutcome
		if err := unitOfWork.Within(ctx, persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
			var err error
			if materialised, err = materialisation.Execute(ctx, actor); err != nil {
				return err
			}
			fired, err = firing.Execute(ctx, actor)
			return err
		}); err != nil {
			t.Fatalf("the passes at %s failed: %v", at.Format(time.RFC3339), err)
		}
		return materialised, fired
	}

	// --- Across the time change ------------------------------------------------------------
	//
	// The first pass runs two days before the transition, with a window that reaches past it.
	start := time.Date(2026, 3, 27, 6, 0, 0, 0, time.UTC)
	materialised, _ := runPasses(start)
	if materialised.Created < 3 {
		t.Fatalf("the first pass created %d occurrences, want the window's", materialised.Created)
	}

	occurrences := occurrencesOfSeries(ctx, t, unitOfWork, items, tenant, collection, template)
	if len(occurrences) < 3 {
		t.Fatalf("%d occurrences exist, want the window's", len(occurrences))
	}
	crossedTheTransition := false
	for _, occurrence := range occurrences {
		local := occurrence.Due.At.In(berlin)
		if local.Hour() != 9 || local.Minute() != 0 {
			t.Errorf("an occurrence falls at %s local, not at 09:00",
				local.Format(time.RFC3339))
		}
		if _, offset := local.Zone(); offset == 7200 {
			crossedTheTransition = true
		}
	}
	if !crossedTheTransition {
		t.Error("no occurrence landed after the transition - the window did not reach it")
	}

	// --- The outage --------------------------------------------------------------------------
	//
	// Nothing runs for two hours. The reminder on the template was due inside that window, and
	// the window itself moved on, so both duties owe something when the scheduler comes back.
	seedRT10Reminder(ctx, t, unitOfWork, reminders, ids, tenant, template,
		start.Add(30*time.Minute))

	back := start.Add(2 * time.Hour)
	_, fired := runPasses(back)
	if fired.Fired != 1 {
		t.Fatalf("the pass after the outage fired %d reminders, want exactly one", fired.Fired)
	}
	if len(notifier.told) != 1 {
		t.Fatalf("%d messages were written for one reminder", len(notifier.told))
	}
	if len(signals.delays) != 1 || signals.delays[0] < 5000 {
		t.Errorf("the delay reported is %v - the reminder should be visibly late", signals.delays)
	}

	// --- And again, at the same moment -------------------------------------------------------
	//
	// No double firing and no double materialisation: the reminder has left PENDING once, and the
	// watermark has moved.
	materialisedAgain, firedAgain := runPasses(back)
	if firedAgain.Fired != 0 {
		t.Errorf("a second pass fired %d reminders", firedAgain.Fired)
	}
	if materialisedAgain.Created != 0 {
		t.Errorf("a second pass created %d occurrences", materialisedAgain.Created)
	}
	if len(notifier.told) != 1 {
		t.Errorf("%d messages exist for one reminder", len(notifier.told))
	}

	// Every occurrence is still exactly one entry, and every announcement is exactly one event.
	after := occurrencesOfSeries(ctx, t, unitOfWork, items, tenant, collection, template)
	seen := map[string]int{}
	for _, occurrence := range after {
		seen[occurrence.Due.At.UTC().Format(time.RFC3339)]++
	}
	for moment, count := range seen {
		if count != 1 {
			t.Errorf("the occurrence for %s exists %d times", moment, count)
		}
	}
	if announced := announcedOccurrences(ctx, t, unitOfWork, outbox, tenant); announced != len(after) {
		t.Errorf("%d occurrences exist and %d were announced", len(after), announced)
	}
}

// seedRT10Fixture writes the tenant, the account, the hub, the collection, the template and its
// series, and answers what the test needs to find them again.
func seedRT10Fixture(
	ctx context.Context, t *testing.T, unitOfWork *postgres.UnitOfWork,
	cursors security.CursorCodec, ids clockport.IDGenerator, due time.Time,
) (tenant, collection, template shared.ID) {
	t.Helper()

	tenant = ids.NewID()
	account, hub := ids.NewID(), ids.NewID()
	collection, template = ids.NewID(), ids.NewID()

	// The tenant and the account are the control plane's act rather than the application's
	// (db/migrations/0001_init.sql), so they are written as the owner - inside the container, so
	// that this package still holds no database driver.
	execAsOwner(ctx, t, fmt.Sprintf(
		`INSERT INTO tenant (id, slug, display_name) VALUES ('%s', 'rt10-%s', 'RT-10')`,
		tenant, tenant.String()[:8]))
	execAsOwner(ctx, t, fmt.Sprintf(
		`INSERT INTO account (id, tenant_id, display_name) VALUES ('%s', '%s', 'Scheduler')`,
		account, tenant))

	containers := postgres.NewContainerRepository(cursors)
	items := postgres.NewItemRepository(cursors)
	recurrences := postgres.NewRecurrenceRepository()
	at := due.Add(-48 * time.Hour)

	if err := unitOfWork.Within(ctx, persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
		if err := containers.Insert(ctx, workdomain.Container{
			ID: hub, TenantID: tenant, Type: workdomain.ContainerHub, Name: "RT-10",
			OrderKey: "a0", CreatedBy: account, CreatedAt: at, UpdatedAt: at, Version: 1,
		}); err != nil {
			return err
		}
		if err := containers.Insert(ctx, workdomain.Container{
			ID: collection, TenantID: tenant, Type: workdomain.ContainerCollection, ParentID: hub,
			Name: "Series", OrderKey: "a0", CreatedBy: account, CreatedAt: at, UpdatedAt: at,
			Version: 1,
		}); err != nil {
			return err
		}

		dueDate, err := workdomain.NewDueDate(&due, false, "Europe/Berlin")
		if err != nil {
			return err
		}
		task := workdomain.WorkItem{
			ID: template, TenantID: tenant, CollectionID: collection, Type: workdomain.ItemTask,
			Path: workdomain.RootPath(template), Depth: 1, Title: "Water the plants",
			OrderKey: "a0", CreatedBy: account, CreatedAt: at, UpdatedAt: at, Version: 1,
		}
		if err := items.Insert(ctx, task); err != nil {
			return err
		}
		task.Due = dueDate
		if err := items.SetDueDate(ctx, task, 1); err != nil {
			return err
		}

		rule, err := workdomain.NewRecurrenceRule(workdomain.NewRecurrenceRuleInput{
			ID: ids.NewID(), TenantID: tenant, ItemID: template,
			Spec: workdomain.RecurrenceSpec{
				RRULE: "FREQ=DAILY", TimeZone: "Europe/Berlin",
				Mode: string(workdomain.RecurrenceOnSchedule), HorizonDays: 5,
			},
			Due: dueDate, Now: at,
		})
		if err != nil {
			return err
		}
		return recurrences.Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("seeding the series: %v", err)
	}
	return tenant, collection, template
}

// seedRT10Reminder puts one absolute reminder on the template, due inside the outage window.
func seedRT10Reminder(
	ctx context.Context, t *testing.T, unitOfWork *postgres.UnitOfWork,
	reminders postgres.ReminderRepository, ids clockport.IDGenerator,
	tenant, template shared.ID, at time.Time,
) {
	t.Helper()

	reminder, err := workdomain.NewReminder(workdomain.NewReminderInput{
		ID: ids.NewID(), TenantID: tenant, ItemID: template,
		OffsetSpec: "ABS:" + at.UTC().Format(time.RFC3339), Now: at,
	})
	if err != nil {
		t.Fatalf("the reminder was refused: %v", err)
	}
	if err := unitOfWork.Within(ctx, persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
		return reminders.Insert(ctx, reminder)
	}); err != nil {
		t.Fatalf("seeding the reminder: %v", err)
	}
}

// occurrencesOfSeries reads the entries a series produced, through the repository rather than
// through a query of this test's own: what the read answers about recurrence_rule_id is part of
// what D-04 and D-05 promise, and a test that went around it would not be reading what a client
// reads.
func occurrencesOfSeries(
	ctx context.Context, t *testing.T, unitOfWork *postgres.UnitOfWork,
	items postgres.ItemRepository, tenant, collection, template shared.ID,
) []workdomain.WorkItem {
	t.Helper()

	var page repository.ItemPage
	if err := unitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
		var err error
		page, err = items.List(ctx, repository.ItemQuery{
			CollectionID: collection, Page: repository.Page{Size: 100},
		})
		return err
	}); err != nil {
		t.Fatalf("reading the occurrences: %v", err)
	}

	var found []workdomain.WorkItem
	for _, item := range page.Items {
		if item.ID == template || item.RecurrenceRuleID.IsZero() {
			continue
		}
		found = append(found, item)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Due.At.Before(found[j].Due.At) })
	return found
}

// announcedOccurrences counts the recurrence.occurrence_created events in the outbox: one per
// occurrence, which is what "no double materialisation" means to a consumer.
//
// Read the way the dispatcher reads them, which is the only reader the outbox has - so this is
// also the last assertion of the test and claims what it counts.
func announcedOccurrences(
	ctx context.Context, t *testing.T, unitOfWork *postgres.UnitOfWork,
	outbox postgres.Outbox, tenant shared.ID,
) int {
	t.Helper()

	var pending []event.Envelope
	if err := unitOfWork.Within(ctx, persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
		var err error
		pending, err = outbox.Claim(ctx, 500)
		return err
	}); err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}

	announced := 0
	for _, envelope := range pending {
		if envelope.Type == event.RecurrenceOccurrenceCreated {
			announced++
		}
	}
	return announced
}
