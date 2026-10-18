// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A restriction of processing holds against retention (UC-PRV-04 check 8, data-retention.md §4
// item 2): a due entry a restricted person created is kept by the rule, counted as held, and says
// that a restriction holds it; once the restriction is lifted the next pass acts on it. Against
// PostgreSQL, as the application role, with the restriction read through the same query automation
// and the AI exclusion use.
func TestARestrictedPersonsDueEntryIsKeptUntilTheRestrictionIsLifted(t *testing.T) {
	ctx := context.Background()
	subject := seedSubject(ctx, t, "restricted-retention@example.test")
	now := time.Now().UTC()
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE work_item SET is_completed = true, completed_at = $3, completed_by = $4
		 WHERE tenant_id = $1 AND id = $2`,
		subject.tenant.String(), subject.item.String(), now.AddDate(0, 0, -400), subject.account.String()); err != nil {
		t.Fatalf("completing the entry: %v", err)
	}

	grace := 0
	rule, err := domain.NewRule(domain.NewRuleInput{
		ID: freshID(t), TenantID: subject.tenant, Scope: domain.Scope{Kind: domain.ScopeTenant},
		DataKind: domain.KindCompletedItem, RetainDays: 30, Action: domain.ActionTrash,
		GraceDays: &grace, CreatedBy: subject.account, Now: now,
	})
	if err != nil {
		t.Fatalf("building the rule: %v", err)
	}
	if err := write(ctx, t, subject.tenant, func(ctx context.Context) error {
		return ruleRepo().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	setStatus := func(status string) {
		t.Helper()
		if err := write(ctx, t, subject.tenant, func(ctx context.Context) error {
			_, err := privacyRepo().SetStatus(ctx, subject.account, status, time.Now().UTC())
			return err
		}); err != nil {
			t.Fatalf("setting the account %s: %v", status, err)
		}
	}
	setStatus("RESTRICTED")

	clock := portclock.Fixed(now)
	hybrid, err := clockadapter.NewHybridClock(clock, "retention-restriction")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	sweeper := lifecycle.Sweeper{
		Rules: ruleRepo(), Marking: markingRepo(), Holds: lifecycleRepo(),
		Restrictions: privacyRepo(), Items: itemRepo(), Changes: postgres.NewChangeLog(),
		Clock: clock, IDs: clockadapter.NewUUIDv7(clock), HLC: hybrid,
	}
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: subject.tenant}
	pass := func() lifecycle.Outcome {
		t.Helper()
		var outcome lifecycle.Outcome
		if err := write(ctx, t, subject.tenant, func(ctx context.Context) error {
			var err error
			outcome, err = sweeper.Pass(ctx, actor)
			return err
		}); err != nil {
			t.Fatalf("the pass failed: %v", err)
		}
		return outcome
	}
	state := func() (blockedBy *string, trashed bool) {
		t.Helper()
		if err := adminPool(ctx, t).QueryRow(ctx,
			`SELECT retention_blocked_by, deleted_at IS NOT NULL FROM work_item WHERE tenant_id = $1 AND id = $2`,
			subject.tenant.String(), subject.item.String()).Scan(&blockedBy, &trashed); err != nil {
			t.Fatalf("reading the entry: %v", err)
		}
		return blockedBy, trashed
	}

	held := pass()
	blockedBy, trashed := state()
	if trashed {
		t.Fatal("the rule removed a restricted person's entry")
	}
	if blockedBy == nil || *blockedBy != domain.BlockedByRestriction {
		t.Errorf("the entry says it is blocked by %v, want %q", blockedBy, domain.BlockedByRestriction)
	}
	if held.Blocked[domain.BlockedByRestriction] != 1 || held.Held != 1 {
		t.Errorf("the pass counted %v held (%d), want one under %q", held.Blocked, held.Held, domain.BlockedByRestriction)
	}

	setStatus("ACTIVE")
	pass()
	if _, trashed := state(); !trashed {
		t.Error("the entry stayed after the restriction was lifted")
	}
}

// The automatic trash purge keeps a deletion a restricted person contributed to, and takes it once
// the restriction is lifted.
func TestTheAutomaticTrashPurgeKeepsARestrictedPersonsDeletion(t *testing.T) {
	ctx := context.Background()
	subject := seedSubject(ctx, t, "restricted-trash@example.test")
	now := time.Now().UTC()
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE work_item SET deleted_at = $3, trash_batch_id = $4 WHERE tenant_id = $1 AND id = $2`,
		subject.tenant.String(), subject.item.String(), now.AddDate(0, 0, -90), freshID(t).String()); err != nil {
		t.Fatalf("trashing the entry: %v", err)
	}

	clock := portclock.Fixed(now)
	ids := clockadapter.NewUUIDv7(clock)
	purger := lifecycle.Purger{
		Trash: postgres.NewTrashRepository(pageCursors()), Expired: lifecycleRepo(), Holds: lifecycleRepo(),
		Restrictions: privacyRepo(), Removals: lifecycleRepo(), Events: postgres.NewOutbox(jobQueue(t)),
		Audit: postgres.NewAuditSink(ids), Clock: clock, IDs: ids, BatchSize: 100,
	}
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: subject.tenant}
	sweep := func() lifecycle.Outcome {
		t.Helper()
		var outcome lifecycle.Outcome
		if err := write(ctx, t, subject.tenant, func(ctx context.Context) error {
			var err error
			outcome, err = purger.Sweep(ctx, actor, lifecycle.Selection{
				Cutoff: now.AddDate(0, 0, -30), Reason: domain.DeletedByRetention,
				ObserveTombstoneWindow: true, ObserveRestrictions: true,
			}, now)
			return err
		}); err != nil {
			t.Fatalf("the purge failed: %v", err)
		}
		return outcome
	}
	present := func() bool {
		t.Helper()
		var count int
		if err := adminPool(ctx, t).QueryRow(ctx,
			`SELECT count(*) FROM work_item WHERE tenant_id = $1 AND id = $2`,
			subject.tenant.String(), subject.item.String()).Scan(&count); err != nil {
			t.Fatalf("reading the entry: %v", err)
		}
		return count == 1
	}
	setStatus := func(status string) {
		t.Helper()
		if err := write(ctx, t, subject.tenant, func(ctx context.Context) error {
			_, err := privacyRepo().SetStatus(ctx, subject.account, status, time.Now().UTC())
			return err
		}); err != nil {
			t.Fatalf("setting the account %s: %v", status, err)
		}
	}

	setStatus("RESTRICTED")
	kept := sweep()
	if !present() {
		t.Fatal("the automatic purge removed a restricted person's deletion")
	}
	if kept.Blocked[domain.BlockedByRestriction] != 1 {
		t.Errorf("the purge counted %v held, want one under %q", kept.Blocked, domain.BlockedByRestriction)
	}

	setStatus("ACTIVE")
	sweep()
	if present() {
		t.Error("the deletion stayed after the restriction was lifted")
	}
}
