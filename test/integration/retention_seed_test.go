// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A workspace nobody provisioned through the API - single mode's, inserted by the operator - and in
// which nothing was ever trashed: its sweep is seeded by the first sign-in, one interval out, and a
// second sign-in leaves the run where it was (data-retention.md §5). Without this its sessions,
// notifications and jumble would never age out.
func TestASignInSeedsTheSweepOfAWorkspaceThatNeverTrashed(t *testing.T) {
	ctx := context.Background()
	writer, _, _ := realWriter(ctx, t)
	writer.Jobs = postgres.NewQueue(writer.IDs, clockadapter.System{})
	writer.RetentionInterval = time.Hour
	admin := adminPool(ctx, t)

	workspace, person := freshID(t), freshID(t)
	slug := "seed-" + strings.ToLower(workspace.String()[24:])
	hash, err := writer.Passwords.Hash(secret.New("a sweep for everyone"))
	if err != nil {
		t.Fatalf("hashing: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name, default_locale, default_time_zone)
		VALUES ($1, $2, 'Never Trashed', 'en', 'UTC')`, workspace.String(), slug); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, password_hash, status)
		VALUES ($1, $2, 'USER', 'sweep@example.org', 'Sweep', $3, 'ACTIVE')`,
		person.String(), workspace.String(), hash); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}

	sweep := func() (int, time.Time) {
		t.Helper()
		var rows int
		var runAt *time.Time
		if err := admin.QueryRow(ctx, `
			SELECT count(*), min(run_at) FROM job
			WHERE kind = $1 AND tenant_id = $2 AND state IN ('PENDING', 'RUNNING')`,
			queue.KindRetentionSweep.String(), workspace.String()).Scan(&rows, &runAt); err != nil {
			t.Fatalf("reading the queue: %v", err)
		}
		if runAt == nil {
			return rows, time.Time{}
		}
		return rows, runAt.UTC()
	}
	signIn := func() {
		t.Helper()
		if _, err := (identityservice.SignIn{Writer: writer}).Execute(ctx, identityservice.SignInCommand{
			Email: "sweep@example.org", Password: secret.New("a sweep for everyone"),
			TenantHeader: workspace.String(), RemoteAddr: "198.51.100.40",
		}); err != nil {
			t.Fatalf("signing in: %v", err)
		}
	}

	if rows, _ := sweep(); rows != 0 {
		t.Fatalf("a sweep is waiting before anybody signed in (%d)", rows)
	}

	before := time.Now().UTC()
	signIn()
	rows, first := sweep()
	if rows != 1 {
		t.Fatalf("%d sweeps after the first sign-in, want one", rows)
	}
	if first.Before(before.Add(time.Hour).Add(-time.Second)) || first.After(time.Now().UTC().Add(time.Hour)) {
		t.Errorf("the sweep runs at %v, want one interval after the sign-in (%v)", first, before.Add(time.Hour))
	}

	signIn()
	rows, second := sweep()
	if rows != 1 || !second.Equal(first) {
		t.Errorf("after a second sign-in: %d sweeps at %v, want the one at %v", rows, second, first)
	}
}
