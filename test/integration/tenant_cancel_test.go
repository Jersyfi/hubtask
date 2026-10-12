// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A pending deletion can be called off (multi-tenancy.md §5), against the real database and as the
// app role: the request keeps where the workspace stood, the cancel returns it there, the grace job's
// mark and the cancel exclude each other on the one row, and the rules the request switched off are
// the ones a cancel finds - none of a neighbour's.

// cancellableWorkspace seeds a workspace of its own with a person and three rules - one on, one on
// with a schedule, one switched off by hand - and answers the workspace, the person and the rules.
func cancellableWorkspace(
	ctx context.Context, t *testing.T, status identity.TenantStatus,
) (tenant shared.ID, rules [3]shared.ID) {
	t.Helper()
	tenant, person := freshID(t), freshID(t)
	rules = [3]shared.ID{freshID(t), freshID(t), freshID(t)}
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name, status) VALUES ($1, $2, 'Leaving soon', $3)`,
			[]any{tenant.String(), "cancel-" + tenant.String()[24:], string(status)}},
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Nora')`,
			[]any{person.String(), tenant.String()}},
		{`INSERT INTO automation_rule (id, tenant_id, scope_type, name, run_as, trigger, actions, created_by, enabled)
		  VALUES ($1, $4, 'TENANT', 'on', $5, '{"kind":"EVENT","event_type":"item.created"}'::jsonb, '[]'::jsonb, $5, true),
		         ($2, $4, 'TENANT', 'nightly', $5, '{"kind":"SCHEDULE","rrule":"FREQ=DAILY;BYHOUR=3","timezone":"UTC"}'::jsonb, '[]'::jsonb, $5, true),
		         ($3, $4, 'TENANT', 'off by hand', $5, '{"kind":"EVENT","event_type":"item.created"}'::jsonb, '[]'::jsonb, $5, false)`,
			[]any{rules[0].String(), rules[1].String(), rules[2].String(), tenant.String(), person.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the workspace: %v\n%s", err, statement.sql)
		}
	}
	t.Cleanup(func() {
		done := context.WithoutCancel(ctx)
		for _, statement := range []string{
			`DELETE FROM automation_rule WHERE tenant_id = $1`,
			`DELETE FROM tenant WHERE id = $1`,
		} {
			if _, err := adminPool(done, t).Exec(done, statement, tenant.String()); err != nil {
				t.Errorf("clearing up: %v", err)
			}
		}
	})
	return tenant, rules
}

// within runs fn in one transaction of the workspace, failing the test on an error.
func within(
	ctx context.Context, t *testing.T, uow persistence.UnitOfWork, tenant shared.ID,
	fn func(context.Context) error,
) {
	t.Helper()
	if err := uow.Within(ctx, persistence.Scope{TenantID: tenant}, fn); err != nil {
		t.Fatalf("transaction: %v", err)
	}
}

// requestDeletion moves the workspace to PENDING_DELETION and switches its rules off, as the
// request's transaction does.
func requestDeletion(ctx context.Context, t *testing.T, uow persistence.UnitOfWork, tenant shared.ID, purgeAfter time.Time) {
	t.Helper()
	within(ctx, t, uow, tenant, func(ctx context.Context) error {
		moved, err := postgres.NewAdminTenantRepository().RequestDeletion(ctx, purgeAfter, time.Now().UTC())
		if err != nil || !moved {
			t.Fatalf("requesting the deletion (%v, %v)", moved, err)
		}
		_, err = postgres.NewAutomationSwitch().DisableAll(ctx, time.Now().UTC())
		return err
	})
}

