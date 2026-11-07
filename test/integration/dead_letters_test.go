// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The operator's dead letters (UC-INS-12): listed by kind, newest first, across workspaces; one is
// requeued with its attempts from zero; one whose key has an open job, and a job that is no dead
// letter, are refused and left as they were.
func TestTheOperatorRequeuesADeadLetter(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	kind := "test.dead-" + freshID(t).String()

	seed := func(tenant any, state, key string) shared.ID {
		t.Helper()
		id := freshID(t)
		var dedupe any
		if key != "" {
			dedupe = key
		}
		if _, err := admin.Exec(ctx, `
			INSERT INTO job (id, tenant_id, kind, state, dedupe_key, attempts, last_error, finished_at, payload)
			VALUES ($1, $2, $3, $4, $5, 8, 'dependency.unavailable', now(), '{"secret":"kept in the table"}')`,
			id.String(), tenant, kind, state, dedupe); err != nil {
			t.Fatalf("seeding: %v", err)
		}
		return id
	}
	first := seed(tenantA.String(), "DEAD_LETTER", "a")
	second := seed(tenantB.String(), "DEAD_LETTER", "b")
	seed(nil, "PENDING", "b")
	finished := seed(nil, "SUCCEEDED", "")

	repo := postgres.NewDeadLetterRepository()
	unit := postgres.NewUnitOfWork(appPool(ctx, t))
	var page []adminrepo.DeadLetter
	if err := unit.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		var err error
		page, err = repo.Page(ctx, kind, "", 10)
		return err
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page) != 2 || page[0].ID != second || page[1].ID != first || page[0].TenantID != tenantB ||
		page[0].ErrorCode != "dependency.unavailable" || page[0].FinishedAt.IsZero() {
		t.Fatalf("listed %+v", page)
	}

	requeue := func(id shared.ID) (adminrepo.DeadLetter, error) {
		var letter adminrepo.DeadLetter
		err := unit.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
			var err error
			letter, err = repo.Requeue(ctx, id, time.Now())
			return err
		})
		return letter, err
	}
	letter, err := requeue(first)
	if err != nil || !letter.Requeued || letter.Attempts != 0 {
		t.Fatalf("requeued %+v, %v", letter, err)
	}
	var state string
	var attempts int
	if err := admin.QueryRow(ctx, `SELECT state, attempts FROM job WHERE id = $1`, first.String()).
		Scan(&state, &attempts); err != nil || state != "PENDING" || attempts != 0 {
		t.Errorf("the row is %s with %d attempts (%v)", state, attempts, err)
	}
	if _, err := requeue(second); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("an open key answered %v", err)
	}
	if _, err := requeue(finished); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a finished job answered %v", err)
	}
	if err := admin.QueryRow(ctx, `SELECT state FROM job WHERE id = $1`, second.String()).Scan(&state); err != nil || state != "DEAD_LETTER" {
		t.Errorf("the refused one is %s (%v)", state, err)
	}
}
