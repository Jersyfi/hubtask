// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	recurrenceadapter "github.com/Jersyfi/hubtask/infrastructure/recurrence"
)

// The materialisation against a real database (D-05): what two passes at once produce, what an
// ON_COMPLETION series owes for one transition (SY-8), and what a rolling window holds.

// Every test here gets a tenant of its own, and the reason is the pass itself: it materialises
// everything a tenant owes, so a shared tenant would make every count depend on what another test
// left behind - and the audit chain, which is per tenant and gapless, would have two tests writing
// into it at the same fixed clock.
func seedMaterialisingTenant(ctx context.Context, t *testing.T) (tenant, author shared.ID) {
	t.Helper()
	admin := adminPool(ctx, t)

	tenant, author = freshID(t), freshID(t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Series')`,
		tenant.String(), "series-"+tenant.String()[24:]); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Dora')`,
		author.String(), tenant.String()); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	return tenant, author
}

// newMaterialisation wires the pass the way the composition root does, with the real copy.
func newMaterialisation(
	ctx context.Context, t *testing.T, at time.Time,
) work.MaterializeOccurrences {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	cursors := pageCursors()
	items := postgres.NewItemRepository(cursors)
	containers := containerRepo()
	fixed := clock.Fixed(at)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	hlc, err := clockadapter.NewHybridClock(fixed, "materialisation-test")
	if err != nil {
		panic(err)
	}
	outbox := postgres.NewOutbox(postgres.NewQueue(ids, fixed))

	return work.MaterializeOccurrences{
		Recurrences: recurrenceRepo(), Items: items, Containers: containers,
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
			UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hlc,
		},
		Expander: recurrenceadapter.New(), Events: outbox,
		Clock: fixed, IDs: ids,
		RuleBatch: work.DefaultRuleBatch, OccurrenceBatch: work.DefaultOccurrenceBatch,
	}
}

func materialisingActor(tenant shared.ID) appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenant}
}

// seedSeries writes a template with a due date and the series that repeats it.
func seedSeries(
	ctx context.Context, t *testing.T, mode workdomain.RecurrenceMode, horizon int, due time.Time,
) (tenant, collection, template shared.ID) {
	t.Helper()

	tenant, author := seedMaterialisingTenant(ctx, t)
	_, collection = hubWithCollection(ctx, t, tenant, author)
	template = seedTask(ctx, t, tenant, author, collection)

	dueDate, err := workdomain.NewDueDate(&due, false, "Europe/Berlin")
	if err != nil {
		t.Fatalf("the due date was refused: %v", err)
	}
	item := findWorkItem(ctx, t, tenant, template)
	item.Due = dueDate
	item.UpdatedAt = due
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, item, item.Version)
	}); err != nil {
		t.Fatalf("setting the due date: %v", err)
	}

	rule, err := workdomain.NewRecurrenceRule(workdomain.NewRecurrenceRuleInput{
		ID: freshID(t), TenantID: tenant, ItemID: template,
		Spec: workdomain.RecurrenceSpec{
			RRULE: "FREQ=DAILY", TimeZone: "Europe/Berlin", Mode: string(mode),
			HorizonDays: horizon,
		},
		Due: dueDate, Now: due,
	})
	if err != nil {
		t.Fatalf("the series was refused: %v", err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return recurrenceRepo().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the series: %v", err)
	}
	return tenant, collection, template
}

// occurrencesOf reads the entries a series produced, oldest date first.
func occurrencesOf(
	ctx context.Context, t *testing.T, tenant, collection, template shared.ID,
) []workdomain.WorkItem {
	t.Helper()

	var listed []workdomain.WorkItem
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		page, err := itemRepo().List(ctx, repository.ItemQuery{
			CollectionID: collection, Page: repository.Page{Size: 100},
		})
		if err != nil {
			return err
		}
		listed = page.Items
		return nil
	}); err != nil {
		t.Fatalf("reading the occurrences: %v", err)
	}

	var found []workdomain.WorkItem
	for _, item := range listed {
		if item.ID == template || item.RecurrenceRuleID.IsZero() {
			continue
		}
		found = append(found, item)
	}
	return found
}

