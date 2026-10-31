// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A calendar, a policy on it and an entry's record are stored and read back, and the workspace next
// door sees none of them (migration 0240, UC-SVC-01, UC-SVC-02). Since migration 0291 the use cases
// hold a policy's calendar by key - the installation's calendars are no rows -, so the store takes
// a policy on a key the workspace does not hold and lets a named calendar go.
func TestServiceDeskBlocksStayInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewServiceDeskRepository()
	now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)

	cal, err := servicedesk.NewCalendar(servicedesk.NewCalendarInput{
		ID: freshID(t), TenantID: tenantA, Key: "office", Name: "Office", Spec: officeCalendar(t).Spec(),
		Now: now, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := servicedesk.NewSlaPolicy(servicedesk.NewSlaPolicyInput{
		ID: freshID(t), TenantID: tenantA, Key: "incidents", Name: "Incidents", CalendarKey: "office",
		EntryTypes: []string{"incident"}, Enabled: true, Now: now, Text: text.Composing{},
		Goals: []servicedesk.SlaGoal{{Kind: servicedesk.GoalResolution, Minutes: 240, PauseStatuses: []string{"waiting"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.Calendars().Insert(ctx, cal); err != nil {
			return err
		}
		return repo.Policies().Insert(ctx, policy)
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := repo.Calendars().Find(ctx, "office"); !errors.Is(err, shared.ErrNotFound) {
			return errors.Join(err, errors.New("the calendar crossed the workspace"))
		}
		if list, err := repo.Policies().List(ctx); err != nil || len(list) != 0 {
			return errors.Join(err, errors.New("the policy crossed the workspace"))
		}
		return nil
	}); err != nil {
		t.Error(err)
	}

	stray := policy
	stray.ID, stray.Key, stray.CalendarKey = freshID(t), "offered", "federal"
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Policies().Insert(ctx, stray)
	}); err != nil {
		t.Errorf("a policy on a key the installation offers: %v", err)
	}
	// A policy for organisations reads them back; one for every requester holds none (migration 0294).
	vip := policy
	vip.ID, vip.Key, vip.Organisations = freshID(t), "key_accounts", []string{"acme"}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.Policies().Insert(ctx, vip); err != nil {
			return err
		}
		vip.Organisations = []string{"acme", "globex"}
		return repo.Policies().Update(ctx, vip, vip.Version)
	}); err != nil {
		t.Fatalf("a policy for organisations: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := repo.Policies().Find(ctx, "key_accounts")
		if err != nil || !slices.Equal(found.Organisations, []string{"acme", "globex"}) {
			return errors.Join(err, fmt.Errorf("organisations read back as %v", found.Organisations))
		}
		everyone, err := repo.Policies().Find(ctx, "incidents")
		if err != nil || len(everyone.Organisations) != 0 {
			return errors.Join(err, fmt.Errorf("a policy for everyone reads %v", everyone.Organisations))
		}
		return nil
	}); err != nil {
		t.Error(err)
	}

	item := seedWorkItemForRules(ctx, t, tenantA)
	due := now.Add(4 * time.Hour)
	record := servicedesk.EntrySla{
		ID: freshID(t), TenantID: tenantA, ItemID: item, Goal: servicedesk.GoalResolution, PolicyKey: "incidents",
		Target: 4 * time.Hour, Standing: servicedesk.StandingRunning, ArmedAt: now, DueAt: &due,
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.Records().Save(ctx, record, now); err != nil {
			return err
		}
		met := record
		met.Standing, met.MetAt, met.DueAt = servicedesk.StandingMet, &due, nil
		return repo.Records().Save(ctx, met, due)
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	// A next response is a goal the store takes (migration 0293).
	other := seedWorkItemForRules(ctx, t, tenantA)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		next := record
		next.ID, next.ItemID, next.Goal, next.ArmedAt = freshID(t), other, servicedesk.GoalNextResponse, now.Add(-3*time.Hour)
		return repo.Records().Save(ctx, next, now)
	}); err != nil {
		t.Errorf("a next response was refused: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		records, err := repo.Records().OfItem(ctx, item)
		if err != nil || len(records) != 1 || records[0].Standing != servicedesk.StandingMet || records[0].Version != 2 {
			return errors.Join(err, errors.New("the record was not kept whole"))
		}
		rows, err := repo.Records().Report(ctx, now.Add(-time.Hour), now.Add(time.Hour))
		if err != nil || len(rows) != 1 || rows[0].Entries != 1 {
			return errors.Join(err, errors.New("the report did not count it"))
		}
		return nil
	}); err != nil {
		t.Error(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		records, err := repo.Records().OfItem(ctx, item)
		rows, reportErr := repo.Records().Report(ctx, now.Add(-time.Hour), now.Add(time.Hour))
		if err != nil || reportErr != nil || len(records) != 0 || len(rows) != 0 {
			return errors.Join(err, reportErr, errors.New("the record crossed the workspace"))
		}
		return nil
	}); err != nil {
		t.Error(err)
	}
}
