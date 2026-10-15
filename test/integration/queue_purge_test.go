// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/worker"
)

// UC-PRV-08 check 2, the queue's part: a finished job goes seven days after it ended, in every
// terminal state and whichever workspace it was for - the table has none to scope by - and a job
// still waiting or running stays, however old.
func TestFinishedJobsGoAfterTheCataloguesPeriod(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	kind := "test.purge-" + freshID(t).String()
	now := time.Now().UTC()
	old, recent := now.Add(-8*24*time.Hour), now.Add(-6*24*time.Hour)
	tenant := fanOutTenant(ctx, t, "purge-"+freshID(t).String()+"@example.org")

	seed := func(state string, tenantID any, finishedAt *time.Time) shared.ID {
		t.Helper()
		id := freshID(t)
		if _, err := admin.Exec(ctx, `
			INSERT INTO job (id, tenant_id, kind, payload, state, created_at, finished_at)
			VALUES ($1, $2, $3, '{"account_id":"someone"}', $4, $5, $6)`,
			id.String(), tenantID, kind, state, old.Add(-time.Hour), finishedAt); err != nil {
			t.Fatalf("seeding a %s job: %v", state, err)
		}
		return id
	}
	gone := []shared.ID{
		seed("SUCCEEDED", tenant.String(), &old),
		seed("FAILED", tenant.String(), &old),
		seed("DEAD_LETTER", nil, &old),
		seed("CANCELLED", tenant.String(), &old),
	}
	kept := []shared.ID{
		seed("SUCCEEDED", tenant.String(), &recent),
		seed("DEAD_LETTER", nil, &recent),
		seed("PENDING", tenant.String(), nil),
		seed("RUNNING", nil, nil),
	}

	jobs := postgres.NewQueue(clockadapter.NewUUIDv7(clockadapter.System{}), clockadapter.System{})
	var purged map[string]int
	if err := postgres.NewUnitOfWork(appPool(ctx, t)).Within(ctx, persistence.SystemScope(),
		func(ctx context.Context) error {
			var err error
			purged, err = jobs.PurgeFinished(ctx, now.Add(-worker.FinishedJobPeriod), 1000)
			return err
		}); err != nil {
		t.Fatalf("purging: %v", err)
	}

	for _, id := range gone {
		if countIn(ctx, t, `SELECT count(*) FROM job WHERE id = $1`, id.String()) != 0 {
			t.Errorf("the finished job %s outlived the period", id)
		}
	}
	for _, id := range kept {
		if countIn(ctx, t, `SELECT count(*) FROM job WHERE id = $1`, id.String()) != 1 {
			t.Errorf("the job %s went though it is recent or not finished", id)
		}
	}
	for _, state := range []string{"SUCCEEDED", "FAILED", "DEAD_LETTER", "CANCELLED"} {
		if purged[state] < 1 {
			t.Errorf("the purge reports %v, want each terminal state counted", purged)
		}
	}
}

// A run takes at most its batch, oldest first; the rest waits for the next one.
func TestAPurgeTakesItsBatchOldestFirst(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	kind := "test.purge-batch-" + freshID(t).String()
	base := time.Now().UTC().Add(-30 * 24 * time.Hour)
	var ids []shared.ID
	for i := range 3 {
		id := freshID(t)
		finished := base.Add(time.Duration(i) * time.Minute)
		if _, err := admin.Exec(ctx, `
			INSERT INTO job (id, kind, state, finished_at) VALUES ($1, $2, 'SUCCEEDED', $3)`,
			id.String(), kind, finished); err != nil {
			t.Fatalf("seeding: %v", err)
		}
		ids = append(ids, id)
	}

	jobs := postgres.NewQueue(clockadapter.NewUUIDv7(clockadapter.System{}), clockadapter.System{})
	// Before the third one ended: only the two older ones are due, and the batch is one.
	before := base.Add(90 * time.Second)
	if err := postgres.NewUnitOfWork(appPool(ctx, t)).Within(ctx, persistence.SystemScope(),
		func(ctx context.Context) error {
			_, err := jobs.PurgeFinished(ctx, before, 1)
			return err
		}); err != nil {
		t.Fatalf("purging: %v", err)
	}
	left := countIn(ctx, t, `SELECT count(*) FROM job WHERE kind = $1`, kind)
	if left != 2 || countIn(ctx, t, `SELECT count(*) FROM job WHERE id = $1`, ids[0].String()) != 0 {
		t.Errorf("%d of the three are left, want the oldest gone and two kept", left)
	}
}
