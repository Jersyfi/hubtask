// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-PRV-05 check 6: a withdrawal switches off the processing it names and nothing else. Kim keeps
// her content out of AI, and capturing, planning, sharing and reminders still work for her through
// the real use cases against PostgreSQL - none of them may read the consent register as a gate.

// coreFeaturesCatalogueFor wires the core features a person uses every day to the real adapters.
func coreFeaturesCatalogueFor(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()

	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sink := postgres.NewAuditSink(ids)
	profiles := postgres.NewCapabilityProfileRepository()
	journal := work.ActivityJournal{Entries: historyRepo(), IDs: ids}
	outbox := postgres.NewOutbox(jobQueue(t))
	changes := postgres.NewChangeLog()
	authorizer := realAuthoriser(ctx, t)
	dueDates := work.DueDateWriter{
		Items: itemRepo(), Containers: containerRepo(), Profiles: profiles,
		Reminders: postgres.NewReminderRepository(), Jobs: jobQueue(t), Authorizer: authorizer,
		Events: outbox, Changes: changes, Audit: sink, Activity: journal,
		UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
	}

	registry, err := usecase.NewRegistry(nil,
		work.CreateWorkItem{
			Items: itemRepo(), Containers: containerRepo(), Profiles: profiles,
			Authorizer: authorizer, Ownership: authorizer, Events: outbox, Changes: changes,
			Audit: sink, Activity: journal, UnitOfWork: unitOfWork,
			Clock: fixed, IDs: ids, HLC: hybrid, DueDates: dueDates,
		}.Descriptor(),
		work.SetDueDate{Due: dueDates}.Descriptor(),
		work.CreateReminder{Writer: work.ReminderWriter{
			Reminders: postgres.NewReminderRepository(), Items: itemRepo(), Containers: containerRepo(),
			Profiles: profiles, Authorizer: authorizer, Visibility: authorizer, Changes: changes,
			Audit: sink, Jobs: jobQueue(t), UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
		}}.Descriptor(),
		identityservice.GrantMembership{
			Grants: postgres.NewMembershipGrantRepository(pageCursors()), Accounts: postgres.NewAccountRepository(),
			Groups: postgres.NewGroupRepository(pageCursors()), Hubs: postgres.NewHubLockRepository(),
			Authorizer: authorizer, Audit: sink, UnitOfWork: unitOfWork, Clock: fixed, IDs: ids,
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

func TestTheCoreFeaturesWorkTheSameAfterAWithdrawal(t *testing.T) {
	ctx := context.Background()
	w := seedKeptWorld(ctx, t)
	w.record(ctx, t, false, created.Add(-time.Hour))

	// The withdrawal is in force where it is meant to be, or nothing below proves anything.
	var kept integration.KeptOut
	if err := read(ctx, t, w.tenant, func(ctx context.Context) error {
		var err error
		kept, err = keptOutExclusion().Read(ctx, w.tenant)
		return err
	}); err != nil {
		t.Fatalf("reading who is kept out of AI: %v", err)
	}
	if !kept.Has(w.kim) {
		t.Fatal("Kim's withdrawal does not keep her out of AI, so this test proves nothing")
	}

	registry := coreFeaturesCatalogueFor(ctx, t)
	actor := func(account shared.ID, scopes ...string) appshared.ActorContext {
		return appshared.ActorContext{
			Kind: appshared.ActorUser, TenantID: w.tenant, AccountID: account, AccountName: "Someone",
			Locale: "en", TimeZone: "UTC", Scopes: scopes,
		}
	}
	kim := actor(w.kim, "items:write", "reminders:write")
	invoke := func(who appshared.ActorContext, name string, in usecase.Input) usecase.Output {
		t.Helper()
		out, err := registry.Invoke(ctx, name, who, in)
		if err != nil {
			t.Fatalf("%s after Kim's withdrawal: %v", name, err)
		}
		return out
	}

	// Sharing: the hub's owner gives Kim a role on it.
	invoke(actor(w.ada, "members:write"), identityservice.GrantMembershipName, usecase.Input{
		"scope_type": "HUB", "scope_id": w.hub.String(), "account_id": w.kim.String(), "role": "ADMIN",
	})
	if rows := countIn(ctx, t, `SELECT count(*) FROM membership WHERE tenant_id = $1 AND account_id = $2
		AND scope_type = 'HUB' AND scope_id = $3 AND role = 'ADMIN'`,
		w.tenant.String(), w.kim.String(), w.hub.String()); rows != 1 {
		t.Errorf("%d grants of Kim's role on the hub", rows)
	}

	// Capturing: Kim writes an entry down.
	entry := invoke(kim, work.CreateWorkItemName, usecase.Input{
		"type": "TASK", "collection_id": w.collection.String(), "title": "Book the venue",
	}).String("id")
	if rows := countIn(ctx, t, `SELECT count(*) FROM work_item WHERE id = $1 AND created_by = $2`,
		entry, w.kim.String()); rows != 1 {
		t.Fatal("Kim's entry was not written")
	}

	// Planning: she gives it a due date.
	invoke(kim, work.SetDueDateName, usecase.Input{"item_id": entry, "due_at": "2026-09-01T09:00:00Z"})
	if rows := countIn(ctx, t, `SELECT count(*) FROM work_item WHERE id = $1 AND due_at = '2026-09-01T09:00:00Z'`,
		entry); rows != 1 {
		t.Error("Kim's entry carries no due date")
	}

	// Reminders: one for her, an hour before, written and scheduled.
	invoke(kim, work.CreateReminderName, usecase.Input{
		"item_id": entry, "offset_spec": "REL:-PT1H", "recipients": []any{w.kim.String()},
	})
	if rows := countIn(ctx, t, `SELECT count(*) FROM reminder WHERE item_id = $1
		AND fire_at = '2026-09-01T08:00:00Z' AND $2 = ANY(recipients)`, entry, w.kim.String()); rows != 1 {
		t.Error("Kim's reminder was not written to fire an hour before the due date")
	}
	if rows := countIn(ctx, t, `SELECT count(*) FROM job WHERE kind = 'reminder.fire' AND dedupe_key = $1
		AND run_at <= '2026-09-01T08:00:00Z'`, w.tenant.String()); rows != 1 {
		t.Error("no wake-up was scheduled for Kim's reminder")
	}
}