func TestACancelReturnsTheWorkspaceWhereItStood(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	tenants := postgres.NewAdminTenantRepository()

	for _, from := range []identity.TenantStatus{identity.TenantActive, identity.TenantSuspended} {
		t.Run(string(from), func(t *testing.T) {
			tenant, _ := cancellableWorkspace(ctx, t, from)
			requestDeletion(ctx, t, uow, tenant, time.Now().Add(time.Hour).UTC())

			within(ctx, t, uow, tenant, func(ctx context.Context) error {
				record, err := tenants.Find(ctx)
				if err != nil {
					return err
				}
				if record.Status != identity.TenantPendingDeletion || record.StatusBeforeDeletion != from {
					t.Errorf("after the request: %s, before %q", record.Status, record.StatusBeforeDeletion)
				}
				back, cancelled, err := tenants.CancelDeletion(ctx, time.Now().UTC())
				if err != nil || !cancelled || back != from {
					t.Errorf("the cancel answered (%s, %v, %v), want %s", back, cancelled, err, from)
				}
				record, err = tenants.Find(ctx)
				if err != nil {
					return err
				}
				if record.Status != from || !record.PurgeAfter.IsZero() || record.StatusBeforeDeletion != "" {
					t.Errorf("after the cancel: %+v", record)
				}
				// A second cancel finds nothing pending.
				if _, again, err := tenants.CancelDeletion(ctx, time.Now().UTC()); err != nil || again {
					t.Errorf("a second cancel answered (%v, %v)", again, err)
				}
				return nil
			})
		})
	}
}

