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
	"github.com/Jersyfi/hubtask/core/port/queue"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A dead duty goes back to work once its wait has run out - an hour doubled by each earlier
// revival - unless its key already has an open job or a newer dead letter; other kinds stay dead.
func TestADeadDutyIsRevivedAfterItsWait(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	duty := "test.duty-" + freshID(t).String()
	other := "test.other-" + freshID(t).String()
	now := time.Now().UTC()

	seed := func(kind, state, key string, revivals int, finishedAt time.Time) shared.ID {
		t.Helper()
		id := freshID(t)
		var dedupe any
		if key != "" {
			dedupe = key
		}
		if _, err := admin.Exec(ctx, `
			INSERT INTO job (id, kind, state, dedupe_key, attempts, revivals, finished_at)
			VALUES ($1, $2, $3, $4, 8, $5, $6)`,
			id.String(), kind, state, dedupe, revivals, finishedAt); err != nil {
			t.Fatalf("seeding: %v", err)
		}
		return id
	}
	revived := seed(duty, "DEAD_LETTER", "a", 0, now.Add(-2*time.Hour))
	waiting := seed(duty, "DEAD_LETTER", "b", 2, now.Add(-2*time.Hour)) // waits four hours
	covered := seed(duty, "DEAD_LETTER", "c", 0, now.Add(-2*time.Hour))
	seed(duty, "PENDING", "c", 0, now)
	older := seed(duty, "DEAD_LETTER", "d", 0, now.Add(-3*time.Hour))
	newer := seed(duty, "DEAD_LETTER", "d", 0, now.Add(-2*time.Hour))
	notADuty := seed(other, "DEAD_LETTER", "a", 0, now.Add(-2*time.Hour))

	jobs := postgres.NewQueue(clockadapter.NewUUIDv7(clockadapter.System{}), clockadapter.System{})
	var counts map[queue.Kind]int
	if err := postgres.NewUnitOfWork(appPool(ctx, t)).Within(ctx, persistence.SystemScope(),
		func(ctx context.Context) error {
			var err error
			counts, err = jobs.ReviveDeadDuties(ctx, []queue.Kind{queue.Kind(duty)}, 1000)
			return err
		}); err != nil {
		t.Fatalf("reviving: %v", err)
	}

	state := func(id shared.ID) (string, int, int) {
		var s string
		var attempts, revivals int
		if err := admin.QueryRow(ctx, `SELECT state, attempts, revivals FROM job WHERE id = $1`, id.String()).
			Scan(&s, &attempts, &revivals); err != nil {
			t.Fatal(err)
		}
		return s, attempts, revivals
	}
	if s, attempts, revivals := state(revived); s != "PENDING" || attempts != 0 || revivals != 1 {
		t.Errorf("the dead duty is %s, %d attempts, %d revivals; want PENDING, 0, 1", s, attempts, revivals)
	}
	if s, _, _ := state(newer); s != "PENDING" {
		t.Errorf("a key's newest dead letter is %s, want PENDING", s)
	}
	for name, id := range map[string]shared.ID{
		"a duty still in its wait": waiting, "a duty with an open job": covered,
		"a key's older dead letter": older, "another kind": notADuty,
	} {
		if s, _, _ := state(id); s != "DEAD_LETTER" {
			t.Errorf("%s was revived (%s)", name, s)
		}
	}
	if counts[queue.Kind(duty)] != 2 {
		t.Errorf("revived %v, want two of the duty kind", counts)
	}
}