// Two passes at once produce one set of entries: the claim keeps them apart and the watermark's
// compare-and-set decides it if they ever meet.
func TestTwoPassesOverOneSeriesProduceOneSetOfOccurrences(t *testing.T) {
	ctx := context.Background()
	due := created.Add(24 * time.Hour)
	tenant, collection, template := seedSeries(ctx, t, workdomain.RecurrenceOnSchedule, 3, due)

	at := due.Add(time.Hour)
	first, second := newMaterialisation(ctx, t, at), newMaterialisation(ctx, t, at)

	var wait sync.WaitGroup
	created := make([]int, 2)
	for index, pass := range []work.MaterializeOccurrences{first, second} {
		wait.Add(1)
		go func(index int, pass work.MaterializeOccurrences) {
			defer wait.Done()
			// One of the two may lose the watermark and roll back, which is the mechanism under
			// test: its error is the correct outcome, not a failure of the run.
			_ = write(ctx, t, tenant, func(ctx context.Context) error {
				outcome, err := pass.Execute(ctx, materialisingActor(tenant))
				created[index] = outcome.Created
				return err
			})
		}(index, pass)
	}
	wait.Wait()

	occurrences := occurrencesOf(ctx, t, tenant, collection, template)
	if len(occurrences) == 0 {
		t.Fatal("two passes produced no occurrences at all")
	}

	moments := map[string]int{}
	for _, occurrence := range occurrences {
		moments[occurrence.Due.At.UTC().Format(time.RFC3339)]++
	}
	for moment, count := range moments {
		if count != 1 {
			t.Errorf("the occurrence for %s exists %d times", moment, count)
		}
	}
	if total := created[0] + created[1]; total != len(occurrences) {
		t.Errorf("the passes reported %d creations for %d occurrences", total, len(occurrences))
	}
}

// SY-8's server half: an ON_COMPLETION series owes one follow-up for one transition, and two
// passes that both see the completion produce one entry rather than two.
func TestACompletionSeriesProducesOneFollowUpForOneTransition(t *testing.T) {
	ctx := context.Background()
	due := created.Add(24 * time.Hour)
	tenant, collection, template := seedSeries(ctx, t, workdomain.RecurrenceOnCompletion, 30, due)

	at := due.Add(time.Hour)
	// While the template is open the series owes nothing: that is what "only once its predecessor
	// is completed" means.
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		outcome, err := newMaterialisation(ctx, t, at).Execute(ctx, materialisingActor(tenant))
		if err == nil && outcome.Created != 0 {
			t.Errorf("an open series produced %d occurrences", outcome.Created)
		}
		return err
	}); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	completeItem(ctx, t, template)

	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_ = write(ctx, t, tenant, func(ctx context.Context) error {
				_, err := newMaterialisation(ctx, t, at).Execute(ctx, materialisingActor(tenant))
				return err
			})
		}()
	}
	wait.Wait()

	occurrences := occurrencesOf(ctx, t, tenant, collection, template)
	if len(occurrences) != 1 {
		t.Fatalf("a completed series produced %d follow-ups, want exactly one", len(occurrences))
	}
	if !occurrences[0].Due.At.After(due) {
		t.Errorf("the follow-up is due %v, which is not after the one that was completed",
			occurrences[0].Due.At)
	}

	// And nothing more while the follow-up is open, which is the state the series waits in.
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		outcome, err := newMaterialisation(ctx, t, at.Add(48*time.Hour)).Execute(ctx, materialisingActor(tenant))
		if err == nil && outcome.Created != 0 {
			t.Errorf("the series produced %d more while its follow-up is open", outcome.Created)
		}
		return err
	}); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
}

// The window is a window: however often the pass runs at one moment, what stands ahead is what the
// horizon says and no more.
func TestTheHorizonHoldsWhatItSaysAndNoMore(t *testing.T) {
	ctx := context.Background()
	due := created.Add(24 * time.Hour)
	tenant, collection, template := seedSeries(ctx, t, workdomain.RecurrenceOnSchedule, 3, due)

	at := due.Add(time.Hour)
	for range 4 {
		if err := write(ctx, t, tenant, func(ctx context.Context) error {
			_, err := newMaterialisation(ctx, t, at).Execute(ctx, materialisingActor(tenant))
			return err
		}); err != nil {
			t.Fatalf("the pass failed: %v", err)
		}
	}

	occurrences := occurrencesOf(ctx, t, tenant, collection, template)
	horizon := at.AddDate(0, 0, 3)
	for _, occurrence := range occurrences {
		if occurrence.Due.At.After(horizon) {
			t.Errorf("an occurrence is due %v, past the horizon at %v",
				occurrence.Due.At, horizon)
		}
	}
	// A daily series in a three-day window: three entries ahead, not four and not thirty.
	if len(occurrences) > 3 {
		t.Errorf("the window holds %d occurrences", len(occurrences))
	}

	// And time moving is what makes it roll: a pass a day later owes the next one.
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		outcome, err := newMaterialisation(ctx, t, at.Add(24*time.Hour)).Execute(ctx, materialisingActor(tenant))
		if err == nil && outcome.Created == 0 {
			t.Error("the window did not roll forward with the clock")
		}
		return err
	}); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
}