// A row requested before the column existed has no earlier status; the cancel returns it suspended.
func TestACancelWithoutTheEarlierStatusReturnsTheWorkspaceSuspended(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	tenant, _ := cancellableWorkspace(ctx, t, identity.TenantActive)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		UPDATE tenant SET status = 'PENDING_DELETION', purge_after = now() + interval '1 day'
		WHERE id = $1`, tenant.String()); err != nil {
		t.Fatalf("an older binary's request: %v", err)
	}
	within(ctx, t, uow, tenant, func(ctx context.Context) error {
		back, cancelled, err := postgres.NewAdminTenantRepository().CancelDeletion(ctx, time.Now().UTC())
		if err != nil || !cancelled || back != identity.TenantSuspended {
			t.Errorf("the cancel answered (%s, %v, %v)", back, cancelled, err)
		}
		return nil
	})
}

// The mark and the cancel exclude each other: once the purge has begun no cancel lands, and a
// cancel that landed first leaves the mark nothing to write.
func TestThePurgesMarkAndACancelExcludeEachOther(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	tenants := postgres.NewAdminTenantRepository()
	purge := postgres.NewTenantPurge()

	t.Run("the mark first", func(t *testing.T) {
		tenant, _ := cancellableWorkspace(ctx, t, identity.TenantActive)
		requestDeletion(ctx, t, uow, tenant, time.Now().Add(-time.Minute).UTC())
		within(ctx, t, uow, tenant, func(ctx context.Context) error {
			marked, err := purge.MarkStarted(ctx, time.Now().UTC())
			if err != nil || !marked {
				t.Fatalf("the mark answered (%v, %v)", marked, err)
			}
			if _, cancelled, err := tenants.CancelDeletion(ctx, time.Now().UTC()); err != nil || cancelled {
				t.Errorf("a cancel after the mark answered (%v, %v)", cancelled, err)
			}
			record, err := tenants.Find(ctx)
			if record.Status != identity.TenantPendingDeletion || record.PurgeStartedAt.IsZero() {
				t.Errorf("after the refused cancel: %+v", record)
			}
			return err
		})
	})

	t.Run("the cancel first", func(t *testing.T) {
		tenant, _ := cancellableWorkspace(ctx, t, identity.TenantActive)
		requestDeletion(ctx, t, uow, tenant, time.Now().Add(-time.Minute).UTC())
		within(ctx, t, uow, tenant, func(ctx context.Context) error {
			if _, cancelled, err := tenants.CancelDeletion(ctx, time.Now().UTC()); err != nil || !cancelled {
				t.Fatalf("the cancel answered (%v, %v)", cancelled, err)
			}
			if marked, err := purge.MarkStarted(ctx, time.Now().UTC()); err != nil || marked {
				t.Errorf("a mark after the cancel answered (%v, %v)", marked, err)
			}
			return nil
		})
	})

	t.Run("not before the deadline", func(t *testing.T) {
		tenant, _ := cancellableWorkspace(ctx, t, identity.TenantActive)
		requestDeletion(ctx, t, uow, tenant, time.Now().Add(time.Hour).UTC())
		within(ctx, t, uow, tenant, func(ctx context.Context) error {
			if marked, err := purge.MarkStarted(ctx, time.Now().UTC()); err != nil || marked {
				t.Errorf("a mark before the deadline answered (%v, %v)", marked, err)
			}
			return nil
		})
	})
}

// The listing answers the purge's mark as the app role, through the enumerator.
func TestTheInstallationListingSaysWhereThePurgeHasBegun(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	started, _ := cancellableWorkspace(ctx, t, identity.TenantActive)
	waiting, _ := cancellableWorkspace(ctx, t, identity.TenantActive)
	requestDeletion(ctx, t, uow, started, time.Now().Add(-time.Minute).UTC())
	requestDeletion(ctx, t, uow, waiting, time.Now().Add(time.Hour).UTC())
	within(ctx, t, uow, started, func(ctx context.Context) error {
		_, err := postgres.NewTenantPurge().MarkStarted(ctx, time.Now().UTC())
		return err
	})

	seen := map[shared.ID]time.Time{}
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		records, err := postgres.NewAdminTenantRepository().List(ctx)
		for _, record := range records {
			seen[record.ID] = record.PurgeStartedAt
		}
		return err
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	if seen[started].IsZero() || !seen[waiting].IsZero() {
		t.Errorf("the listing says started %v and waiting %v", seen[started], seen[waiting])
	}
}

// The request marks exactly the rules it switched off; the cancel's reader finds those, and a
// neighbour's transaction finds none of them (the cross-tenant negative).
func TestTheRulesTheRequestSwitchedOffAreTheOnesFound(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	tenant, rules := cancellableWorkspace(ctx, t, identity.TenantActive)
	neighbour, _ := cancellableWorkspace(ctx, t, identity.TenantActive)
	requestDeletion(ctx, t, uow, tenant, time.Now().Add(time.Hour).UTC())
	paused := postgres.NewPausedAutomationRules()

	within(ctx, t, uow, neighbour, func(ctx context.Context) error {
		found, err := paused.List(ctx)
		if len(found) != 0 {
			t.Errorf("the neighbour sees %d paused rules", len(found))
		}
		for _, rule := range rules {
			if resumed, err := paused.Resume(ctx, rule, time.Time{}, time.Now().UTC()); err != nil || resumed {
				t.Errorf("the neighbour resumed %s (%v, %v)", rule, resumed, err)
			}
		}
		return err
	})

	within(ctx, t, uow, tenant, func(ctx context.Context) error {
		found, err := paused.List(ctx)
		if err != nil {
			return err
		}
		ids := map[shared.ID]bool{}
		for _, rule := range found {
			ids[rule.ID] = true
		}
		if len(found) != 2 || !ids[rules[0]] || !ids[rules[1]] || ids[rules[2]] {
			t.Errorf("found %v, want the two the request switched off", ids)
		}
		next := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
		if resumed, err := paused.Resume(ctx, rules[1], next, time.Now().UTC()); err != nil || !resumed {
			t.Errorf("resuming answered (%v, %v)", resumed, err)
		}
		if again, err := paused.Resume(ctx, rules[1], next, time.Now().UTC()); err != nil || again {
			t.Errorf("a second resume answered (%v, %v)", again, err)
		}
		return nil
	})

	var enabled, marked bool
	var nextRun *time.Time
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT enabled, paused_for_deletion, next_run_at FROM automation_rule WHERE id = $1`,
		rules[1].String()).Scan(&enabled, &marked, &nextRun); err != nil {
		t.Fatalf("reading the rule back: %v", err)
	}
	if !enabled || marked || nextRun == nil {
		t.Errorf("the resumed rule reads enabled %v, marked %v, next %v", enabled, marked, nextRun)
	}
}
